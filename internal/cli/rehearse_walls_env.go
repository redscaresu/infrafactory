package cli

import (
	"path/filepath"
	"strings"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// rehearsalFakeAccessKeyID and rehearsalFakeSecretAccessKey are the static
// keys every wall hands tofu, the environment and the provider override
// alike. fakeaws reads no key, and they are not the example pair the
// fail-closed tests plant in the shell, so a request carrying that pair
// came from the shell, not from a wall.
const (
	rehearsalFakeAccessKeyID     = "infrafactory-rehearsal-fake-key"
	rehearsalFakeSecretAccessKey = "infrafactory-rehearsal-fake-secret"
)

// rehearsalWallsEnv is replace mode's whole environment for a tofu step
// after prepare (ADR-0042 § The environment): the rehearsal's HOME and
// TF_DATA_DIR, the fake keys, AWS_REGION and no other region variable, no
// IMDS, unreadable shared files, and an endpoint per modelled service at
// fakeawsURL. S3, the catch-all and both proxies go to rehearsalDeadProxy,
// with no NO_PROXY. An empty fakeawsURL sends every service there too, as
// awsLayer2Env does.
func rehearsalWallsEnv(rehearsal, fakeawsURL, region string) map[string]string {
	env := map[string]string{
		"HOME":                        filepath.Join(rehearsal, "home"),
		"TF_DATA_DIR":                 filepath.Join(rehearsal, "data"),
		"AWS_ACCESS_KEY_ID":           rehearsalFakeAccessKeyID,
		"AWS_SECRET_ACCESS_KEY":       rehearsalFakeSecretAccessKey,
		"AWS_REGION":                  region,
		"AWS_EC2_METADATA_DISABLED":   "true",
		"AWS_CONFIG_FILE":             harness.AWSSealedConfigFile,
		"AWS_SHARED_CREDENTIALS_FILE": harness.AWSSealedSharedCredentialsFile,
		"AWS_ENDPOINT_URL":            rehearsalDeadProxy,
		"AWS_ENDPOINT_URL_S3":         rehearsalDeadProxy,
		"HTTPS_PROXY":                 rehearsalDeadProxy,
		"HTTP_PROXY":                  rehearsalDeadProxy,
	}
	fakeaws := strings.TrimRight(strings.TrimSpace(fakeawsURL), "/")
	for svc, path := range awsFakeawsPaths {
		if svc == "S3" {
			continue
		}
		endpoint := rehearsalDeadProxy
		if fakeaws != "" {
			endpoint = fakeaws + strings.ReplaceAll(path, "{region}", region)
		}
		env["AWS_ENDPOINT_URL_"+svc] = endpoint
	}
	return env
}
