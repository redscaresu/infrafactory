package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/redscaresu/infrafactory/internal/harness"
)

const (
	wallsTestRehearsal = "/repo/.infrafactory/rehearsals/r1"
	wallsTestFakeaws   = "http://127.0.0.1:4567"
	wallsTestRegion    = "eu-west-2"
)

// The whole map, literally: a key added, dropped or misspelt, a region
// variable beside AWS_REGION, a NO_PROXY, another cloud's variable or S3
// at fakeaws all turn it red.
func TestRehearsalWallsEnvExactKeys(t *testing.T) {
	t.Parallel()

	env := rehearsalWallsEnv(wallsTestRehearsal, wallsTestFakeaws+"/", wallsTestRegion)

	assert.Equal(t, map[string]string{
		"HOME":                             wallsTestRehearsal + "/home",
		"TF_DATA_DIR":                      wallsTestRehearsal + "/data",
		"AWS_ACCESS_KEY_ID":                rehearsalFakeAccessKeyID,
		"AWS_SECRET_ACCESS_KEY":            rehearsalFakeSecretAccessKey,
		"AWS_REGION":                       wallsTestRegion,
		"AWS_EC2_METADATA_DISABLED":        "true",
		"AWS_CONFIG_FILE":                  harness.AWSSealedConfigFile,
		"AWS_SHARED_CREDENTIALS_FILE":      harness.AWSSealedSharedCredentialsFile,
		"AWS_ENDPOINT_URL":                 rehearsalDeadProxy,
		"AWS_ENDPOINT_URL_S3":              rehearsalDeadProxy,
		"AWS_ENDPOINT_URL_EC2":             wallsTestFakeaws + "/ec2/region/eu-west-2",
		"AWS_ENDPOINT_URL_IAM":             wallsTestFakeaws + "/iam",
		"AWS_ENDPOINT_URL_EKS":             wallsTestFakeaws + "/eks/region/eu-west-2",
		"AWS_ENDPOINT_URL_RDS":             wallsTestFakeaws + "/rds/region/eu-west-2",
		"AWS_ENDPOINT_URL_SQS":             wallsTestFakeaws + "/sqs/region/eu-west-2",
		"AWS_ENDPOINT_URL_DYNAMODB":        wallsTestFakeaws + "/dynamodb/region/eu-west-2",
		"AWS_ENDPOINT_URL_SECRETS_MANAGER": wallsTestFakeaws + "/secretsmanager/region/eu-west-2",
		"AWS_ENDPOINT_URL_KMS":             wallsTestFakeaws + "/kms/region/eu-west-2",
		"AWS_ENDPOINT_URL_ROUTE_53":        wallsTestFakeaws + "/route53",
		"AWS_ENDPOINT_URL_STS":             wallsTestFakeaws + "/sts",
		"AWS_ENDPOINT_URL_SSM":             wallsTestFakeaws + "/ssm/region/eu-west-2",
		"HTTPS_PROXY":                      rehearsalDeadProxy,
		"HTTP_PROXY":                       rehearsalDeadProxy,
	}, env)
}

func TestRehearsalWallsEnvSetsOnlyAWSRegion(t *testing.T) {
	t.Parallel()

	env := rehearsalWallsEnv(wallsTestRehearsal, wallsTestFakeaws, wallsTestRegion)

	assert.Equal(t, wallsTestRegion, env["AWS_REGION"])
	for key, value := range env {
		if key != "AWS_REGION" && !strings.HasPrefix(key, "AWS_ENDPOINT_URL_") {
			assert.NotContains(t, key, "REGION", "a second region variable")
		}
		assert.NotContains(t, value, "{region}", key)
	}
}

func TestRehearsalWallsEnvSetsTheFakeKeys(t *testing.T) {
	t.Parallel()

	env := rehearsalWallsEnv(wallsTestRehearsal, wallsTestFakeaws, wallsTestRegion)

	assert.Equal(t, rehearsalFakeAccessKeyID, env["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, rehearsalFakeSecretAccessKey, env["AWS_SECRET_ACCESS_KEY"])
	// The example pair the fail-closed tests plant in the shell
	// (rehearsal_assumptions_test.go, package cli_test): a wall's keys must
	// be told apart from it.
	assert.NotEqual(t, "AKIAIOSFODNN7EXAMPLE", rehearsalFakeAccessKeyID)
	assert.NotEqual(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", rehearsalFakeSecretAccessKey)
}

// An empty fakeaws URL leaves no service with a relative endpoint: every
// one goes to the dead proxy, as awsLayer2Env sends them to its catch-all.
func TestRehearsalWallsEnvWithoutFakeawsIsDead(t *testing.T) {
	t.Parallel()

	env := rehearsalWallsEnv(wallsTestRehearsal, " ", wallsTestRegion)

	for key, value := range env {
		if strings.HasPrefix(key, "AWS_ENDPOINT_URL") {
			assert.Equal(t, rehearsalDeadProxy, value, key)
		}
	}
}
