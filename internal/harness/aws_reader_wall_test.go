package harness

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	describeVpcsXMLBody = `<DescribeVpcsResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><requestId>r-1</requestId><vpcSet/></DescribeVpcsResponse>`
	ec2ThrottleXMLBody  = `<Response><Errors><Error><Code>RequestLimitExceeded</Code><Message>Request limit exceeded.</Message></Error></Errors><RequestID>r-1</RequestID></Response>`
	stsThrottleXMLBody  = `<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><Error><Type>Sender</Type><Code>Throttling</Code><Message>Rate exceeded</Message></Error><RequestId>r-1</RequestId></ErrorResponse>`
)

// recordingAWSDoer answers every request with one canned response and
// records each body that reached it, so a test sees what went on the wire.
type recordingAWSDoer struct {
	status int
	body   string
	bodies [][]byte
}

func (d *recordingAWSDoer) Do(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		var err error
		if body, err = io.ReadAll(req.Body); err != nil {
			return nil, err
		}
	}
	d.bodies = append(d.bodies, body)
	status := d.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"text/xml"}},
		Body:       io.NopCloser(strings.NewReader(d.body)),
		Request:    req,
	}, nil
}

// awsWallRequest builds a request as the SDK sends it to service:
// contentType "" means the protocol's own media type.
type awsWallRequest struct {
	service, url, target, contentType, body string
	mutate                                  func(*http.Request)
}

func (r awsWallRequest) build(t *testing.T) *http.Request {
	t.Helper()
	url := r.url
	if url == "" {
		url = "https://aws.test.invalid/"
	}
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(r.body))
	require.NoError(t, err)
	contentType := r.contentType
	if svc, _ := awsReaderService(r.service); contentType == "" && svc.Protocol == awsProtocolJSON {
		contentType = awsJSONMediaType
	} else if contentType == "" {
		contentType = awsQueryMediaType
	}
	req.Header.Set("Content-Type", contentType)
	if r.target != "" {
		req.Header.Set("X-Amz-Target", r.target)
	}
	if r.mutate != nil {
		r.mutate(req)
	}
	return req
}

