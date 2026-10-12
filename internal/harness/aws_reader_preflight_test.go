package harness

import (
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// awsCannedAnswer is one response, or a transport error, for a request.
type awsCannedAnswer struct {
	status int
	body   string
	err    error
}

// dryRunEC2Doer records each request's form body and answers it by its
// Action from answers, UnauthorizedOperation by default.
type dryRunEC2Doer struct {
	answers map[string]awsCannedAnswer
	forms   []url.Values
}

func (d *dryRunEC2Doer) Do(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	d.forms = append(d.forms, form)
	answer, ok := d.answers[form.Get("Action")]
	if !ok {
		answer = awsCannedAnswer{status: http.StatusForbidden, body: ec2UnauthorizedBody}
	}
	if answer.err != nil {
		return nil, answer.err
	}
	return &http.Response{
		StatusCode: answer.status,
		Header:     http.Header{"Content-Type": {"text/xml"}},
		Body:       io.NopCloser(strings.NewReader(answer.body)),
		Request:    req,
	}, nil
}

func (d *dryRunEC2Doer) actions() []string {
	var actions []string
	for _, form := range d.forms {
		actions = append(actions, form.Get("Action"))
	}
	return actions
}

func ec2ErrorBody(code string) string {
	return `<Response><Errors><Error><Code>` + code + `</Code><Message>m</Message></Error></Errors>` +
		`<RequestID>00000000-0000-0000-0000-000000000000</RequestID></Response>`
}

func placeholderIDPattern(prefix string) string {
	return "^" + regexp.QuoteMeta(prefix) + "-[0-9a-f]{17}$"
}

// awsDryRunWireParams is, for each DryRun-table action, the wire field
// that names what it acts on and the pattern its value must match.
var awsDryRunWireParams = map[string]map[string]string{
	"TerminateInstances":         {"InstanceId.1": placeholderIDPattern("i")},
	"DeleteNatGateway":           {"NatGatewayId": placeholderIDPattern("nat")},
	"DisassociateAddress":        {"AssociationId": placeholderIDPattern("eipassoc")},
	"ReleaseAddress":             {"AllocationId": placeholderIDPattern("eipalloc")},
	"DeleteLaunchTemplate":       {"LaunchTemplateId": placeholderIDPattern("lt")},
	"DeregisterImage":            {"ImageId": placeholderIDPattern("ami")},
	"DeleteSnapshot":             {"SnapshotId": placeholderIDPattern("snap")},
	"DeleteVolume":               {"VolumeId": placeholderIDPattern("vol")},
	"DetachNetworkInterface":     {"AttachmentId": placeholderIDPattern("eni-attach")},
	"DeleteNetworkInterface":     {"NetworkInterfaceId": placeholderIDPattern("eni")},
	"DeleteKeyPair":              {"KeyPairId": placeholderIDPattern("key")},
	"RevokeSecurityGroupIngress": {"GroupId": placeholderIDPattern("sg"), "IpPermissions.1.IpProtocol": "^tcp$", "IpPermissions.1.IpRanges.1.CidrIp": `^192\.0\.2\.0/24$`},
	"RevokeSecurityGroupEgress":  {"GroupId": placeholderIDPattern("sg"), "IpPermissions.1.IpProtocol": "^tcp$", "IpPermissions.1.IpRanges.1.CidrIp": `^192\.0\.2\.0/24$`},
	"DeleteSecurityGroup":        {"GroupId": placeholderIDPattern("sg")},
	"DisassociateRouteTable":     {"AssociationId": placeholderIDPattern("rtbassoc")},
	"DeleteRouteTable":           {"RouteTableId": placeholderIDPattern("rtb")},
	"DeleteSubnet":               {"SubnetId": placeholderIDPattern("subnet")},
	"DetachInternetGateway":      {"InternetGatewayId": placeholderIDPattern("igw"), "VpcId": placeholderIDPattern("vpc")},
	"DeleteInternetGateway":      {"InternetGatewayId": placeholderIDPattern("igw")},
	"DeleteVpc":                  {"VpcId": placeholderIDPattern("vpc")},
}

// assertEveryDryRunRowSentOnce holds the wire to one DryRun=true request
// per DryRun-table row, each naming what it acts on with a placeholder.
func assertEveryDryRunRowSentOnce(t *testing.T, inner *dryRunEC2Doer) {
	t.Helper()
	assert.ElementsMatch(t, AWSReaderDryRunActions, inner.actions(), "the preflight sent another set of actions")
	for _, form := range inner.forms {
		action := form.Get("Action")
		assert.Equal(t, []string{"true"}, form["DryRun"], "%s went on the wire without DryRun=true", action)
		for field, pattern := range awsDryRunWireParams[action] {
			assert.Regexp(t, pattern, form.Get(field), "%s %s", action, field)
		}
	}
}

func TestAWSDryRunWireParamsCoverTheDryRunTable(t *testing.T) {
	t.Parallel()

	assert.ElementsMatch(t, AWSReaderDryRunActions, slices.Collect(maps.Keys(awsDryRunWireParams)))
}

func TestPreflightAWSReadKeyPassesWhenEveryActionIsUnauthorized(t *testing.T) {
	t.Parallel()

	inner := &dryRunEC2Doer{}
	wall, err := NewAWSReadWall("ec2", inner)
	require.NoError(t, err)

	require.NoError(t, PreflightAWSReadKey(context.Background(), awsReadTestConfig(&recordingAWSDoer{}), wall))
	assertEveryDryRunRowSentOnce(t, inner)
	assert.Equal(t, len(AWSReaderDryRunActions), wall.Calls())
}

func TestPreflightAWSReadKeyRefusesAnyOtherAnswerNamingTheAction(t *testing.T) {
	t.Parallel()

	const bad = "DeleteVpc"
	cases := []struct {
		name   string
		answer awsCannedAnswer
		want   string
	}{
		{"DryRunOperation", awsCannedAnswer{status: http.StatusPreconditionFailed, body: ec2ErrorBody("DryRunOperation")},
			bad + " answered DryRunOperation"},
		{"a 200", awsCannedAnswer{status: http.StatusOK, body: `<DeleteVpcResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><requestId>r-1</requestId><return>true</return></DeleteVpcResponse>`},
			bad + " succeeded with DryRun=true"},
		{"InvalidParameterValue", awsCannedAnswer{status: http.StatusBadRequest, body: ec2ErrorBody("InvalidParameterValue")},
			bad + " answered InvalidParameterValue"},
		{"AccessDenied", awsCannedAnswer{status: http.StatusForbidden, body: ec2ErrorBody("AccessDenied")},
			bad + " answered AccessDenied"},
		{"a 403 with AuthFailure", awsCannedAnswer{status: http.StatusForbidden, body: ec2ErrorBody("AuthFailure")},
			bad + " answered AuthFailure"},
		{"a transport error", awsCannedAnswer{err: errors.New("connection reset by peer")},
			bad + " got no EC2 answer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inner := &dryRunEC2Doer{answers: map[string]awsCannedAnswer{bad: tc.answer}}
			wall, err := NewAWSReadWall("ec2", inner)
			require.NoError(t, err)

			err = PreflightAWSReadKey(context.Background(), awsReadTestConfig(&recordingAWSDoer{}), wall)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			for _, action := range AWSReaderDryRunActions {
				if action != bad {
					assert.NotContains(t, err.Error(), action, "named an action that answered UnauthorizedOperation")
				}
			}
			assertEveryDryRunRowSentOnce(t, inner)
		})
	}
}

func TestPreflightAWSReadRefusesARowWithNoPlaceholderSendingNothing(t *testing.T) {
	t.Parallel()

	inner := &dryRunEC2Doer{}
	wall, err := NewAWSReadWall("ec2", inner)
	require.NoError(t, err)
	actions := append(slices.Clone(AWSReaderDryRunActions), "AuthorizeSecurityGroupIngress")

	err = preflightAWSRead(context.Background(), awsReadTestConfig(&recordingAWSDoer{}), wall, actions)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `rows with no placeholder request: AuthorizeSecurityGroupIngress`)
	assert.Empty(t, inner.forms, "a request went on the wire")
	assert.Zero(t, wall.Calls())
}

func TestPreflightAWSReadRefusesAnEmptyTable(t *testing.T) {
	t.Parallel()

	inner := &dryRunEC2Doer{}
	wall, err := NewAWSReadWall("ec2", inner)
	require.NoError(t, err)

	err = preflightAWSRead(context.Background(), awsReadTestConfig(&recordingAWSDoer{}), wall, nil)
	assert.ErrorContains(t, err, "the DryRun table is empty")
	assert.Empty(t, inner.forms, "a request went on the wire")
}
