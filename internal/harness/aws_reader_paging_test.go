package harness

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// awsReadPagesDir holds the paged answers, one file per page, named
// <service>.<action>.<page>.<xml|json>.
const awsReadPagesDir = "testdata/aws_reader_pages"

// The tokens the page fixtures carry, each asking for the page after.
const (
	ec2PageToken2  = "eyJ2IjoxLCJjIjoiaW5zdGFuY2VzIiwicCI6Mn0="
	ec2PageToken3  = "eyJ2IjoxLCJjIjoiaW5zdGFuY2VzIiwicCI6M30="
	iamPageMarker2 = "AAEAAUdyb3VwUG9saWN5TWFya2VyMg=="
)

func awsPageFixture(t *testing.T, service, action string, page int) string {
	t.Helper()
	ext := "xml"
	if svc, _ := awsReaderService(service); svc.Protocol == awsProtocolJSON {
		ext = "json"
	}
	data, err := os.ReadFile(filepath.Join(awsReadPagesDir, fmt.Sprintf("%s.%s.%d.%s", service, action, page, ext)))
	require.NoError(t, err)
	return string(data)
}

func awsOK(body string) awsCannedAnswer {
	return awsCannedAnswer{status: http.StatusOK, body: body}
}

// awsQueryErrorBody is an IAM or STS error answer, from namespace.
func awsQueryErrorBody(namespace, code string) string {
	return `<ErrorResponse xmlns="` + namespace + `"><Error><Type>Sender</Type><Code>` + code +
		`</Code><Message>Rate exceeded</Message></Error><RequestId>r-1</RequestId></ErrorResponse>`
}

// awsReadHosts is the host each reader-table service's requests reach in
// awsReadTestConfig's region.
var awsReadHosts = map[string]string{"ec2": awsReadEC2Host, "iam": awsReadIAMHost, "ssm": awsReadSSMHost, "sts": awsReadSTSHost}

// awsPagingDoers is a recording doer per reader-table service; sts
// answers with the key's identity once its queue is spent.
type awsPagingDoers map[string]*awsWireDoer

func newAWSPagingDoers() awsPagingDoers {
	d := awsPagingDoers{}
	for service := range awsReadHosts {
		d[service] = &awsWireDoer{}
	}
	d["sts"].answer = awsOK(getCallerIdentityXMLBody)
	return d
}

func (d awsPagingDoers) doer() awsHostDoer {
	hosts := awsHostDoer{}
	for service, doer := range d {
		hosts[awsReadHosts[service]] = doer
	}
	return hosts
}

// sent is the number of requests that reached every doer.
func (d awsPagingDoers) sent() int {
	n := 0
	for _, doer := range d {
		n += len(doer.sent)
	}
	return n
}

// awsFakeClock is the read's clock in a test: a sleep returns at once,
// advancing it.
type awsFakeClock struct {
	now   time.Time
	slept []time.Duration
}

func (c *awsFakeClock) Now() time.Time { return c.now }

func (c *awsFakeClock) Sleep(ctx context.Context, d time.Duration) error {
	c.slept = append(c.slept, d)
	c.now = c.now.Add(d)
	return ctx.Err()
}

// sendsNoIdentityCheck passes the identity check with nothing sent, so a
// read's calls are its read set's alone.
func sendsNoIdentityCheck(context.Context, aws.Config, *AWSReadWall, string, string) error {
	return nil
}