func TestAWSReadWallRefusesWithNothingSent(t *testing.T) {
	t.Parallel()

	const deleteVpc = "Action=DeleteVpc&VpcId=vpc-0123456789abcdef0&Version=2016-11-15"
	cases := []struct {
		name string
		req  awsWallRequest
	}{
		{"ec2 CreateVpc", awsWallRequest{service: "ec2", body: "Action=CreateVpc&CidrBlock=10.0.0.0%2F16&Version=2016-11-15"}},
		{"iam CreateAccessKey in the form body", awsWallRequest{service: "iam", body: "Action=CreateAccessKey&UserName=reader&Version=2010-05-08"}},
		{"sts AssumeRole", awsWallRequest{service: "sts", body: "Action=AssumeRole&RoleArn=arn%3Aaws%3Aiam%3A%3A123456789012%3Arole%2Fadmin&RoleSessionName=x&Version=2011-06-15"}},
		{"ssm PutParameter by X-Amz-Target", awsWallRequest{service: "ssm", target: "AmazonSSM.PutParameter", body: `{"Name":"/app/db/host","Value":"x","Overwrite":true}`}},
		{"query request with neither Action nor X-Amz-Target", awsWallRequest{service: "ec2", body: "Version=2016-11-15"}},
		{"json request with neither Action nor X-Amz-Target", awsWallRequest{service: "ssm", body: `{"Name":"/app/db/host"}`}},
		{"Action in the URL query", awsWallRequest{service: "ec2", url: "https://aws.test.invalid/?Action=CreateVpc", body: "Action=DescribeVpcs&Version=2016-11-15"}},
		{"Action in an opaque URL", awsWallRequest{service: "ec2", body: "Action=DescribeVpcs&Version=2016-11-15", mutate: func(r *http.Request) { r.URL.Opaque = "/?Action=CreateVpc" }}},
		{"two Action values", awsWallRequest{service: "ec2", body: "Action=DescribeVpcs&Action=CreateVpc&Version=2016-11-15"}},
		{"a case variant of Action", awsWallRequest{service: "ec2", body: "Action=DescribeVpcs&action=CreateVpc&Version=2016-11-15"}},
		{"DeleteVpc without DryRun", awsWallRequest{service: "ec2", body: deleteVpc}},
		{"DeleteVpc with DryRun=false", awsWallRequest{service: "ec2", body: deleteVpc + "&DryRun=false"}},
		{"DeleteVpc with DryRun=True", awsWallRequest{service: "ec2", body: deleteVpc + "&DryRun=True"}},
		{"DeleteVpc with DryRun=1", awsWallRequest{service: "ec2", body: deleteVpc + "&DryRun=1"}},
		{"DeleteVpc with DryRun=true&DryRun=false", awsWallRequest{service: "ec2", body: deleteVpc + "&DryRun=true&DryRun=false"}},
		{"DeleteVpc with a case variant of DryRun", awsWallRequest{service: "ec2", body: deleteVpc + "&DryRun=true&dryrun=false"}},
		{"a DryRun-table action on a service that is not EC2", awsWallRequest{service: "iam", body: "Action=DeleteVpc&DryRun=true&Version=2010-05-08"}},
		{"query request carrying X-Amz-Target", awsWallRequest{service: "ec2", target: "AmazonEC2.DescribeVpcs", body: "Action=DescribeVpcs&Version=2016-11-15"}},
		{"query request carrying X-Amz-Target under a non-canonical key", awsWallRequest{service: "ec2", body: "Action=DescribeVpcs&Version=2016-11-15", mutate: func(r *http.Request) { r.Header["x-amz-target"] = []string{"AmazonEC2.CreateVpc"} }}},
		{"json request with an Action field", awsWallRequest{service: "ssm", target: "AmazonSSM.GetParameter", body: `{"Name":"/app/db/host","Action":"PutParameter"}`}},
		{"json target with another service's prefix", awsWallRequest{service: "ssm", target: "AmazonEC2.GetParameter", body: `{"Name":"/app/db/host"}`}},
		{"json body that is not an object", awsWallRequest{service: "ssm", target: "AmazonSSM.GetParameter", body: `["Name"]`}},
		{"an encoded body", awsWallRequest{service: "ec2", body: "Action=DescribeVpcs&Version=2016-11-15", mutate: func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") }}},
		{"a multipart query request", awsWallRequest{service: "ec2", contentType: "multipart/form-data; boundary=wall", body: "Action=DescribeVpcs&Version=2016-11-15"}},
		{"a query request with no Content-Type", awsWallRequest{service: "ec2", body: "Action=DescribeVpcs&Version=2016-11-15", mutate: func(r *http.Request) { r.Header.Del("Content-Type") }}},
		{"two Content-Type values", awsWallRequest{service: "ec2", body: "Action=DescribeVpcs&Version=2016-11-15", mutate: func(r *http.Request) { r.Header["content-type"] = []string{awsQueryMediaType} }}},
		{"a Content-Type that does not parse", awsWallRequest{service: "ec2", contentType: "application/x-www-form-urlencoded; charset", body: "Action=DescribeVpcs&Version=2016-11-15"}},
		{"a json Content-Type on a query service", awsWallRequest{service: "ec2", contentType: awsJSONMediaType, body: "Action=DescribeVpcs&Version=2016-11-15"}},
		{"a query charset other than utf-8", awsWallRequest{service: "ec2", contentType: awsQueryMediaType + "; charset=utf-16", body: "Action=DescribeVpcs&Version=2016-11-15"}},
		{"a form Content-Type on the json service", awsWallRequest{service: "ssm", target: "AmazonSSM.GetParameter", contentType: awsQueryMediaType, body: `{"Name":"/app/db/host"}`}},
		{"a json Content-Type with a parameter", awsWallRequest{service: "ssm", target: "AmazonSSM.GetParameter", contentType: awsJSONMediaType + "; charset=utf-8", body: `{"Name":"/app/db/host"}`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inner := &recordingAWSDoer{}
			wall, err := NewAWSReadWall(tc.req.service, inner)
			require.NoError(t, err)

			resp, err := wall.Do(tc.req.build(t))
			if resp != nil {
				resp.Body.Close()
			}
			assert.ErrorIs(t, err, ErrAWSReadWallRefused)
			assert.Empty(t, inner.bodies, "a refused request reached the inner doer")
			assert.Zero(t, wall.Calls())
		})
	}
}

func TestAWSReadWallPassesReadsByteIdentical(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		req        awsWallRequest
		wantAction string
		wantParams string
	}{
		{"ec2 DescribeVpcs", awsWallRequest{service: "ec2", body: "Action=DescribeVpcs&Filter.1.Name=tag-key&Filter.1.Value.1=env&Version=2016-11-15"},
			"DescribeVpcs", `{"Filter.1.Name":"tag-key","Filter.1.Value.1":"env"}`},
		{"iam GetRole with charset=UTF-8", awsWallRequest{service: "iam", contentType: awsQueryMediaType + "; charset=UTF-8", body: "Action=GetRole&RoleName=web&Version=2010-05-08"},
			"GetRole", `{"RoleName":"web"}`},
		{"sts GetCallerIdentity", awsWallRequest{service: "sts", body: "Action=GetCallerIdentity&Version=2011-06-15"},
			"GetCallerIdentity", `{}`},
		{"ssm GetParameter", awsWallRequest{service: "ssm", target: "AmazonSSM.GetParameter", body: `{"Name": "/app/db/host", "WithDecryption": true}`},
			"GetParameter", `{"Name": "/app/db/host", "WithDecryption": true}`},
		{"ec2 DeleteVpc with DryRun=true", awsWallRequest{service: "ec2", body: "Action=DeleteVpc&VpcId=vpc-0123456789abcdef0&DryRun=true&Version=2016-11-15"},
			"DeleteVpc", `{"VpcId":"vpc-0123456789abcdef0","DryRun":"true"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inner := &recordingAWSDoer{}
			wall, err := NewAWSReadWall(tc.req.service, inner)
			require.NoError(t, err)

			req := tc.req.build(t)
			resp, err := wall.Do(req)
			require.NoError(t, err)
			resp.Body.Close()
			require.Len(t, inner.bodies, 1)
			assert.Equal(t, tc.req.body, string(inner.bodies[0]))
			assert.Equal(t, 1, wall.Calls())
			resend, err := req.GetBody()
			require.NoError(t, err)
			again, err := io.ReadAll(resend)
			require.NoError(t, err)
			assert.Equal(t, tc.req.body, string(again), "GetBody would resend other bytes")

			entry, _, err := wall.entryOf(tc.req.build(t))
			require.NoError(t, err)
			assert.Equal(t, tc.wantAction, entry.Action)
			assert.JSONEq(t, tc.wantParams, string(entry.Params))
			if wall.protocol == awsProtocolJSON {
				assert.Equal(t, tc.req.body, string(entry.Params), "json params are the body verbatim")
			}
		})
	}
}

func TestAWSReadWallCountsOnlyWhatItPasses(t *testing.T) {
	t.Parallel()

	inner := &recordingAWSDoer{}
	wall, err := NewAWSReadWall("ec2", inner)
	require.NoError(t, err)
	for _, body := range []string{"Action=DescribeVpcs", "Action=CreateVpc", "Action=DescribeSubnets"} {
		resp, _ := wall.Do(awsWallRequest{service: "ec2", body: body}.build(t))
		if resp != nil {
			resp.Body.Close()
		}
	}
	assert.Equal(t, 2, wall.Calls())
	assert.Len(t, inner.bodies, 2)
}

// Not parallel: it edits the package's reader and DryRun tables, and no
// parallel test runs until every sequential one has finished.
func TestAWSReadWallIgnoresLaterEditsToTheTables(t *testing.T) {
	savedTable := slices.Clone(AWSReaderTable)
	for i := range savedTable {
		savedTable[i].Actions = slices.Clone(savedTable[i].Actions)
	}
	savedDryRun := slices.Clone(AWSReaderDryRunActions)
	t.Cleanup(func() { AWSReaderTable, AWSReaderDryRunActions = savedTable, savedDryRun })

	inner := &recordingAWSDoer{}
	wall, err := NewAWSReadWall("ec2", inner)
	require.NoError(t, err)

	row := slices.IndexFunc(AWSReaderTable, func(s AWSReaderService) bool { return s.Service == "ec2" })
	require.GreaterOrEqual(t, row, 0)
	// In place, in the backing array the wall was built from, then appended.
	AWSReaderTable[row].Actions[0] = "CreateVpc"
	AWSReaderTable[row].Actions = append(AWSReaderTable[row].Actions, "CreateSubnet")
	AWSReaderDryRunActions = append(AWSReaderDryRunActions, "AuthorizeSecurityGroupIngress")

	for _, body := range []string{"Action=CreateVpc", "Action=CreateSubnet", "Action=AuthorizeSecurityGroupIngress&DryRun=true"} {
		resp, err := wall.Do(awsWallRequest{service: "ec2", body: body}.build(t))
		if resp != nil {
			resp.Body.Close()
		}
		assert.ErrorIs(t, err, ErrAWSReadWallRefused, body)
	}
	assert.Empty(t, inner.bodies)
}

func TestNewAWSReadWallRefusesAnUnknownServiceAndANilDoer(t *testing.T) {
	t.Parallel()

	_, err := NewAWSReadWall("lambda", &recordingAWSDoer{})
	assert.ErrorContains(t, err, `"lambda" is not a reader-table service`)
	_, err = NewAWSReadWall("ec2", nil)
	assert.ErrorContains(t, err, "needs an HTTP client")
}

func TestAWSReaderDryRunTableHoldsNoReaderRow(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, AWSReaderDryRunActions)
	assert.Contains(t, AWSReaderDryRunActions, "DeleteVpc")
	assert.Contains(t, AWSReaderDryRunActions, "TerminateInstances")
	for _, action := range AWSReaderDryRunActions {
		assert.False(t, strings.HasPrefix(action, "Describe"), "%s is a read, not a reap write", action)
		for _, svc := range AWSReaderTable {
			assert.NotContains(t, svc.Actions, action, "DryRun-table action %s is a %s reader-table row", action, svc.Service)
		}
	}
}

func TestValidateAWSReadSetNamesEveryEntryOutsideTheTable(t *testing.T) {
	t.Parallel()

	ok := []AWSReadEntry{
		{Service: "ec2", Protocol: "query", Action: "DescribeVpcs"},
		{Service: "ssm", Protocol: "json", Action: "GetParameter"},
	}
	require.NoError(t, ValidateAWSReadSet(ok))

	err := ValidateAWSReadSet(append(ok,
		AWSReadEntry{Service: "s3", Protocol: "rest", Action: "GET /s3/{bucket}/"},
		AWSReadEntry{Service: "ec2", Protocol: "query", Action: "CreateVpc"},
		AWSReadEntry{Service: "lambda", Protocol: "json", Action: "Invoke"},
		AWSReadEntry{Service: "ec2", Protocol: "query", Action: "DeleteVpc"},
		AWSReadEntry{Service: "ssm", Protocol: "query", Action: "GetParameter"},
	))
	require.Error(t, err)
	for _, want := range []string{
		`entry 2 (service "s3", protocol "rest", action "GET /s3/{bucket}/")`,
		`entry 3 (service "ec2", protocol "query", action "CreateVpc")`,
		`entry 4 (service "lambda", protocol "json", action "Invoke")`,
		`entry 5 (service "ec2", protocol "query", action "DeleteVpc")`,
		`entry 6 (service "ssm", protocol "query", action "GetParameter")`,
	} {
		assert.Contains(t, err.Error(), want)
	}
	assert.NotContains(t, err.Error(), "DescribeVpcs")
}

func TestDecodeAWSReadSetRefusesAnythingButOneArrayOfEntries(t *testing.T) {
	t.Parallel()

	const entry = `{"service":"ec2","protocol":"query","action":"DescribeVpcs","params":{}}`
	entries, err := DecodeAWSReadSet([]byte("[" + entry + "]\n"))
	require.NoError(t, err)
	assert.Equal(t, []AWSReadEntry{{Service: "ec2", Protocol: "query", Action: "DescribeVpcs", Params: json.RawMessage(`{}`)}}, entries)

	for name, data := range map[string]string{
		"an unknown field":       `[{"service":"ec2","protocol":"query","action":"DescribeVpcs","params":{},"region":"x"}]`,
		"an object, no array":    entry,
		"data after the array":   "[" + entry + "][]",
		"a second entry outside": "[" + entry + "]" + entry,
	} {
		_, err := DecodeAWSReadSet([]byte(data))
		assert.Error(t, err, name)
	}
}

func TestADR0041ExamplesParseIntoTheGoTypes(t *testing.T) {
	t.Parallel()

	adr, err := os.Open(filepath.Join("..", "..", "docs", "decisions", "0041-account-read-contract-and-recorded-read-format.md"))
	require.NoError(t, err)
	defer adr.Close()

	decode := func(raw string, v any) error {
		dec := json.NewDecoder(strings.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(v); err != nil {
			return err
		}
		if _, err := dec.Token(); !errors.Is(err, io.EOF) {
			return errors.New("trailing data after the example")
		}
		return nil
	}
	var section, block string
	inBlock := false
	readSets, recorded := 0, 0
	scanner := bufio.NewScanner(adr)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case !inBlock && strings.HasPrefix(line, "### "):
			section = line
		case !inBlock && line == "```json":
			inBlock, block = true, ""
		case inBlock && line == "```":
			inBlock = false
			switch section {
			case "### Read-set entry":
				readSets++
				if strings.HasPrefix(strings.TrimSpace(block), "[") {
					assert.NoError(t, decode(block, &[]AWSReadEntry{}), block)
				} else {
					assert.NoError(t, decode(block, &AWSReadEntry{}), block)
				}
			case "### Recorded read":
				recorded++
				var read AWSRecordedRead
				if assert.NoError(t, decode(block, &read), block) {
					assert.NotEmpty(t, read.Exchanges)
				}
			default:
				t.Errorf("a json example under %q has no Go type", section)
			}
		case inBlock:
			block += line + "\n"
		}
	}
	require.NoError(t, scanner.Err())
	assert.NotZero(t, readSets, "found no read-set example")
	assert.NotZero(t, recorded, "found no recorded-read example")
}

// awsReadTestConfig is a caller's aws.Config that would reach trap and
// retry five times if a walled client kept either.
func awsReadTestConfig(trap aws.HTTPClient) aws.Config {
	return aws.Config{
		Region:           "eu-west-1",
		Credentials:      credentials.NewStaticCredentialsProvider(testAWSKeyID, testAWSSecret, ""),
		HTTPClient:       trap,
		Retryer:          func() aws.Retryer { return retry.NewStandard() },
		RetryMaxAttempts: 5,
	}
}

func TestAWSReadClientsSendOnlyThroughTheirWall(t *testing.T) {
	t.Parallel()

	type call func(context.Context) error
	cases := []struct {
		service        string
		okBody         string
		throttleStatus int
		throttleBody   string
		throttleCode   string
		build          func(aws.Config, *AWSReadWall) (call, error)
	}{
		{"ec2", describeVpcsXMLBody, http.StatusServiceUnavailable, ec2ThrottleXMLBody, "RequestLimitExceeded",
			func(cfg aws.Config, wall *AWSReadWall) (call, error) {
				client, err := newAWSReadEC2Client(cfg, wall)
				if err != nil {
					return nil, err
				}
				return func(ctx context.Context) error {
					_, err := client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{})
					return err
				}, nil
			}},
		{"sts", getCallerIdentityXMLBody, http.StatusBadRequest, stsThrottleXMLBody, "Throttling",
			func(cfg aws.Config, wall *AWSReadWall) (call, error) {
				client, err := newAWSReadSTSClient(cfg, wall)
				if err != nil {
					return nil, err
				}
				return func(ctx context.Context) error {
					_, err := client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
					return err
				}, nil
			}},
	}
	for _, tc := range cases {
		t.Run(tc.service+" sends one call through the wall", func(t *testing.T) {
			t.Parallel()
			inner, trap := &recordingAWSDoer{body: tc.okBody}, &recordingAWSDoer{body: tc.okBody}
			wall, err := NewAWSReadWall(tc.service, inner)
			require.NoError(t, err)
			send, err := tc.build(awsReadTestConfig(trap), wall)
			require.NoError(t, err)

			require.NoError(t, send(context.Background()))
			assert.Len(t, inner.bodies, 1)
			assert.Equal(t, 1, wall.Calls())
			assert.Empty(t, trap.bodies, "the caller's own HTTP client was used")
		})
		t.Run(tc.service+" does not retry a throttle", func(t *testing.T) {
			t.Parallel()
			inner := &recordingAWSDoer{status: tc.throttleStatus, body: tc.throttleBody}
			wall, err := NewAWSReadWall(tc.service, inner)
			require.NoError(t, err)
			send, err := tc.build(awsReadTestConfig(&recordingAWSDoer{}), wall)
			require.NoError(t, err)

			err = send(context.Background())
			var apiErr smithy.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tc.throttleCode, apiErr.ErrorCode())
			assert.Len(t, inner.bodies, 1, "the SDK retried")
			assert.Equal(t, 1, wall.Calls())
		})
		t.Run(tc.service+" refuses a nil or another service's wall", func(t *testing.T) {
			t.Parallel()
			_, err := tc.build(awsReadTestConfig(&recordingAWSDoer{}), nil)
			assert.ErrorContains(t, err, "needs a read wall")
			other, err := NewAWSReadWall("iam", &recordingAWSDoer{})
			require.NoError(t, err)
			_, err = tc.build(awsReadTestConfig(&recordingAWSDoer{}), other)
			assert.ErrorContains(t, err, "the wall is bound to iam")
		})
	}
}

