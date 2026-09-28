package cli

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/redscaresu/infrafactory/internal/harness"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const awsSealProbe = `printf 'ec2=%s profile=%s key=%s tfvar=%s http=%s https=%s cert=%s' ` +
	`"${AWS_ENDPOINT_URL_EC2-UNSET}" "${AWS_PROFILE-UNSET}" "${AWS_ACCESS_KEY_ID-UNSET}" ` +
	`"${TF_VAR_volume_size-UNSET}" "${HTTP_PROXY-UNSET}" "${HTTPS_PROXY-UNSET}" "${SSL_CERT_FILE-UNSET}"`

// plantAWSShellEnv exports what a developer's shell might carry into a
// Layer 3 run. http.ProxyFromEnvironment reads the proxy vars once per
// process, so it is primed first: the planted proxy must not become every
// later test's proxy.
func plantAWSShellEnv(t *testing.T) {
	t.Helper()
	_, _ = http.ProxyFromEnvironment(&http.Request{URL: &url.URL{Scheme: "https", Host: "example.invalid"}})
	t.Setenv("AWS_ENDPOINT_URL_EC2", "http://127.0.0.1:8082")
	t.Setenv("AWS_PROFILE", "planted")
	t.Setenv("AWS_ACCESS_KEY_ID", "PLANTED")
	t.Setenv("TF_VAR_volume_size", "10000")
	t.Setenv("HTTP_PROXY", "http://proxy.example:3128")
	t.Setenv("HTTPS_PROXY", "http://proxy.example:3129")
	t.Setenv("SSL_CERT_FILE", "/etc/ssl/corp.pem")
}

// The strip half of the AWS seal, through a real subprocess: a struct
// carrying the right StripEnv proves nothing if the runner ignores it.
func TestAWSSealedEnvIsAllALayer3SubprocessSees(t *testing.T) {
	plantAWSShellEnv(t)
	credFile := filepath.Join(t.TempDir(), "layer3-aws.env")
	require.NoError(t, os.WriteFile(credFile, []byte("AWS_ACCESS_KEY_ID=AKIAFROMTHEFILE00001\nAWS_SECRET_ACCESS_KEY=s\n"), 0o600))
	env, err := harness.AWSSealedEnv(credFile, "eu-west-2")
	require.NoError(t, err)

	result, err := execCommandRunner{}.Run(context.Background(), harness.Command{
		Name:     "sh",
		Args:     []string{"-c", awsSealProbe},
		Env:      env,
		StripEnv: harness.SandboxStripEnv,
	})
	require.NoError(t, err)

	assert.Equal(t, "ec2=UNSET profile=UNSET key=AKIAFROMTHEFILE00001 tfvar=UNSET "+
		"http=http://proxy.example:3128 https=http://proxy.example:3129 cert=/etc/ssl/corp.pem",
		string(result.Stdout))
}

// Layer 2 points the provider at fakeaws with AWS_ENDPOINT_URL_*, so the
// strip must stay per-command.
func TestLayer2SubprocessKeepsAWSEndpointURL(t *testing.T) {
	plantAWSShellEnv(t)

	result, err := execCommandRunner{}.Run(context.Background(), harness.Command{
		Name: "sh",
		Args: []string{"-c", `printf '%s' "${AWS_ENDPOINT_URL_EC2-UNSET}"`},
	})
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:8082", string(result.Stdout))
}
