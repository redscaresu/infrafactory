package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// TestAWSScopeClaimAgainstFakeaws runs the claim, stamp and default-VPC
// primitives through the real sealed env and SDK clients against fakeaws
// SSM and EC2. The required CI job runs it by name and fails if it does
// not report PASS.
func TestAWSScopeClaimAgainstFakeaws(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)
	const region = "eu-west-2"
	ssmURL := mock.URL + "/ssm/region/" + region
	ec2URL := mock.URL + "/ec2/region/" + region

	credFile := filepath.Join(t.TempDir(), "layer3-aws.env")
	require.NoError(t, os.WriteFile(credFile, []byte("AWS_ACCESS_KEY_ID=fakeaws-key-id\nAWS_SECRET_ACCESS_KEY=fakeaws-secret\n"), 0o600))
	env, err := harness.AWSSealedEnv(credFile, region)
	require.NoError(t, err)
	doer := harness.NewLoopbackOnlyHTTPClient()
	ctx := context.Background()

	first, err := harness.NewAWSClaimHolder("e2e-first")
	require.NoError(t, err)
	second, err := harness.NewAWSClaimHolder("e2e-second")
	require.NoError(t, err)
	// A holder whose run died on another machine, so it can be taken over.
	const crashed = "e2e-crashed@another-host.example:4242"

	readHolder := func() (string, bool) {
		holder, held, err := harness.ReadAWSClaimHolder(ctx, env, doer, ssmURL)
		require.NoError(t, err)
		return holder, held
	}

	require.NoError(t, harness.TakeAWSClaim(ctx, env, doer, ssmURL, first), "take")

	err = harness.TakeAWSClaim(ctx, env, doer, ssmURL, second)
	require.ErrorIs(t, err, harness.ErrAWSScopeClaimed, "a second holder")
	assert.Contains(t, err.Error(), first)

	err = harness.ReleaseAWSClaim(ctx, env, doer, ssmURL, second)
	require.Error(t, err, "a wrong-holder release")
	assert.Contains(t, err.Error(), first)
	holder, held := readHolder()
	assert.True(t, held)
	assert.Equal(t, first, holder)

	require.NoError(t, harness.ReleaseAWSClaim(ctx, env, doer, ssmURL, first), "release")
	_, held = readHolder()
	assert.False(t, held)

	require.NoError(t, harness.TakeAWSClaim(ctx, env, doer, ssmURL, crashed), "retake")

	named, held := readHolder()
	require.True(t, held)
	require.NoError(t, harness.TakeOverAWSClaim(ctx, env, doer, ssmURL, named, first), "take over")
	holder, _ = readHolder()
	assert.Equal(t, first, holder)

	err = harness.AssertAWSScopeStamp(ctx, env, doer, ssmURL, fakeawsAccount)
	require.Error(t, err, "stamp absent")
	assert.Contains(t, err.Error(), "docs/operations.md § Layer 3 (AWS)")
	// The stamp is written by hand at setup, never by harness code.
	setup := ssm.New(ssm.Options{
		Region:       region,
		Credentials:  credentials.NewStaticCredentialsProvider("fakeaws-key-id", "fakeaws-secret", ""),
		BaseEndpoint: aws.String(ssmURL),
		HTTPClient:   doer,
	})
	_, err = setup.PutParameter(ctx, &ssm.PutParameterInput{
		Name: aws.String(harness.AWSStampParameter), Value: aws.String(fakeawsAccount), Type: ssmtypes.ParameterTypeString,
	})
	require.NoError(t, err)
	assert.NoError(t, harness.AssertAWSScopeStamp(ctx, env, doer, ssmURL, fakeawsAccount), "stamp present")

	assert.NoError(t, harness.AssertNoAWSDefaultVPC(ctx, env, doer, ec2URL), "no default VPC")
}