const awsReadClientsFile = "aws_reader_clients.go"

// awsReadUnwalled names what the account read must not reach: the
// sweep's and the reap's unwalled constructors, identity check and doers.
var awsReadUnwalled = map[string]bool{
	"newAWSEC2Client": true, "newAWSSSMClient": true, "NewAWSSTSClient": true,
	"VerifyAWSIdentity": true, "AWSDoers": true,
}

// awsReadSeamViolations names each way the read file at path steps around
// the walled clients: a reference to an unwalled name, an SDK client
// built outside aws_reader_clients.go, or, in that file, an SDK New or a
// NewFromConfig whose options do not set o.HTTPClient to its wall.
func awsReadSeamViolations(path string, file *ast.File) []string {
	sdk := map[string]bool{}
	for _, imp := range file.Imports {
		importPath, _ := strconv.Unquote(imp.Path.Value)
		if !strings.HasPrefix(importPath, "github.com/aws/aws-sdk-go-v2/service/") {
			continue
		}
		name := pathpkg.Base(importPath)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		sdk[name] = true
	}
	inClients := path == awsReadClientsFile
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Ident:
			if awsReadUnwalled[n.Name] {
				out = append(out, "references "+n.Name)
			}
		case *ast.SelectorExpr:
			x, _ := n.X.(*ast.Ident)
			sdkNew := n.Sel.Name == "New" && x != nil && sdk[x.Name]
			if sdkNew || (n.Sel.Name == "NewFromConfig" && !inClients) {
				out = append(out, "builds an SDK client with "+n.Sel.Name)
			}
		}
		return true
	})
	if inClients {
		out = append(out, awsReadOptionsViolations(file)...)
	}
	return out
}

