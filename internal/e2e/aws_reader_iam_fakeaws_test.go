package e2e

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// TestAWSReadIAMAgainstFakeaws reads one of two created roles through the
// walls against fakeaws IAM, so the role's follow-ups (aws_reader_iam.go)
// are checked against fakeaws's own answers. fakeaws at the pin keeps no
// inline policy (PutRolePolicy is a no-op), so ListRolePolicies lists none
// and no GetRolePolicy is sent; the unit tests cover that call. The
// required CI job runs it by name and fails if it does not report PASS.
func TestAWSReadIAMAgainstFakeaws(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)
	const (
		region   = "eu-west-2"
		attached = "arn:aws:iam::aws:policy/ReadOnlyAccess"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	credFile := filepath.Join(t.TempDir(), "reader-aws.env")
	require.NoError(t, os.WriteFile(credFile, []byte("AWS_ACCESS_KEY_ID=fakeaws-key-id\nAWS_SECRET_ACCESS_KEY=fakeaws-secret\n"), 0o600))
	cfg, err := harness.AWSReaderConfig(credFile, region)
	require.NoError(t, err)

	// Planted unwalled: the read itself sends only reader-table actions.
	seed := func(params neturl.Values) {
		params.Set("Version", "2010-05-08")
		resp, body := awsPost(t, mock.URL+"/iam", params.Encode(), "application/x-www-form-urlencoded")
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", params.Get("Action"), body)
	}
	for _, role := range []string{"web", "other"} {
		seed(neturl.Values{"Action": {"CreateRole"}, "RoleName": {role}, "AssumeRolePolicyDocument": {`{"Version":"2012-10-17","Statement":[]}`}})
		seed(neturl.Values{"Action": {"AttachRolePolicy"}, "RoleName": {role}, "PolicyArn": {attached}})
	}
	seed(neturl.Values{"Action": {"PutRolePolicy"}, "RoleName": {"web"}, "PolicyName": {"inline"}, "PolicyDocument": {`{"Version":"2012-10-17","Statement":[]}`}})

	read, err := harness.ReadAWSAccount(ctx, harness.AWSReadInput{
		Config: cfg, Account: fakeawsAccount, Principal: fakeawsPrincipal,
		ReadSet:   []harness.AWSReadEntry{{Service: "iam", Protocol: "query", Action: "GetRole", Params: json.RawMessage(`{"RoleName":"web"}`)}},
		Doer:      harness.NewLoopbackOnlyHTTPClient(),
		Identity:  harness.VerifyAWSReadIdentity,
		Preflight: func(context.Context, aws.Config, *harness.AWSReadWall) error { return nil },
		Endpoints: map[string]string{
			"sts": mock.URL + "/sts",
			"ssm": mock.URL + "/ssm/region/" + region,
			"ec2": mock.URL + "/ec2/region/" + region,
			"iam": mock.URL + "/iam",
		},
	})
	require.NoError(t, err)

	byAction := map[string][]harness.AWSReadExchange{}
	for _, exchange := range read.Exchanges {
		assert.Equal(t, http.StatusOK, exchange.Status, "%s %s: %s", exchange.Action, exchange.Params, exchange.Body)
		assert.NotContains(t, string(exchange.Params)+exchange.Body, "role/other", "the unread role was read: %s", exchange.Action)
		byAction[exchange.Action] = append(byAction[exchange.Action], exchange)
	}
	require.Len(t, byAction["GetRole"], 1)
	assert.Contains(t, byAction["GetRole"][0].Body, "<RoleName>web</RoleName>")

	require.Len(t, byAction["ListRolePolicies"], 1)
	var inline struct {
		Names []string `xml:"ListRolePoliciesResult>PolicyNames>member"`
	}
	require.NoError(t, xml.Unmarshal([]byte(byAction["ListRolePolicies"][0].Body), &inline))
	assert.Len(t, byAction["GetRolePolicy"], len(inline.Names), "one GetRolePolicy per listed inline policy")

	require.Len(t, byAction["ListAttachedRolePolicies"], 1)
	assert.Contains(t, byAction["ListAttachedRolePolicies"][0].Body, "<PolicyArn>"+attached+"</PolicyArn>")
	require.Len(t, byAction["ListInstanceProfilesForRole"], 1)
	for _, exchange := range read.Exchanges {
		assert.Contains(t, string(exchange.Params), `"RoleName":"web"`, "%s", exchange.Action)
	}
	assert.Equal(t, 1+len(read.Exchanges), read.Calls, "identity and each IAM request, once")
}
