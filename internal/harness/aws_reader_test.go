package harness

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The hosts the read's endpoint map sends to in awsReadTestConfig's region.
const (
	awsReadSTSHost = "sts.eu-west-1.amazonaws.com"
	awsReadEC2Host = "ec2.eu-west-1.amazonaws.com"
	awsReadSSMHost = "ssm.eu-west-1.amazonaws.com"
	awsReadIAMHost = "iam.amazonaws.com"
)

// awsWireRequest is one request as it reached the wire.
type awsWireRequest struct {
	host   string
	header http.Header
	body   string
}

func (r awsWireRequest) form(t *testing.T) url.Values {
	t.Helper()
	form, err := url.ParseQuery(r.body)
	require.NoError(t, err)
	return form
}

// awsWireDoer answers every request with answer, calling onSend first,
// and records each request that reached it.
type awsWireDoer struct {
	answer awsCannedAnswer
	onSend func()
	sent   []awsWireRequest
}

func (d *awsWireDoer) Do(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	d.sent = append(d.sent, awsWireRequest{host: req.URL.Host, header: req.Header.Clone(), body: string(body)})
	if d.onSend != nil {
		d.onSend()
	}
	return &http.Response{
		StatusCode: cmp.Or(d.answer.status, http.StatusOK),
		Header:     http.Header{"Content-Type": {"text/xml"}},
		Body:       io.NopCloser(strings.NewReader(d.answer.body)),
		Request:    req,
	}, nil
}

// awsHostDoer sends each request to the doer for its URL's host, and
// fails any request to another host.
type awsHostDoer map[string]aws.HTTPClient

func (d awsHostDoer) Do(req *http.Request) (*http.Response, error) {
	doer, ok := d[req.URL.Host]
	if !ok {
		return nil, errors.New("no doer for host " + req.URL.Host)
	}
	return doer.Do(req)
}

// awsReadTestDoers is a recording doer per service; sts answers with the
// key's identity.
type awsReadTestDoers struct {
	sts, ssm, iam *awsWireDoer
	ec2           *dryRunEC2Doer
}

func newAWSReadTestDoers() *awsReadTestDoers {
	return &awsReadTestDoers{
		sts: &awsWireDoer{answer: awsCannedAnswer{body: getCallerIdentityXMLBody}},
		ec2: &dryRunEC2Doer{answers: map[string]awsCannedAnswer{}},
		ssm: &awsWireDoer{}, iam: &awsWireDoer{},
	}
}

func (d *awsReadTestDoers) doer() awsHostDoer {
	return awsHostDoer{awsReadSTSHost: d.sts, awsReadEC2Host: d.ec2, awsReadSSMHost: d.ssm, awsReadIAMHost: d.iam}
}

// sent is the number of requests that reached every doer.
func (d *awsReadTestDoers) sent() int {
	return len(d.sts.sent) + len(d.ec2.forms) + len(d.ssm.sent) + len(d.iam.sent)
}

func passingPreflight(context.Context, aws.Config, *AWSReadWall) error { return nil }

// awsReadTestInput is a read of the test account through doer, with the
// real identity check and a preflight that passes.
func awsReadTestInput(doer aws.HTTPClient, readSet ...AWSReadEntry) AWSReadInput {
	return AWSReadInput{
		Config:    awsReadTestConfig(&recordingAWSDoer{}),
		Account:   testAWSAccount,
		ReadSet:   readSet,
		Doer:      doer,
		Identity:  VerifyAWSReadIdentity,
		Preflight: passingPreflight,
	}
}

func readEntry(service, protocol, action, params string) AWSReadEntry {
	return AWSReadEntry{Service: service, Protocol: protocol, Action: action, Params: json.RawMessage(params)}
}

