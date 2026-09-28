package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
)

const (
	preflightAWSAccount   = "123456789012"
	preflightAWSPrincipal = "arn:aws:iam::123456789012:user/infrafactory-layer3"
	preflightAWSKeyID     = "AKIAFROMTHEFILE00001"
	preflightAWSSecret    = "secret-from-the-credential-file"
)

func callerIdentityXML(account, arn string) string {
	return `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><GetCallerIdentityResult>` +
		`<Arn>` + arn + `</Arn><UserId>AIDAEXAMPLEUSERID0001</UserId><Account>` + account + `</Account>` +
		`</GetCallerIdentityResult></GetCallerIdentityResponse>`
}

// stsReroute is the preflight's doer in these tests. The endpoint is not
// injectable, so it records the host each request was addressed to, then
// sends it to the test's STS through harness's loopback-only client.
type stsReroute struct {
	target *url.URL
	next   *http.Client

	mu    sync.Mutex
	hosts []string
	auth  []string
}

func (d *stsReroute) Do(req *http.Request) (*http.Response, error) {
	d.mu.Lock()
	d.hosts = append(d.hosts, req.URL.Host)
	d.auth = append(d.auth, req.Header.Get("Authorization"))
	d.mu.Unlock()
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host, req.Host = d.target.Scheme, d.target.Host, ""
	return d.next.Do(req)
}

// awsPreflightFixture is a runtime whose aws block, HOME credential file
// and STS all agree, with the STS answering status and body.
func awsPreflightFixture(t *testing.T, status int, body string) (*CommandRuntime, *stsReroute) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	require.NoError(t, err)

	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "infrafactory")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "layer3-aws.env"),
		[]byte("AWS_ACCESS_KEY_ID="+preflightAWSKeyID+"\nAWS_SECRET_ACCESS_KEY="+preflightAWSSecret+"\n"), 0o600))

	doer := &stsReroute{target: target, next: harness.NewLoopbackOnlyHTTPClient()}
	rt := &CommandRuntime{Config: config.Default(), Deps: RuntimeDependencies{AWSSTS: doer}}
	rt.Config.AWS = config.AWSConfig{Region: "eu-west-2", AccountID: preflightAWSAccount, PrincipalARN: preflightAWSPrincipal}
	return rt, doer
}

func TestAWSPreflightPassesTheConfiguredIdentityAndEnvMakesNoCall(t *testing.T) {
	rt, doer := awsPreflightFixture(t, http.StatusOK, callerIdentityXML(preflightAWSAccount, preflightAWSPrincipal))

	require.NoError(t, assertSandboxCredentials(rt, layer3AWS))
	require.Len(t, doer.hosts, 1)
	assert.Equal(t, "sts.eu-west-2.amazonaws.com", doer.hosts[0], "the preflight asks real STS, never a configured endpoint")
	assert.Contains(t, doer.auth[0], "Credential="+preflightAWSKeyID+"/", "the request is signed with the credential file's key")
	assert.Contains(t, sandboxPreflightPassDetail(rt, layer3AWS), preflightAWSAccount)

	env, err := sandboxCommandEnvForProject(rt, layer3AWS, preflightAWSAccount)
	require.NoError(t, err)
	assert.Len(t, env, 6)
	assert.Equal(t, preflightAWSKeyID, env["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, preflightAWSSecret, env["AWS_SECRET_ACCESS_KEY"])
	assert.Equal(t, "eu-west-2", env["AWS_REGION"])
	assert.Len(t, doer.hosts, 1, "building the env makes no STS request")
}

func TestAWSPreflightRefusesTheWrongIdentityOrAFailedCall(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   []string
	}{
		"wrong account": {http.StatusOK, callerIdentityXML("210987654321", "arn:aws:iam::210987654321:user/infrafactory-layer3"),
			[]string{"210987654321", preflightAWSAccount}},
		"wrong principal": {http.StatusOK, callerIdentityXML(preflightAWSAccount, "arn:aws:iam::123456789012:user/admin"),
			[]string{"arn:aws:iam::123456789012:user/admin", preflightAWSPrincipal}},
		"sts 403": {http.StatusForbidden,
			`<ErrorResponse><Error><Type>Sender</Type><Code>InvalidClientTokenId</Code><Message>invalid</Message></Error></ErrorResponse>`,
			[]string{"sts:GetCallerIdentity failed", "403", "InvalidClientTokenId"}},
		"malformed xml": {http.StatusOK, "<GetCallerIdentityResponse><GetCallerIdentityResult>",
			[]string{"sts:GetCallerIdentity failed", "deserialization failed"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rt, doer := awsPreflightFixture(t, tc.status, tc.body)

			err := assertSandboxCredentials(rt, layer3AWS)

			require.Error(t, err)
			for _, want := range tc.want {
				assert.Contains(t, err.Error(), want)
			}
			assert.NotContains(t, err.Error(), preflightAWSSecret)
			assert.NotEmpty(t, doer.hosts)
		})
	}
}

func TestAWSPreflightLocalRefusalsMakeNoSTSRequest(t *testing.T) {
	credFile := func() string {
		return filepath.Join(os.Getenv("HOME"), ".config", "infrafactory", "layer3-aws.env")
	}
	cases := map[string]struct {
		mutate func(t *testing.T, rt *CommandRuntime)
		want   string
	}{
		"missing credential file": {func(t *testing.T, _ *CommandRuntime) { require.NoError(t, os.Remove(credFile())) },
			"no such file"},
		"credential file mode 0644": {func(t *testing.T, _ *CommandRuntime) { require.NoError(t, os.Chmod(credFile(), 0o644)) },
			"mode 0644"},
		"empty account_id":    {func(_ *testing.T, rt *CommandRuntime) { rt.Config.AWS.AccountID = "" }, "aws.account_id is empty"},
		"empty principal_arn": {func(_ *testing.T, rt *CommandRuntime) { rt.Config.AWS.PrincipalARN = "" }, "aws.principal_arn is empty"},
		"empty region":        {func(_ *testing.T, rt *CommandRuntime) { rt.Config.AWS.Region = "" }, "aws.region is empty"},
		"malformed region": {func(_ *testing.T, rt *CommandRuntime) { rt.Config.AWS.Region = "x@evil.example/" },
			`"x@evil.example/" is not an AWS region name`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rt, doer := awsPreflightFixture(t, http.StatusOK, callerIdentityXML(preflightAWSAccount, preflightAWSPrincipal))
			tc.mutate(t, rt)

			err := assertSandboxCredentials(rt, layer3AWS)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Empty(t, doer.hosts)
		})
	}

	// Without an injected client the preflight refuses rather than fall
	// back to the SDK's, which nothing here could stop reaching AWS.
	t.Run("no STS client", func(t *testing.T) {
		rt, _ := awsPreflightFixture(t, http.StatusOK, callerIdentityXML(preflightAWSAccount, preflightAWSPrincipal))
		rt.Deps.AWSSTS = nil
		err := assertSandboxCredentials(rt, layer3AWS)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no STS HTTP client")
	})
}

func TestAWSCommandEnvRefusesAnyScopeButTheConfiguredAccount(t *testing.T) {
	for _, scope := range []string{"", "  ", "210987654321", preflightAWSAccount + " "} {
		rt, doer := awsPreflightFixture(t, http.StatusOK, callerIdentityXML(preflightAWSAccount, preflightAWSPrincipal))

		env, err := sandboxCommandEnvForProject(rt, layer3AWS, scope)

		require.Error(t, err, "scope %q", scope)
		assert.Nil(t, env)
		assert.Empty(t, doer.hosts)
	}
}