func TestReadAWSAccountRecordsEveryPageAsItsOwnExchange(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		entry  AWSReadEntry
		field  string
		tokens []string // what each page after the first is asked for by
	}{
		{"a three-page EC2 list", readEntry("ec2", "query", "DescribeInstances", `{"Filter.1.Name": "vpc-id", "Filter.1.Value.1": "vpc-0123456789abcdef0"}`),
			"NextToken", []string{ec2PageToken2, ec2PageToken3}},
		{"a two-page IAM list", readEntry("iam", "query", "ListRolePolicies", `{"RoleName": "web"}`),
			"Marker", []string{iamPageMarker2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doers := newAWSPagingDoers()
			service := doers[tc.entry.Service]
			for page := 1; page <= len(tc.tokens)+1; page++ {
				service.queue = append(service.queue, awsOK(awsPageFixture(t, tc.entry.Service, tc.entry.Action, page)))
			}
			followUps := tc.entry.Service == "ec2"
			service.answer = awsCannedAnswer{status: http.StatusInternalServerError}
			if followUps {
				// The EC2 pages list an instance in a VPC, whose
				// follow-ups (readEC2) come after them; none of those
				// asks for a page.
				service.answer = awsOK("<Response/>")
			}

			read, err := ReadAWSAccount(context.Background(), awsReadTestInput(doers.doer(), tc.entry))

			require.NoError(t, err)
			assert.Equal(t, doers.sent(), read.Calls)
			if followUps {
				require.GreaterOrEqual(t, len(service.sent), len(service.queue))
				require.GreaterOrEqual(t, len(read.Exchanges), len(service.queue))
				for _, req := range service.sent[len(service.queue):] {
					assert.NotContains(t, req.form(t), tc.field, "a page past the last was asked for")
				}
			} else {
				assert.Equal(t, 1+len(service.queue), read.Calls, "identity and each page")
				require.Len(t, service.sent, len(service.queue))
				require.Len(t, read.Exchanges, len(service.queue))
			}
			assert.Equal(t, tc.entry, read.Exchanges[0].AWSReadEntry, "the first page is the entry verbatim")
			var base map[string]string
			require.NoError(t, json.Unmarshal(tc.entry.Params, &base))
			for i, exchange := range read.Exchanges[:len(service.queue)] {
				want := maps.Clone(base)
				if i > 0 {
					want[tc.field] = tc.tokens[i-1]
				}
				var params map[string]string
				require.NoError(t, json.Unmarshal(exchange.Params, &params))
				assert.Equal(t, want, params, "page %d's params", i+1)
				assert.Equal(t, service.queue[i].body, exchange.Body, "page %d's body", i+1)
				assert.Equal(t, http.StatusOK, exchange.Status)

				form := service.sent[i].form(t)
				assert.Equal(t, tc.entry.Action, form.Get("Action"))
				form.Del("Action")
				form.Del("Version")
				wire := map[string]string{}
				for key := range form {
					wire[key] = form.Get(key)
				}
				assert.Equal(t, want, wire, "page %d on the wire", i+1)
			}
		})
	}
}

func TestReadAWSAccountSendsAThrottledPageAgainWithItsToken(t *testing.T) {
	t.Parallel()

	entry := readEntry("iam", "query", "ListRolePolicies", `{"RoleName":"web"}`)
	doers := newAWSPagingDoers()
	doers["iam"].queue = []awsCannedAnswer{
		awsOK(awsPageFixture(t, "iam", "ListRolePolicies", 1)),
		{status: http.StatusBadRequest, body: awsQueryErrorBody("https://iam.amazonaws.com/doc/2010-05-08/", "Throttling")},
		awsOK(awsPageFixture(t, "iam", "ListRolePolicies", 2)),
	}
	doers["iam"].answer = awsCannedAnswer{status: http.StatusInternalServerError}
	clock := &awsFakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
	in := awsReadTestInput(doers.doer(), entry)
	in.Now, in.Sleep = clock.Now, clock.Sleep

	read, err := ReadAWSAccount(context.Background(), in)

	require.NoError(t, err)
	assert.Equal(t, doers.sent(), read.Calls)
	require.Len(t, read.Exchanges, 2, "a throttled answer is not a page")
	require.Len(t, doers["iam"].sent, 3)
	for _, sent := range doers["iam"].sent[1:] {
		assert.Equal(t, iamPageMarker2, sent.form(t).Get("Marker"))
	}
	assert.Equal(t, doers["iam"].queue[2].body, read.Exchanges[1].Body)
	assert.Equal(t, []time.Duration{time.Second}, clock.slept)
}