// awsReadOptionsViolations names each NewFromConfig call whose options
// are not all func literals setting o.HTTPClient to the enclosing func's
// *AWSReadWall parameter, and never to anything else.
func awsReadOptionsViolations(file *ast.File) []string {
	var out []string
	for _, decl := range file.Decls {
		where, wall := "package scope", ""
		if fn, ok := decl.(*ast.FuncDecl); ok {
			where, wall = fn.Name.Name, awsReadWallParam(fn)
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "NewFromConfig" {
				return true
			}
			if len(call.Args) < 2 {
				out = append(out, where+": NewFromConfig has no options func")
			}
			for _, arg := range call.Args[min(1, len(call.Args)):] {
				if !setsHTTPClientToWall(arg, wall) {
					out = append(out, where+": an options func does not set o.HTTPClient to its wall")
				}
			}
			return true
		})
	}
	return out
}

func awsReadWallParam(fn *ast.FuncDecl) string {
	for _, field := range fn.Type.Params.List {
		star, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if ident, ok := star.X.(*ast.Ident); ok && ident.Name == "AWSReadWall" && len(field.Names) == 1 {
			return field.Names[0].Name
		}
	}
	return ""
}

func setsHTTPClientToWall(arg ast.Expr, wall string) bool {
	lit, ok := arg.(*ast.FuncLit)
	if !ok || wall == "" || len(lit.Type.Params.List) != 1 || len(lit.Type.Params.List[0].Names) != 1 {
		return false
	}
	opts := lit.Type.Params.List[0].Names[0].Name
	set, onlyWall := false, true
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			sel, ok := lhs.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "HTTPClient" {
				continue
			}
			if len(assign.Rhs) != len(assign.Lhs) || !isIdentNamed(assign.Rhs[i], wall) {
				onlyWall = false
			} else if isIdentNamed(sel.X, opts) {
				set = true
			}
		}
		return true
	})
	return set && onlyWall
}