// assertSignedFor holds a request to a SigV4 Authorization header scoped
// to service in region, signed with the test key.
func assertSignedFor(t *testing.T, req awsWireRequest, region, service string) {
	t.Helper()
	auth := req.header.Get("Authorization")
	assert.True(t, strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential="+testAWSKeyID+"/"), "Authorization %q", auth)
	assert.Contains(t, auth, "/"+region+"/"+service+"/aws4_request,", "Authorization %q", auth)
}

func TestReadAWSAccountReplaysEachEntryVerbatimAndSigned(t *testing.T) {
	t.Parallel()

	const ssmParams = `{"Name": "/app/db/host", "WithDecryption": true}`
	doers := newAWSReadTestDoers()
	doers.ec2.answers["DescribeSecurityGroups"] = awsCannedAnswer{status: http.StatusOK, body: "<DescribeSecurityGroupsResponse/>"}
	doers.ec2.answers["DescribeImages"] = awsCannedAnswer{status: http.StatusOK, body: "<DescribeImagesResponse/>"}
	doers.ssm.answer.body = `{"Parameter":{"Name":"/app/db/host"}}`
	doers.iam.answer.body = "<GetRoleResponse/>"
	readSet := []AWSReadEntry{
		readEntry("ec2", "query", "DescribeSecurityGroups", `{"Filter.1.Name": "vpc-id", "Filter.1.Value.1": "vpc-0123456789abcdef0", "GroupName.1": "web sg+1"}`),
		readEntry("ec2", "query", "DescribeImages", `{"Owner.1": "amazon", "Filter.1.Name": "name", "Filter.1.Value.1": "al2023-ami-*"}`),
		readEntry("ssm", "json", "GetParameter", ssmParams),
		readEntry("iam", "query", "GetRole", `{"RoleName": "web"}`),
	}

	read, err := ReadAWSAccount(context.Background(), awsReadTestInput(doers.doer(), readSet...))
	require.NoError(t, err)

	require.Len(t, doers.ec2.forms, 2)
	assert.Equal(t, url.Values{
		"Action": {"DescribeSecurityGroups"}, "Version": {"2016-11-15"},
		"Filter.1.Name": {"vpc-id"}, "Filter.1.Value.1": {"vpc-0123456789abcdef0"}, "GroupName.1": {"web sg+1"},
	}, doers.ec2.forms[0])
	assert.Equal(t, url.Values{
		"Action": {"DescribeImages"}, "Version": {"2016-11-15"},
		"Owner.1": {"amazon"}, "Filter.1.Name": {"name"}, "Filter.1.Value.1": {"al2023-ami-*"},
	}, doers.ec2.forms[1])

	require.Len(t, doers.ssm.sent, 1)
	assert.Equal(t, ssmParams, doers.ssm.sent[0].body, "the json params are the body verbatim")
	assert.Equal(t, "AmazonSSM.GetParameter", doers.ssm.sent[0].header.Get("X-Amz-Target"))
	assertSignedFor(t, doers.ssm.sent[0], "eu-west-1", "ssm")

	require.Len(t, doers.iam.sent, 1)
	assert.Equal(t, awsReadIAMHost, doers.iam.sent[0].host)
	assert.Equal(t, url.Values{"Action": {"GetRole"}, "Version": {"2010-05-08"}, "RoleName": {"web"}}, doers.iam.sent[0].form(t))
	assertSignedFor(t, doers.iam.sent[0], "us-east-1", "iam")

	require.Len(t, doers.sts.sent, 1)
	assertSignedFor(t, doers.sts.sent[0], "eu-west-1", "sts")

	require.Len(t, read.Exchanges, len(readSet))
	for i, exchange := range read.Exchanges {
		assert.Equal(t, readSet[i], exchange.AWSReadEntry, "exchange %d holds its entry verbatim", i)
		assert.Equal(t, http.StatusOK, exchange.Status)
	}
	assert.Equal(t, `{"Parameter":{"Name":"/app/db/host"}}`, read.Exchanges[2].Body)
	assert.Equal(t, testAWSAccount, read.Account)
	assert.Equal(t, "eu-west-1", read.Region)
}

// The EC2 replay's signature, which dryRunEC2Doer does not keep.
func TestReadAWSAccountSignsEC2ForItsRegion(t *testing.T) {
	t.Parallel()

	doers := newAWSReadTestDoers()
	ec2 := &awsWireDoer{answer: awsCannedAnswer{body: describeVpcsXMLBody}}
	doer := doers.doer()
	doer[awsReadEC2Host] = ec2

	_, err := ReadAWSAccount(context.Background(), awsReadTestInput(doer, readEntry("ec2", "query", "DescribeVpcs", `{}`)))
	require.NoError(t, err)
	require.Len(t, ec2.sent, 1)
	assertSignedFor(t, ec2.sent[0], "eu-west-1", "ec2")
}

func TestReadAWSAccountRefusesWithNothingSent(t *testing.T) {
	t.Parallel()

	describeVpcs := readEntry("ec2", "query", "DescribeVpcs", `{}`)
	cases := []struct {
		name    string
		edit    func(*AWSReadInput)
		wantErr string
	}{
		{"no identity check", func(in *AWSReadInput) { in.Identity = nil }, "no identity check was given"},
		{"no preflight", func(in *AWSReadInput) { in.Preflight = nil }, "no preflight was given"},
		{"an entry outside the reader table", func(in *AWSReadInput) {
			in.ReadSet = append(in.ReadSet, readEntry("ec2", "query", "CreateVpc", `{"CidrBlock":"10.0.0.0/16"}`))
		}, `entry 1 (service "ec2", protocol "query", action "CreateVpc") is not in the reader table`},
		{"an endpoint for a service outside the table", func(in *AWSReadInput) {
			in.Endpoints = map[string]string{"EC2": "http://127.0.0.1:1"}
		}, `"EC2", which is not a reader-table service`},
		{"a query entry repeating a param", func(in *AWSReadInput) {
			in.ReadSet = append(in.ReadSet, readEntry("ec2", "query", "DescribeSecurityGroups", `{"GroupName.1":"web","GroupName.1":"db"}`))
		}, `entry 1 (service "ec2", protocol "query", action "DescribeSecurityGroups"): query params repeat "GroupName.1"`},
		{"a query param that is not a string", func(in *AWSReadInput) {
			in.ReadSet = append(in.ReadSet, readEntry("ec2", "query", "DescribeVpcs", `{"VpcId.1":null}`))
		}, `entry 1 (service "ec2", protocol "query", action "DescribeVpcs"): query param "VpcId.1" must be a string`},
		{"a query param naming another action", func(in *AWSReadInput) {
			in.ReadSet = append(in.ReadSet, readEntry("ec2", "query", "DescribeVpcs", `{"action":"CreateVpc"}`))
		}, `the form carries "action", a case variant of Action`},
		{"a case variant of DryRun", func(in *AWSReadInput) {
			in.ReadSet = append(in.ReadSet, readEntry("ec2", "query", "DescribeVpcs", `{"dryRun":"true"}`))
		}, `entry 1 (service "ec2", protocol "query", action "DescribeVpcs"): aws read wall: refused: ec2 request: the form carries "dryRun", a case variant of DryRun`},
		{"DryRun=false", func(in *AWSReadInput) {
			in.ReadSet = append(in.ReadSet, readEntry("ec2", "query", "DescribeVpcs", `{"DryRun":"false"}`))
		}, `DryRun is "false", not exactly true`},
		{"a json Action field", func(in *AWSReadInput) {
			in.ReadSet = append(in.ReadSet, readEntry("ssm", "json", "GetParameter", `{"Name":"/a","action":"x"}`))
		}, `the json body carries "action", a query-protocol field`},
		{"query params that are not an object", func(in *AWSReadInput) {
			in.ReadSet = append(in.ReadSet, readEntry("ec2", "query", "DescribeVpcs", `["VpcId.1"]`))
		}, "query params must be a JSON object of strings"},
		{"query params with data after the object", func(in *AWSReadInput) {
			in.ReadSet = append(in.ReadSet, readEntry("ec2", "query", "DescribeVpcs", `{"VpcId.1":"vpc-1"}{"Action":"CreateVpc"}`))
		}, `entry 1 (service "ec2", protocol "query", action "DescribeVpcs"): query params carry data after the object`},
		{"json params that are not an object", func(in *AWSReadInput) {
			in.ReadSet = append(in.ReadSet, readEntry("ssm", "json", "GetParameter", `["/app/db/host"]`))
		}, `entry 1 (service "ssm", protocol "json", action "GetParameter"): aws read wall: refused: ssm request: the body is not a JSON object`},
		{"json params with data after the object", func(in *AWSReadInput) {
			in.ReadSet = append(in.ReadSet, readEntry("ssm", "json", "GetParameter", `{"Name":"/a"}{"Name":"/b"}`))
		}, "the body is not a JSON object"},
		{"a GovCloud region", func(in *AWSReadInput) { in.Config.Region = "us-gov-west-1" }, `region "us-gov-west-1" is outside the aws partition`},
		{"a China region", func(in *AWSReadInput) { in.Config.Region = "cn-north-1" }, `region "cn-north-1" is outside the aws partition`},
		{"an endpoint on another host", func(in *AWSReadInput) {
			in.Endpoints = map[string]string{"ec2": "https://ec2.example.com"}
		}, `the ec2 endpoint: "https://ec2.example.com" is not on a loopback IP`},
		{"an endpoint named localhost", func(in *AWSReadInput) {
			in.Endpoints = map[string]string{"sts": "http://localhost:8082/sts"}
		}, `"http://localhost:8082/sts" is not on a loopback IP`},
		{"an endpoint over another scheme", func(in *AWSReadInput) {
			in.Endpoints = map[string]string{"ssm": "ftp://127.0.0.1:1/ssm"}
		}, `"ftp://127.0.0.1:1/ssm" is not http or https`},
		{"no credentials", func(in *AWSReadInput) { in.Config.Credentials = nil }, "no credentials"},
		{"no doer", func(in *AWSReadInput) { in.Doer = nil }, "needs an HTTP client"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wire := &awsWireDoer{}
			in := awsReadTestInput(wire, describeVpcs)
			tc.edit(&in)

			read, err := ReadAWSAccount(context.Background(), in)

			assert.ErrorContains(t, err, tc.wantErr)
			assert.Empty(t, wire.sent, "a refused read sent a request")
			assert.Empty(t, read.Exchanges)
		})
	}
}