func TestReadAWSAccountFailsAListItCannotReadInFull(t *testing.T) {
	t.Parallel()

	describeInstances := readEntry("ec2", "query", "DescribeInstances", `{}`)
	listRolePolicies := readEntry("iam", "query", "ListRolePolicies", `{"RoleName":"web"}`)
	ec2Page := func(page int) string { return awsPageFixture(t, "ec2", "DescribeInstances", page) }
	iamPage := func(page int) string { return awsPageFixture(t, "iam", "ListRolePolicies", page) }
	replace := func(body, old, new string) string {
		require.Contains(t, body, old)
		return strings.Replace(body, old, new, 1)
	}
	oks := func(bodies ...string) []awsCannedAnswer {
		var answers []awsCannedAnswer
		for _, body := range bodies {
			answers = append(answers, awsOK(body))
		}
		return answers
	}
	var endless []awsCannedAnswer
	for page := range awsReadMaxPages + 1 {
		endless = append(endless, awsOK(replace(ec2Page(1), ec2PageToken2, fmt.Sprintf("page-%d", page+2))))
	}
	iamNoSuchEntity := awsCannedAnswer{status: http.StatusNotFound, body: awsQueryErrorBody("https://iam.amazonaws.com/doc/2010-05-08/", "NoSuchEntity")}
	ec2NotFound := awsCannedAnswer{status: http.StatusBadRequest, body: ec2ErrorBody("InvalidInstanceID.NotFound")}
	cases := []struct {
		name    string
		entry   AWSReadEntry
		answers []awsCannedAnswer
		sends   int // 0: every answer
		wantErr string
	}{
		{"a repeated nextToken", describeInstances, oks(ec2Page(1), ec2Page(2), ec2Page(1)), 0,
			fmt.Sprintf("NextToken %q came back a second time", ec2PageToken2)},
		{"IsTruncated=true without a Marker", listRolePolicies,
			oks(replace(iamPage(1), "<Marker>"+iamPageMarker2+"</Marker>", "")), 0,
			"IsTruncated is true with 0 Markers, not one"},
		{"an SSM NextToken", readEntry("ssm", "json", "GetParameter", `{"Name":"/app/db/host"}`),
			oks(awsPageFixture(t, "ssm", "GetParameter", 1)), 0,
			"the answer is truncated: it carries a NextToken, and ssm pages no list"},
		{"a nextToken on IAM", listRolePolicies,
			oks(replace(iamPage(2), "</ListRolePoliciesResult>", "<nextToken>t</nextToken></ListRolePoliciesResult>")), 0,
			"it carries a nextToken, and iam does not page by one"},
		{"IsTruncated=true on EC2", describeInstances,
			oks(replace(ec2Page(3), "</DescribeInstancesResponse>", "<IsTruncated>true</IsTruncated></DescribeInstancesResponse>")), 0,
			"IsTruncated is true, and ec2 does not page by a Marker"},
		{"an IsTruncated neither true nor false", listRolePolicies,
			oks(replace(iamPage(2), "<IsTruncated>false</IsTruncated>", "<IsTruncated>yes</IsTruncated>")), 0,
			`IsTruncated is "yes", not true or false`},
		{"a list past the page bound", describeInstances, endless, awsReadMaxPages,
			fmt.Sprintf("the list runs past %d pages", awsReadMaxPages)},
		{"IAM NoSuchEntity on page 2", listRolePolicies, append(oks(iamPage(1)), iamNoSuchEntity), 0,
			`page 2: answered 404 "NoSuchEntity", so the list is incomplete`},
		{"EC2 NotFound on page 2", describeInstances, append(oks(ec2Page(1)), ec2NotFound), 0,
			`page 2: answered 400 "InvalidInstanceID.NotFound", so the list is incomplete`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doers := newAWSPagingDoers()
			service := doers[tc.entry.Service]
			service.queue = tc.answers
			service.answer = awsCannedAnswer{status: http.StatusInternalServerError}

			read, err := ReadAWSAccount(context.Background(), awsReadTestInput(doers.doer(), tc.entry))

			require.Error(t, err)
			assert.Contains(t, err.Error(), awsReadEntryName(0, tc.entry))
			assert.Contains(t, err.Error(), tc.wantErr)
			sends := cmp.Or(tc.sends, len(tc.answers))
			assert.Len(t, service.sent, sends)
			assert.Equal(t, 1+sends, doers.sent(), "identity and each request sent")
			assert.Empty(t, read.Exchanges)
		})
	}
}

// A page fixture's name is its reader-table row: a test cannot page an
// action the read would never send.
func TestAWSReaderPageFixturesAreReaderTableRows(t *testing.T) {
	t.Parallel()

	files, err := os.ReadDir(awsReadPagesDir)
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, file := range files {
		name := strings.Split(file.Name(), ".")
		if !assert.Len(t, name, 4, "%s is not <service>.<action>.<page>.<ext>", file.Name()) {
			continue
		}
		svc, _ := awsReaderService(name[0])
		assert.True(t, awsReaderTableHas(AWSReadEntry{Service: name[0], Protocol: svc.Protocol, Action: name[1]}),
			"%s pages %s %s, which is not a reader-table row", file.Name(), name[0], name[1])
	}
}

func TestReadAWSAccountRetriesAThrottledRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		entry     AWSReadEntry
		throttled awsCannedAnswer
		body      string
	}{
		{"EC2 RequestLimitExceeded", readEntry("ec2", "query", "DescribeVpcs", `{}`),
			awsCannedAnswer{status: http.StatusServiceUnavailable, body: ec2ErrorBody("RequestLimitExceeded")}, describeVpcsXMLBody},
		{"SSM ThrottlingException", readEntry("ssm", "json", "GetParameter", `{"Name":"/a"}`),
			awsCannedAnswer{status: http.StatusBadRequest, body: `{"__type":"ThrottlingException","message":"Rate exceeded"}`}, `{"Parameter":{"Name":"/a"}}`},
		{"IAM Throttling", readEntry("iam", "query", "ListRolePolicies", `{"RoleName":"web"}`),
			awsCannedAnswer{status: http.StatusBadRequest, body: awsQueryErrorBody("https://iam.amazonaws.com/doc/2010-05-08/", "Throttling")}, awsPageFixture(t, "iam", "ListRolePolicies", 2)},
		{"STS Throttling", readEntry("sts", "query", "GetCallerIdentity", `{}`),
			awsCannedAnswer{status: http.StatusBadRequest, body: awsQueryErrorBody("https://sts.amazonaws.com/doc/2011-06-15/", "Throttling")}, getCallerIdentityXMLBody},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doers := newAWSPagingDoers()
			doers[tc.entry.Service].queue = []awsCannedAnswer{tc.throttled, awsOK(tc.body)}
			clock := &awsFakeClock{now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
			in := awsReadTestInput(doers.doer(), tc.entry)
			in.Identity = sendsNoIdentityCheck
			in.Now, in.Sleep = clock.Now, clock.Sleep

			read, err := ReadAWSAccount(context.Background(), in)

			require.NoError(t, err)
			assert.Equal(t, 2, read.Calls)
			assert.Equal(t, doers.sent(), read.Calls)
			require.Len(t, read.Exchanges, 1)
			assert.Equal(t, tc.body, read.Exchanges[0].Body)
			assert.Equal(t, []time.Duration{time.Second}, clock.slept)
			assert.Equal(t, time.Second.Milliseconds(), read.DurationMS, "the backoff is on the read's clock")
		})
	}
}

func TestReadAWSAccountDoesNotRetryAnUnnamedCode(t *testing.T) {
	t.Parallel()

	for _, code := range []string{"SlowDown", "Throttling"} {
		t.Run(code, func(t *testing.T) {
			t.Parallel()
			entry := readEntry("ec2", "query", "DescribeVpcs", `{}`)
			doers := newAWSPagingDoers()
			doers["ec2"].answer = awsCannedAnswer{status: http.StatusServiceUnavailable, body: ec2ErrorBody(code)}
			clock := &awsFakeClock{}
			in := awsReadTestInput(doers.doer(), entry)
			in.Identity = sendsNoIdentityCheck
			in.Now, in.Sleep = clock.Now, clock.Sleep

			read, err := ReadAWSAccount(context.Background(), in)

			require.Error(t, err)
			assert.Contains(t, err.Error(), awsReadEntryName(0, entry))
			assert.Contains(t, err.Error(), fmt.Sprintf("503 %q", code))
			assert.Equal(t, 1, doers.sent())
			assert.Empty(t, clock.slept)
			assert.Empty(t, read.Exchanges)
		})
	}
}

func TestReadAWSAccountGivesUpOnAThrottleThatNeverEnds(t *testing.T) {
	t.Parallel()

	entry := readEntry("ec2", "query", "DescribeVpcs", `{}`)
	doers := newAWSPagingDoers()
	doers["ec2"].answer = awsCannedAnswer{status: http.StatusServiceUnavailable, body: ec2ErrorBody("RequestLimitExceeded")}
	clock := &awsFakeClock{}
	in := awsReadTestInput(doers.doer(), entry)
	in.Identity = sendsNoIdentityCheck
	in.Now, in.Sleep = clock.Now, clock.Sleep
	started := time.Now()

	read, err := ReadAWSAccount(context.Background(), in)

	assert.Less(t, time.Since(started), 5*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), awsReadEntryName(0, entry))
	assert.Contains(t, err.Error(), `throttled: answered 503 "RequestLimitExceeded" to each of 5 attempts`)
	assert.Equal(t, 5, doers.sent())
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}, clock.slept)
	assert.Empty(t, read.Exchanges)
}