func isIdentNamed(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

func TestAWSReadSeamUsesOnlyTheWalledClients(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob("aws_reader*.go")
	require.NoError(t, err)
	cliPaths, err := filepath.Glob(filepath.Join("..", "cli", "rehearse_read*.go"))
	require.NoError(t, err)

	var violations []string
	sawClients := false
	for _, path := range append(paths, cliPaths...) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		sawClients = sawClients || path == awsReadClientsFile
		for _, v := range awsReadSeamViolations(path, file) {
			violations = append(violations, path+": "+v)
		}
	}
	require.True(t, sawClients, "the audit did not find %s; it is looking in the wrong place", awsReadClientsFile)
	assert.Empty(t, violations, "the account read must build its SDK clients only in %s, each sending through its wall", awsReadClientsFile)
}

func TestAWSReadSeamAuditFixtures(t *testing.T) {
	t.Parallel()

	const walled = `package harness
import ("github.com/aws/aws-sdk-go-v2/aws"; "github.com/aws/aws-sdk-go-v2/service/ec2")
func c(cfg aws.Config, wall *AWSReadWall) *ec2.Client {
	return ec2.NewFromConfig(cfg, func(o *ec2.Options) { o.HTTPClient = wall; o.Retryer = aws.NopRetryer{} })
}`
	cases := []struct {
		name, path, src string
		want            []string
	}{
		{"walled client", awsReadClientsFile, walled, nil},
		{"the same client outside the clients file", "aws_reader.go", walled, []string{"builds an SDK client with NewFromConfig"}},
		{"unwalled constructor", "aws_reader.go", `package harness
func f() { newAWSEC2Client(nil, nil, "") }`, []string{"references newAWSEC2Client"}},
		{"identity check and doers from the CLI", "../cli/rehearse_read.go", `package cli
import "github.com/redscaresu/infrafactory/internal/harness"
var _ harness.AWSDoers
var _ = harness.VerifyAWSIdentity`, []string{"references AWSDoers", "references VerifyAWSIdentity"}},
		{"SDK New", "aws_reader.go", `package harness
import awsssm "github.com/aws/aws-sdk-go-v2/service/ssm"
var _ = awsssm.New(awsssm.Options{})`, []string{"builds an SDK client with New"}},
		{"SDK New in the clients file", awsReadClientsFile, `package harness
import "github.com/aws/aws-sdk-go-v2/service/ec2"
func c(wall *AWSReadWall) *ec2.Client { return ec2.New(ec2.Options{HTTPClient: wall}) }`, []string{"builds an SDK client with New"}},
		{"errors.New is not a client", "aws_reader.go", `package harness
import "errors"
var _ = errors.New("x")`, nil},
		{"options func without HTTPClient", awsReadClientsFile, `package harness
func c(cfg aws.Config, wall *AWSReadWall) { ec2.NewFromConfig(cfg, func(o *ec2.Options) { o.Retryer = nil }) }`,
			[]string{"c: an options func does not set o.HTTPClient to its wall"}},
		{"HTTPClient set to the wall, then to another doer", awsReadClientsFile, `package harness
func c(cfg aws.Config, wall *AWSReadWall, other ec2.HTTPClient) {
	ec2.NewFromConfig(cfg, func(o *ec2.Options) { o.HTTPClient = wall; o.HTTPClient = other })
}`, []string{"c: an options func does not set o.HTTPClient to its wall"}},
		{"no options func", awsReadClientsFile, `package harness
func c(cfg aws.Config, wall *AWSReadWall) { ec2.NewFromConfig(cfg) }`, []string{"c: NewFromConfig has no options func"}},
		{"no wall parameter", awsReadClientsFile, `package harness
func c(cfg aws.Config, wall ec2.HTTPClient) { ec2.NewFromConfig(cfg, func(o *ec2.Options) { o.HTTPClient = wall }) }`,
			[]string{"c: an options func does not set o.HTTPClient to its wall"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", tc.src, parser.SkipObjectResolution)
			require.NoError(t, err)
			assert.Equal(t, tc.want, awsReadSeamViolations(tc.path, file))
		})
	}
}
