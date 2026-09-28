package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAWSKeyID  = "AKIAFROMTHEFILE00001"
	testAWSSecret = "secret-from-the-credential-file"
)

// writeAWSCredFile writes a Layer 3 AWS credential file with an explicit
// mode, so the umask cannot decide what the test checks.
func writeAWSCredFile(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "layer3-aws.env")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	require.NoError(t, os.Chmod(path, mode))
	return path
}

func validAWSCredFile(t *testing.T) string {
	t.Helper()
	return writeAWSCredFile(t, "AWS_ACCESS_KEY_ID="+testAWSKeyID+"\nAWS_SECRET_ACCESS_KEY="+testAWSSecret+"\n", 0o600)
}

func TestAWSSealedEnvHasExactlyTheSixKeys(t *testing.T) {
	t.Parallel()

	env, err := AWSSealedEnv(validAWSCredFile(t), "eu-west-2")
	require.NoError(t, err)

	assert.Equal(t, map[string]string{
		"AWS_ACCESS_KEY_ID":           testAWSKeyID,
		"AWS_SECRET_ACCESS_KEY":       testAWSSecret,
		"AWS_REGION":                  "eu-west-2",
		"AWS_EC2_METADATA_DISABLED":   "true",
		"AWS_SHARED_CREDENTIALS_FILE": AWSSealedSharedCredentialsFile,
		"AWS_CONFIG_FILE":             AWSSealedConfigFile,
	}, env)

	// The file paths only seal ~/.aws if nothing can ever exist there.
	for _, key := range []string{"AWS_SHARED_CREDENTIALS_FILE", "AWS_CONFIG_FILE"} {
		path := env[key]
		assert.True(t, filepath.IsAbs(path), "%s=%q must be absolute", key, path)
		_, err := os.Stat(path)
		assert.Error(t, err, "%s=%q must not exist", key, path)
		assert.Error(t, os.MkdirAll(filepath.Dir(path), 0o700), "%s=%q: its parent must be uncreatable", key, path)
	}
}

func TestAWSSealedEnvIgnoresTheProcessEnv(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "PLANTED")
	t.Setenv("AWS_REGION", "ap-southeast-2")
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "config"))

	env, err := AWSSealedEnv(validAWSCredFile(t), "eu-west-2")
	require.NoError(t, err)
	assert.Equal(t, testAWSKeyID, env["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "eu-west-2", env["AWS_REGION"])
	assert.Equal(t, AWSSealedConfigFile, env["AWS_CONFIG_FILE"])
}

func TestAWSSealedEnvRefuses(t *testing.T) {
	t.Parallel()

	valid := "AWS_ACCESS_KEY_ID=" + testAWSKeyID + "\nAWS_SECRET_ACCESS_KEY=" + testAWSSecret + "\n"
	cases := []struct {
		name     string
		credFile func(t *testing.T) string
		region   string
		reason   string
	}{
		{"missing file", func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.env") }, "us-east-1", "no such file"},
		{"directory", func(t *testing.T) string { return t.TempDir() }, "us-east-1", "not a regular file"},
		{"symlink", func(t *testing.T) string {
			link := filepath.Join(t.TempDir(), "link.env")
			require.NoError(t, os.Symlink(validAWSCredFile(t), link))
			return link
		}, "us-east-1", "not a regular file"},
		{"mode 0644", func(t *testing.T) string { return writeAWSCredFile(t, valid, 0o644) }, "us-east-1", "mode 0644"},
		{"mode 0640", func(t *testing.T) string { return writeAWSCredFile(t, valid, 0o640) }, "us-east-1", "mode 0640"},
		{"empty key", func(t *testing.T) string {
			return writeAWSCredFile(t, "AWS_ACCESS_KEY_ID=\nAWS_SECRET_ACCESS_KEY="+testAWSSecret+"\n", 0o600)
		}, "us-east-1", "no value for AWS_ACCESS_KEY_ID"},
		{"missing key", func(t *testing.T) string {
			return writeAWSCredFile(t, "AWS_ACCESS_KEY_ID="+testAWSKeyID+"\n", 0o600)
		}, "us-east-1", "no value for AWS_SECRET_ACCESS_KEY"},
		{"AWS_SESSION_TOKEN", func(t *testing.T) string {
			return writeAWSCredFile(t, valid+"AWS_SESSION_TOKEN=tok\n", 0o600)
		}, "us-east-1", `unknown key "AWS_SESSION_TOKEN"`},
		{"duplicate key", func(t *testing.T) string {
			return writeAWSCredFile(t, valid+"AWS_ACCESS_KEY_ID=AKIASECOND\n", 0o600)
		}, "us-east-1", "sets AWS_ACCESS_KEY_ID twice"},
		{"not KEY=VALUE", func(t *testing.T) string {
			return writeAWSCredFile(t, valid+"garbage\n", 0o600)
		}, "us-east-1", "line 3 is not KEY=VALUE"},
		{"empty region", validAWSCredFile, "", "not an AWS region name"},
		{"region with a host", validAWSCredFile, "x@evil.example/", "not an AWS region name"},
		{"region with a domain", validAWSCredFile, "us-east-1.evil.com", "not an AWS region name"},
		{"upper-case region", validAWSCredFile, "US-EAST-1", "not an AWS region name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env, err := AWSSealedEnv(tc.credFile(t), tc.region)
			require.Error(t, err)
			assert.Nil(t, env)
			assert.Contains(t, err.Error(), tc.reason)
			assert.NotContains(t, err.Error(), testAWSSecret, "a refusal must never echo the secret")
		})
	}
}

func TestAWSSealedEnvAcceptsOwnerOnlyFileAndRealRegions(t *testing.T) {
	t.Parallel()

	for _, region := range []string{"us-east-1", "eu-west-2", "us-gov-west-1"} {
		env, err := AWSSealedEnv(validAWSCredFile(t), region)
		require.NoError(t, err, region)
		assert.Equal(t, region, env["AWS_REGION"])
	}
}
