package cli

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// layer2AWSEndpointKeys is every endpoint variable cloudEnv must set: the
// SDK's spelling of each service fakeaws serves, and the catch-all.
var layer2AWSEndpointKeys = []string{
	"AWS_ENDPOINT_URL_EC2", "AWS_ENDPOINT_URL_IAM", "AWS_ENDPOINT_URL_EKS",
	"AWS_ENDPOINT_URL_RDS", "AWS_ENDPOINT_URL_SQS", "AWS_ENDPOINT_URL_DYNAMODB",
	"AWS_ENDPOINT_URL_SECRETS_MANAGER", "AWS_ENDPOINT_URL_KMS", "AWS_ENDPOINT_URL_ROUTE_53",
	"AWS_ENDPOINT_URL_S3", "AWS_ENDPOINT_URL_STS", "AWS_ENDPOINT_URL_SSM",
	"AWS_ENDPOINT_URL",
}

// cloudEnvAWS is the AWS_* part of cloudEnv for cfg.
func cloudEnvAWS(cfg config.Config) map[string]string {
	env := map[string]string{}
	for key, value := range cloudEnv(&CommandRuntime{Config: cfg}) {
		if strings.HasPrefix(key, "AWS_") {
			env[key] = value
		}
	}
	return env
}

// A misspelt endpoint variable is not an error: the SDK ignores it and the
// service falls through to the catch-all. So the names are pinned here,
// literally, rather than read back from the map that sets them.
func TestCloudEnvPinsTheLayer2AWSEnv(t *testing.T) {
	t.Parallel()
	const fakeaws = "http://127.0.0.1:8082"

	env := cloudEnvAWS(config.Config{
		Fakeaws: config.FakeawsConfig{URL: fakeaws + "/"},
		S3:      config.S3Config{URL: "http://127.0.0.1:9090"},
	})

	assert.Equal(t, map[string]string{
		"AWS_ENDPOINT_URL_EC2":             fakeaws + "/ec2/region/us-east-1",
		"AWS_ENDPOINT_URL_IAM":             fakeaws + "/iam",
		"AWS_ENDPOINT_URL_EKS":             fakeaws + "/eks/region/us-east-1",
		"AWS_ENDPOINT_URL_RDS":             fakeaws + "/rds/region/us-east-1",
		"AWS_ENDPOINT_URL_SQS":             fakeaws + "/sqs/region/us-east-1",
		"AWS_ENDPOINT_URL_DYNAMODB":        fakeaws + "/dynamodb/region/us-east-1",
		"AWS_ENDPOINT_URL_SECRETS_MANAGER": fakeaws + "/secretsmanager/region/us-east-1",
		"AWS_ENDPOINT_URL_KMS":             fakeaws + "/kms/region/us-east-1",
		"AWS_ENDPOINT_URL_ROUTE_53":        fakeaws + "/route53",
		"AWS_ENDPOINT_URL_S3":              "http://127.0.0.1:9090",
		"AWS_ENDPOINT_URL_STS":             fakeaws + "/sts",
		"AWS_ENDPOINT_URL_SSM":             fakeaws + "/ssm/region/us-east-1",
		"AWS_ENDPOINT_URL":                 "http://127.0.0.1:1",
		"AWS_REGION":                       "us-east-1",
		"AWS_EC2_METADATA_DISABLED":        "true",
		"AWS_ACCESS_KEY_ID":                "test",
		"AWS_SECRET_ACCESS_KEY":            "test",
		"AWS_CONFIG_FILE":                  harness.AWSSealedConfigFile,
		"AWS_SHARED_CREDENTIALS_FILE":      harness.AWSSealedSharedCredentialsFile,
	}, env)

	for _, key := range []string{"AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE"} {
		_, err := os.Stat(env[key])
		assert.Error(t, err, "%s=%q must not exist, or ~/.aws is read", key, env[key])
	}

	withoutS3 := cloudEnvAWS(config.Config{Fakeaws: config.FakeawsConfig{URL: fakeaws}})
	assert.Equal(t, fakeaws+"/s3", withoutS3["AWS_ENDPOINT_URL_S3"], "no s3.url means fakeaws's own S3")
}

