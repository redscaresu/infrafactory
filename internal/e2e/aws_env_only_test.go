package e2e

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every service fakeaws serves without Docker, applied through the
// provider block generation writes, which names no endpoint: each call
// reaches fakeaws through its AWS_ENDPOINT_URL_<SERVICE> in cloudEnv, and
// STS is called at provider configure. A misspelt variable sends that
// service to the dead catch-all and the apply fails. S3 and EKS are
// TestE2E_AWSFullStack's (Docker).
func TestE2E_AWSEnvOnlyEveryService(t *testing.T) {
	run, mock := startSealedAWSFixture(t, "aws-env-only", "every-service")
	collections := [][2]string{
		{"ec2", "vpcs"}, {"ec2", "subnets"}, {"iam", "roles"}, {"sqs", "queues"},
		{"dynamodb", "tables"}, {"route53", "hosted_zones"}, {"secretsmanager", "secrets"},
		{"rds", "db_subnet_groups"}, {"rds", "db_parameter_groups"},
	}

	// A pass means the second plan was empty: drift fails the run.
	applied := run("--no-destroy")
	require.NoError(t, applied.Err, "stdout:\n%s\nfakeaws log: %s", applied.Stdout, mock.LogPath())
	assert.Contains(t, applied.Stdout, "run/terminal_reason: pass (target_reached)")
	state := mock.FetchState(t)
	for _, c := range collections {
		assert.NotZero(t, awsStateItemCount(state, c[0], c[1]), "%s/%s after apply", c[0], c[1])
	}

	destroyed := run()
	require.NoError(t, destroyed.Err, "stdout:\n%s\nfakeaws log: %s", destroyed.Stdout, mock.LogPath())
	assert.Contains(t, destroyed.Stdout, "run/terminal_reason: pass (target_reached)")
	state = mock.FetchState(t)
	for _, c := range collections {
		assert.Zero(t, awsStateItemCount(state, c[0], c[1]), "%s/%s after destroy", c[0], c[1])
	}
}

// With no skip_* in the provider block, the provider calls STS when it
// configures, so `validate` needs fakeaws up. Down, plan fails dialing
// fakeaws's address: the dead proxy instead would mean STS went to real
// AWS.
func TestE2E_AWSValidateNeedsFakeawsSTS(t *testing.T) {
	SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")
	closed := fmt.Sprintf("127.0.0.1:%d", pickFreePort(t))
	SealNetwork(t)
	command := sealedAWSCommand(t, "aws-layer2-env", "vpc-subnet", "http://"+closed)

	generated := command("generate")
	require.NoError(t, generated.Err, "stdout:\n%s\nstderr:\n%s", generated.Stdout, generated.Stderr)
	result := command("validate")

	require.Error(t, result.Err, "stdout:\n%s", result.Stdout)
	output := result.Stdout + result.Stderr
	assert.Contains(t, output, "static/plan: fail", output)
	assert.Contains(t, output, "dial tcp "+closed+":", output)
	assert.NotContains(t, output, "dial tcp 127.0.0.1:9:")
}