func TestReadAWSAccountWrongAccountSendsOnlyGetCallerIdentity(t *testing.T) {
	t.Parallel()

	doers := newAWSReadTestDoers()
	in := awsReadTestInput(doers.doer(), readEntry("ec2", "query", "DescribeVpcs", `{}`), readEntry("iam", "query", "GetRole", `{"RoleName":"web"}`))
	in.Account = "210987654321"
	in.Preflight = PreflightAWSReadKey

	_, err := ReadAWSAccount(context.Background(), in)

	assert.ErrorContains(t, err, `--account "210987654321"`)
	require.Len(t, doers.sts.sent, 1)
	assert.Equal(t, "GetCallerIdentity", doers.sts.sent[0].form(t).Get("Action"))
	assert.Equal(t, 1, doers.sent(), "a request followed the wrong account's identity answer")
}

func TestReadAWSAccountFailingPreflightSendsNoReadSetRequest(t *testing.T) {
	t.Parallel()

	doers := newAWSReadTestDoers()
	doers.ec2.answers["DeleteVpc"] = awsCannedAnswer{status: http.StatusPreconditionFailed, body: ec2ErrorBody("DryRunOperation")}
	doers.ec2.answers["DescribeVpcs"] = awsCannedAnswer{status: http.StatusOK, body: describeVpcsXMLBody}
	in := awsReadTestInput(doers.doer(), readEntry("ec2", "query", "DescribeVpcs", `{}`), readEntry("ssm", "json", "GetParameter", `{"Name":"/a"}`))
	in.Preflight = PreflightAWSReadKey

	_, err := ReadAWSAccount(context.Background(), in)

	assert.ErrorContains(t, err, "DeleteVpc answered DryRunOperation")
	assertEveryDryRunRowSentOnce(t, doers.ec2)
	assert.Empty(t, doers.ssm.sent)
	assert.Len(t, doers.sts.sent, 1)
}

