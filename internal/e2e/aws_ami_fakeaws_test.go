package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// TestAWSAMIParityAgainstFakeaws reads the AL2023 parameter from fakeaws
// SSM through the real resolver and requires exactly AWSLayer2AMI: the id
// Layer 2 hands the model must be one fakeaws serves. The required CI job
// runs it by name and fails if it does not report PASS.
func TestAWSAMIParityAgainstFakeaws(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)

	credFile := filepath.Join(t.TempDir(), "layer3-aws.env")
	require.NoError(t, os.WriteFile(credFile, []byte("AWS_ACCESS_KEY_ID=fakeaws-key-id\nAWS_SECRET_ACCESS_KEY=fakeaws-secret\n"), 0o600))
	env, err := harness.AWSSealedEnv(credFile, "us-east-1")
	require.NoError(t, err)

	id, err := harness.ResolveAWSAMIFromSSM(context.Background(), env, harness.NewLoopbackOnlyHTTPClient(), mock.URL+"/ssm/region/us-east-1")

	require.NoError(t, err)
	assert.Equal(t, harness.AWSLayer2AMI, id)
}