func TestCloudEnvAWSEndpointsFailClosedWithoutFakeaws(t *testing.T) {
	t.Parallel()

	env := cloudEnvAWS(config.Config{S3: config.S3Config{URL: "http://127.0.0.1:9090"}})

	for _, key := range layer2AWSEndpointKeys {
		assert.Equal(t, awsDeadEndpointURL, env[key], key)
	}
}

func TestCloudEnvAWSRegionReachesEveryRegionalPath(t *testing.T) {
	t.Parallel()

	for configured, want := range map[string]string{"eu-west-2": "eu-west-2", "": "us-east-1"} {
		env := cloudEnvAWS(config.Config{
			Fakeaws: config.FakeawsConfig{URL: "http://127.0.0.1:8082"},
			AWS:     config.AWSConfig{Region: configured},
		})

		assert.Equal(t, want, env["AWS_REGION"])
		var regional []string
		for key, value := range env {
			if _, region, ok := strings.Cut(value, "/region/"); ok {
				regional = append(regional, key)
				assert.Equal(t, want, region, key)
			}
		}
		assert.Len(t, regional, 8, "EC2, EKS, RDS, SQS, DYNAMODB, SECRETS_MANAGER, KMS and SSM are regional: %v", regional)
	}
}

// unsetEnv unsets key for the rest of the test; t.Setenv restores it.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	require.NoError(t, os.Unsetenv(key))
}

// The SDK the provider is built on, reading cloudEnv's AWS vars from the
// process env as a Layer 2 subprocess does. A service with no endpoint of
// its own dials the catch-all; with the catch-all gone too it takes the
// real AWS default, which the loopback-only client refuses. Either way
// nothing leaves loopback.
func TestLayer2AWSEnvKeepsTheSDKOnLoopback(t *testing.T) {
	for _, kv := range os.Environ() {
		if key, _, _ := strings.Cut(kv, "="); strings.HasPrefix(key, "AWS_") {
			unsetEnv(t, key)
		}
	}
	for key, value := range cloudEnvAWS(config.Config{Fakeaws: config.FakeawsConfig{URL: "http://127.0.0.1:8082"}}) {
		t.Setenv(key, value)
	}
	unsetEnv(t, "AWS_ENDPOINT_URL_STS")

	getCallerIdentity := func() error {
		ctx := context.Background()
		cfg, err := awsconfig.LoadDefaultConfig(ctx,
			awsconfig.WithHTTPClient(harness.NewLoopbackOnlyHTTPClient()),
			awsconfig.WithRetryMaxAttempts(1))
		require.NoError(t, err)
		_, err = sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		return err
	}

	err := getCallerIdentity()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dial tcp 127.0.0.1:1:", "STS must dial the catch-all")
	assert.NotErrorIs(t, err, harness.ErrNonLoopbackDial)

	unsetEnv(t, "AWS_ENDPOINT_URL")
	assert.ErrorIs(t, getCallerIdentity(), harness.ErrNonLoopbackDial)
}

// A new Layer 2 stage built without Layer2StripEnv would hand that one
// command the developer's AWS_PROFILE.
func TestLayer2HarnessesDeclareStripEnv(t *testing.T) {
	t.Parallel()

	var commands []harness.Command
	runner := harness.CommandRunnerFunc(func(_ context.Context, cmd harness.Command) (harness.CommandResult, error) {
		commands = append(commands, cmd)
		return harness.CommandResult{Stdout: []byte("{}")}, nil
	})
	ctx, dir := context.Background(), t.TempDir()

	_, err := harness.NewStaticHarness(runner).Run(ctx, dir, map[string]string{})
	require.NoError(t, err)
	_, err = harness.NewMockDeployHarness(runner, stubMockStateClient{}).Run(ctx, dir, map[string]string{}, harness.MockDeployModeClean)
	require.NoError(t, err)
	_, err = harness.NewDestroyHarness(runner, stubMockStateClient{}).Run(ctx, dir, map[string]string{})
	require.NoError(t, err)

	require.Len(t, commands, 8, "static init, validate, plan, show; mock init, apply, converge; destroy")
	for _, cmd := range commands {
		assert.True(t, slices.Contains(cmd.StripEnv, "AWS_*"), "tofu %v does not strip AWS_* (StripEnv=%v)", cmd.Args, cmd.StripEnv)
	}
}