func TestReadAWSAccountAnswers(t *testing.T) {
	t.Parallel()

	iamError := func(code string) string {
		return `<ErrorResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/"><Error><Type>Sender</Type><Code>` + code +
			`</Code><Message>m</Message></Error><RequestId>r-1</RequestId></ErrorResponse>`
	}
	cases := []struct {
		name    string
		entry   AWSReadEntry
		host    string
		answer  awsCannedAnswer
		wantErr []string // nil: recorded with the answer's status
	}{
		{"EC2 InvalidVpcID.NotFound is an answer", readEntry("ec2", "query", "DescribeVpcs", `{"VpcId.1":"vpc-0123456789abcdef0"}`), awsReadEC2Host,
			awsCannedAnswer{status: http.StatusBadRequest, body: ec2ErrorBody("InvalidVpcID.NotFound")}, nil},
		{"EC2 InvalidGroup.NotFound is an answer", readEntry("ec2", "query", "DescribeSecurityGroups", `{"GroupId.1":"sg-0123456789abcdef0"}`), awsReadEC2Host,
			awsCannedAnswer{status: http.StatusBadRequest, body: ec2ErrorBody("InvalidGroup.NotFound")}, nil},
		{"EC2 InvalidKeyPair.NotFound is an answer", readEntry("ec2", "query", "DescribeKeyPairs", `{"KeyName.1":"web"}`), awsReadEC2Host,
			awsCannedAnswer{status: http.StatusBadRequest, body: ec2ErrorBody("InvalidKeyPair.NotFound")}, nil},
		{"SSM ParameterNotFound is an answer", readEntry("ssm", "json", "GetParameter", `{"Name":"/app/missing"}`), awsReadSSMHost,
			awsCannedAnswer{status: http.StatusBadRequest, body: `{"__type":"ParameterNotFound","message":"Parameter /app/missing not found."}`}, nil},
		{"SSM ParameterNotFound with a namespace is an answer", readEntry("ssm", "json", "GetParameter", `{"Name":"/app/missing"}`), awsReadSSMHost,
			awsCannedAnswer{status: http.StatusBadRequest, body: `{"__type":"com.amazonaws.ssm#ParameterNotFound"}`}, nil},
		{"IAM NoSuchEntity is an answer", readEntry("iam", "query", "GetRole", `{"RoleName":"missing"}`), awsReadIAMHost,
			awsCannedAnswer{status: http.StatusNotFound, body: iamError("NoSuchEntity")}, nil},
		{"IAM AccessDenied fails", readEntry("iam", "query", "GetRole", `{"RoleName":"web"}`), awsReadIAMHost,
			awsCannedAnswer{status: http.StatusForbidden, body: iamError("AccessDenied")},
			[]string{`entry 0 (service "iam", protocol "query", action "GetRole")`, `403 "AccessDenied"`}},
		{"EC2 UnauthorizedOperation fails", readEntry("ec2", "query", "DescribeVpcs", `{}`), awsReadEC2Host,
			awsCannedAnswer{status: http.StatusForbidden, body: ec2UnauthorizedBody},
			[]string{`entry 0 (service "ec2", protocol "query", action "DescribeVpcs")`, `403 "UnauthorizedOperation"`}},
		{"another service's NotFound code fails", readEntry("ec2", "query", "DescribeVpcs", `{}`), awsReadEC2Host,
			awsCannedAnswer{status: http.StatusBadRequest, body: ec2ErrorBody("ParameterNotFound")},
			[]string{`entry 0 (service "ec2"`, `400 "ParameterNotFound"`}},
		{"a NotFound code on a 5xx fails", readEntry("ec2", "query", "DescribeVpcs", `{}`), awsReadEC2Host,
			awsCannedAnswer{status: http.StatusInternalServerError, body: ec2ErrorBody("InvalidVpcID.NotFound")},
			[]string{`entry 0 (service "ec2"`, `500 "InvalidVpcID.NotFound"`}},
		{"EC2 nextToken fails", readEntry("ec2", "query", "DescribeInstances", `{}`), awsReadEC2Host,
			awsCannedAnswer{status: http.StatusOK, body: `<DescribeInstancesResponse><reservationSet/><nextToken>page-2</nextToken></DescribeInstancesResponse>`},
			[]string{`entry 0 (service "ec2", protocol "query", action "DescribeInstances")`, "truncated"}},
		{"IAM IsTruncated=true fails", readEntry("iam", "query", "ListRolePolicies", `{"RoleName":"web"}`), awsReadIAMHost,
			awsCannedAnswer{status: http.StatusOK, body: `<ListRolePoliciesResponse><ListRolePoliciesResult><PolicyNames/><IsTruncated>true</IsTruncated><Marker>m</Marker></ListRolePoliciesResult></ListRolePoliciesResponse>`},
			[]string{`entry 0 (service "iam", protocol "query", action "ListRolePolicies")`, "truncated"}},
		{"SSM NextToken fails", readEntry("ssm", "json", "GetParameter", `{"Name":"/a"}`), awsReadSSMHost,
			awsCannedAnswer{status: http.StatusOK, body: `{"Parameter":{"Name":"/a"},"NextToken":"page-2"}`},
			[]string{`entry 0 (service "ssm", protocol "json", action "GetParameter")`, "truncated"}},
		{"an empty EC2 nextToken is the last page", readEntry("ec2", "query", "DescribeInstances", `{}`), awsReadEC2Host,
			awsCannedAnswer{status: http.StatusOK, body: `<DescribeInstancesResponse><reservationSet/><nextToken/></DescribeInstancesResponse>`}, nil},
		{"IAM IsTruncated=false is the last page", readEntry("iam", "query", "ListRolePolicies", `{"RoleName":"web"}`), awsReadIAMHost,
			awsCannedAnswer{status: http.StatusOK, body: `<ListRolePoliciesResponse><ListRolePoliciesResult><PolicyNames/><IsTruncated>false</IsTruncated></ListRolePoliciesResult></ListRolePoliciesResponse>`}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			service := &awsWireDoer{answer: tc.answer}
			doer := newAWSReadTestDoers().doer()
			doer[tc.host] = service

			read, err := ReadAWSAccount(context.Background(), awsReadTestInput(doer, tc.entry))

			require.Len(t, service.sent, 1)
			if tc.wantErr != nil {
				require.Error(t, err)
				for _, want := range tc.wantErr {
					assert.Contains(t, err.Error(), want)
				}
				return
			}
			require.NoError(t, err)
			require.Len(t, read.Exchanges, 1)
			assert.Equal(t, tc.answer.status, read.Exchanges[0].Status)
			assert.Equal(t, tc.answer.body, read.Exchanges[0].Body)
		})
	}
}

