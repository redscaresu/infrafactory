package harness

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

const (
	awsReaderKeyID   = "AWS_ACCESS_KEY_ID"
	awsReaderSecret  = "AWS_SECRET_ACCESS_KEY"
	awsReaderToken   = "AWS_SESSION_TOKEN"
	awsReaderAllowed = awsReaderKeyID + ", " + awsReaderSecret + " and " + awsReaderToken
)

// awsReaderNameableKey is the only shape of unknown key an error may
// name. A key id, secret or session token contains no underscore and
// never starts AWS_, so a value on its own (a reversed or wrapped line)
// never matches. A key name glued to a key id (AWS_AKIA...=x) would match
// and be named.
var awsReaderNameableKey = regexp.MustCompile(`^AWS_[A-Z0-9_]+$`)

// AWSReaderConfig builds the aws.Config for a read-only account read
// from the reader key file alone: region from the caller, and a static
// provider carrying the file's session token when it has one. Like
// sealedAWSConfig it never reads the process env or ~/.aws.
//
// credFile is the reader's own key file, separate from layer3-aws.env:
// KEY=VALUE lines holding AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and
// optionally AWS_SESSION_TOKEN, readable by its owner alone.
func AWSReaderConfig(credFile, region string) (aws.Config, error) {
	if !ValidAWSRegion(region) {
		return aws.Config{}, fmt.Errorf("aws reader config: region %q is not an AWS region name", region)
	}
	creds, err := readAWSReaderCredentials(credFile)
	if err != nil {
		return aws.Config{}, fmt.Errorf("aws reader config: %w", err)
	}
	provider := credentials.NewStaticCredentialsProvider(creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken)
	return aws.Config{
		Region:      region,
		Credentials: aws.NewCredentialsCache(provider),
	}, nil
}

// readAWSReaderCredentials follows readAWSCredentialFile's rules. It
// never puts a value in an error, nor a key awsReaderNameableKey does not
// match: either could be a secret.
func readAWSReaderCredentials(path string) (aws.Credentials, error) {
	data, err := readOwnerOnlyFile(path)
	if err != nil {
		return aws.Credentials{}, err
	}

	values := map[string]string{}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return aws.Credentials{}, fmt.Errorf("credential file %s line %d is not KEY=VALUE", path, i+1)
		}
		switch key {
		case awsReaderKeyID, awsReaderSecret, awsReaderToken:
		default:
			if awsReaderNameableKey.MatchString(key) {
				return aws.Credentials{}, fmt.Errorf("credential file %s line %d has unknown key %q; only %s are allowed", path, i+1, key, awsReaderAllowed)
			}
			return aws.Credentials{}, fmt.Errorf("credential file %s line %d has an unrecognised key; only %s are allowed", path, i+1, awsReaderAllowed)
		}
		if _, seen := values[key]; seen {
			return aws.Credentials{}, fmt.Errorf("credential file %s sets %s twice", path, key)
		}
		// An empty AWS_SESSION_TOKEN= is refused, never read as absent.
		if value == "" {
			return aws.Credentials{}, fmt.Errorf("credential file %s line %d has no value for %s", path, i+1, key)
		}
		values[key] = value
	}
	for _, key := range []string{awsReaderKeyID, awsReaderSecret} {
		if values[key] == "" {
			return aws.Credentials{}, fmt.Errorf("credential file %s has no %s", path, key)
		}
	}
	return aws.Credentials{
		AccessKeyID:     values[awsReaderKeyID],
		SecretAccessKey: values[awsReaderSecret],
		SessionToken:    values[awsReaderToken],
	}, nil
}

// readOwnerOnlyFile reads path only when it is a regular file, not a
// symlink, granting no group or other permission. It opens path once and
// checks and reads that handle, so nothing swapped in after the check is
// read. O_NOFOLLOW refuses a symlink (ELOOP); O_NONBLOCK keeps a FIFO
// from blocking the open before the regular-file check refuses it.
func readOwnerOnlyFile(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, syscall.ELOOP) {
		return nil, fmt.Errorf("credential file %s is not a regular file (a symlink)", path)
	}
	if err != nil {
		return nil, fmt.Errorf("credential file %s: %w", path, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("credential file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("credential file %s is not a regular file (%s)", path, info.Mode().Type())
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return nil, fmt.Errorf("credential file %s has mode %04o; it must grant no group or other permission (chmod 600)", path, perm)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("credential file %s: %w", path, err)
	}
	return data, nil
}
