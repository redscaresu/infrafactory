package harness

import (
	"fmt"
	"regexp"
)

// The shared credentials and config files both layers' env points the AWS
// SDK and terraform-provider-aws at. Children of /dev/null: absolute,
// and no user, root included, can create them, so ~/.aws/credentials,
// ~/.aws/config and their `services` endpoint overrides are never read.
// The SDK treats an unreadable shared file as empty.
const (
	AWSSealedSharedCredentialsFile = "/dev/null/infrafactory-aws-credentials"
	AWSSealedConfigFile            = "/dev/null/infrafactory-aws-config"
)

var awsRegionPattern = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]+$`)

// ValidAWSRegion reports whether region is shaped like an AWS region
// name. The region is interpolated into the STS hostname, so anything
// else (a dot, a slash, an @) could retarget the request.
func ValidAWSRegion(region string) bool {
	return awsRegionPattern.MatchString(region)
}

// AWSSealedEnv builds the complete AWS environment for a Layer 3 command
// and for infrafactory's own AWS calls, from credFile alone. It never
// reads the process env: an inherited AWS_PROFILE or AWS_ENDPOINT_URL_*
// would otherwise retarget "real AWS" (ADR-0023 rules 1-2), and the
// subprocess half of that is SandboxStripEnv's AWS_*.
//
// credFile is $HOME/.config/infrafactory/layer3-aws.env, beside
// layer3.env: KEY=VALUE lines holding only AWS_ACCESS_KEY_ID and
// AWS_SECRET_ACCESS_KEY, readable by its owner alone.
func AWSSealedEnv(credFile, region string) (map[string]string, error) {
	if !ValidAWSRegion(region) {
		return nil, fmt.Errorf("aws sealed env: region %q is not an AWS region name", region)
	}
	creds, err := readAWSCredentialFile(credFile)
	if err != nil {
		return nil, fmt.Errorf("aws sealed env: %w", err)
	}
	return map[string]string{
		"AWS_ACCESS_KEY_ID":           creds["AWS_ACCESS_KEY_ID"],
		"AWS_SECRET_ACCESS_KEY":       creds["AWS_SECRET_ACCESS_KEY"],
		"AWS_REGION":                  region,
		"AWS_EC2_METADATA_DISABLED":   "true",
		"AWS_SHARED_CREDENTIALS_FILE": AWSSealedSharedCredentialsFile,
		"AWS_CONFIG_FILE":             AWSSealedConfigFile,
	}, nil
}

// readAWSCredentialFile reads layer3-aws.env through readOwnerOnlyFile
// and parseCredentialLines: exactly the key id and the secret, each with
// a value.
func readAWSCredentialFile(path string) (map[string]string, error) {
	data, err := readOwnerOnlyFile(path)
	if err != nil {
		return nil, err
	}
	keys := []string{awsReaderKeyID, awsReaderSecret}
	lines, err := parseCredentialLines(path, data, keys)
	if err != nil {
		return nil, err
	}
	creds := map[string]string{}
	for _, key := range keys {
		if lines[key].value == "" {
			return nil, fmt.Errorf("credential file %s has no value for %s", path, key)
		}
		creds[key] = lines[key].value
	}
	return creds, nil
}