func TestReadAWSAccountCountsEveryCall(t *testing.T) {
	t.Parallel()

	doers := newAWSReadTestDoers()
	doers.ec2.answers["DescribeVpcs"] = awsCannedAnswer{status: http.StatusOK, body: describeVpcsXMLBody}
	doers.ec2.answers["DescribeSubnets"] = awsCannedAnswer{status: http.StatusOK, body: "<DescribeSubnetsResponse/>"}
	doers.ssm.answer.body = `{"Parameter":{"Name":"/a"}}`
	doers.iam.answer.body = "<GetRoleResponse/>"
	in := awsReadTestInput(doers.doer(),
		readEntry("ec2", "query", "DescribeVpcs", `{}`),
		readEntry("ec2", "query", "DescribeSubnets", `{"Filter.1.Name":"vpc-id","Filter.1.Value.1":"vpc-0123456789abcdef0"}`),
		readEntry("ssm", "json", "GetParameter", `{"Name":"/a"}`),
		readEntry("iam", "query", "GetRole", `{"RoleName":"web"}`),
		readEntry("iam", "query", "ListRolePolicies", `{"RoleName":"web"}`),
	)
	in.Preflight = PreflightAWSReadKey

	read, err := ReadAWSAccount(context.Background(), in)

	require.NoError(t, err)
	assert.Equal(t, doers.sent(), read.Calls)
	assert.Equal(t, 1+len(AWSReaderDryRunActions)+len(in.ReadSet), read.Calls, "identity, preflight and read set")
}

