package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	code := m.Run()
	cleanupAWSMirror()
	os.Exit(code)
}

// startSealedAWSFixture starts fakeaws and seals the network, and returns
// a func that runs `run` on testdata/aws-layer2-env/<name>.yaml with
// <name>.tf as the generated HCL.
func startSealedAWSFixture(t *testing.T, name string) (func(args ...string) InfrafactoryResult, *MockwayInstance) {
	t.Helper()
	SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")
	mock := StartFakeaws(t)
	SealNetwork(t)

	fixtures := filepath.Join(RepoRoot(t), "internal", "e2e", "testdata", "aws-layer2-env")
	hcl, err := os.ReadFile(filepath.Join(fixtures, name+".tf"))
	require.NoError(t, err)
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "infrafactory.yaml")
	WriteConfigMultiCloud(t, configPath, "http://127.0.0.1:1", "", mock.URL, "", filepath.Join(workspace, "output"))

	return func(args ...string) InfrafactoryResult {
		return RunInfrafactory(t, InfrafactoryRunOptions{
			Args:           append([]string{"run", filepath.Join(fixtures, name+".yaml"), "--config", configPath}, args...),
			GeneratorFiles: map[string][]byte{"main.tf": hcl},
		})
	}, mock
}

// Layer 2 strips the shell's AWS_* from every tofu it runs. Without the
// strip, the planted profile is one the provider cannot find, and the
// apply fails on it.
func TestE2E_AWSLayer2IgnoresShellAWS(t *testing.T) {
	run, mock := startSealedAWSFixture(t, "vpc-subnet")
	t.Setenv("AWS_PROFILE", "infrafactory-planted-missing")
	t.Setenv("AWS_IGNORE_CONFIGURED_ENDPOINT_URLS", "true")
	t.Setenv("AWS_ACCESS_KEY_ID", "PLANTED")

	// A pass means the second plan was empty: drift fails the run.
	applied := run("--no-destroy")
	require.NoError(t, applied.Err, "stdout:\n%s\nfakeaws log: %s", applied.Stdout, mock.LogPath())
	assert.Contains(t, applied.Stdout, "run/terminal_reason: pass (target_reached)")
	state := mock.FetchState(t)
	assert.Equal(t, 1, awsStateItemCount(state, "ec2", "vpcs"))
	assert.Equal(t, 1, awsStateItemCount(state, "ec2", "subnets"))

	destroyed := run()
	require.NoError(t, destroyed.Err, "stdout:\n%s\nfakeaws log: %s", destroyed.Stdout, mock.LogPath())
	assert.Contains(t, destroyed.Stdout, "run/terminal_reason: pass (target_reached)")
	state = mock.FetchState(t)
	assert.Zero(t, awsStateItemCount(state, "ec2", "vpcs"))
	assert.Zero(t, awsStateItemCount(state, "ec2", "subnets"))
}

// logs has no endpoint in the provider block or in cloudEnv, so the
// provider must take AWS_ENDPOINT_URL, the dead catch-all. An error naming
// the dead proxy instead would mean the provider ignored the catch-all
// and aimed at real AWS.
func TestE2E_AWSLayer2CatchAllFailsClosed(t *testing.T) {
	run, mock := startSealedAWSFixture(t, "log-group")

	result := run("--repair-iterations-max", "1")

	require.Error(t, result.Err, "stdout:\n%s", result.Stdout)
	output := result.Stdout + result.Stderr
	assert.Contains(t, output, "dial tcp 127.0.0.1:1:", "fakeaws log: %s", mock.LogPath())
	assert.NotContains(t, output, "dial tcp 127.0.0.1:9:")
}