func TestReadAWSAccountTimesTheReadOnItsClock(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC)
	const advance = 842 * time.Millisecond
	now := start
	doers := newAWSReadTestDoers()
	doers.ssm.answer.body = `{"Parameter":{"Name":"/a"}}`
	doers.ssm.onSend = func() { now = now.Add(advance) }
	in := awsReadTestInput(doers.doer(), readEntry("ssm", "json", "GetParameter", `{"Name":"/a"}`))
	in.Now = func() time.Time { return now }

	read, err := ReadAWSAccount(context.Background(), in)

	require.NoError(t, err)
	assert.Equal(t, start, read.Timestamp)
	assert.Equal(t, advance.Milliseconds(), read.DurationMS)
}

// A fixed clock times the read; the requests are still signed now, or
// AWS would refuse them as expired.
func TestReadAWSAccountSignsAtTheRealTime(t *testing.T) {
	t.Parallel()

	doers := newAWSReadTestDoers()
	doers.ssm.answer.body = `{"Parameter":{"Name":"/a"}}`
	in := awsReadTestInput(doers.doer(), readEntry("ssm", "json", "GetParameter", `{"Name":"/a"}`))
	in.Now = func() time.Time { return time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC) }

	_, err := ReadAWSAccount(context.Background(), in)

	require.NoError(t, err)
	require.Len(t, doers.ssm.sent, 1)
	signed, err := time.Parse("20060102T150405Z", doers.ssm.sent[0].header.Get("X-Amz-Date"))
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), signed, time.Minute)
}

func TestAWSReadEndpointsCoverTheReaderTable(t *testing.T) {
	t.Parallel()

	endpoints := awsReadEndpoints("eu-west-1")
	for _, svc := range AWSReaderTable {
		endpoint, ok := endpoints[svc.Service]
		if !assert.True(t, ok, "reader-table service %s has no endpoint", svc.Service) {
			continue
		}
		assert.NotEmpty(t, endpoint.signingRegion, svc.Service)
		if svc.Protocol == awsProtocolQuery {
			assert.NotEmpty(t, endpoint.version, "query service %s has no API version", svc.Service)
		}
	}
	assert.Equal(t, awsReadEndpoint{url: "https://iam.amazonaws.com", signingRegion: "us-east-1", version: "2010-05-08"}, endpoints["iam"])
	assert.Len(t, endpoints, len(AWSReaderTable), "an endpoint for a service outside the reader table")
}
