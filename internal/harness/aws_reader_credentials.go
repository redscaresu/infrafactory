package harness

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

const (
	awsReaderKeyID  = "AWS_ACCESS_KEY_ID"
	awsReaderSecret = "AWS_SECRET_ACCESS_KEY"
	awsReaderToken  = "AWS_SESSION_TOKEN"

	// credentialFileMaxBytes caps what readOwnerOnlyFile reads: a key
	// file is a few short lines.
	credentialFileMaxBytes = 4096
)

// awsCredentialKeyNames are every key a credential file may hold, and
// the only key names a refusal prints; it prints them from here.
var awsCredentialKeyNames = []string{awsReaderKeyID, awsReaderSecret, awsReaderToken}

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

// readAWSReaderCredentials requires the key id and the secret, and takes
// the session token when there is one. An empty value is refused on its
// line, never read as absent.
func readAWSReaderCredentials(path string) (aws.Credentials, error) {
	data, err := readOwnerOnlyFile(path)
	if err != nil {
		return aws.Credentials{}, err
	}
	lines, err := parseCredentialLines(path, data, awsCredentialKeyNames)
	if err != nil {
		return aws.Credentials{}, err
	}
	for _, key := range awsCredentialKeyNames {
		if l, ok := lines[key]; ok && l.value == "" {
			return aws.Credentials{}, fmt.Errorf("credential file %s line %d has no value for %s", path, l.line, key)
		}
	}
	for _, key := range []string{awsReaderKeyID, awsReaderSecret} {
		if _, ok := lines[key]; !ok {
			return aws.Credentials{}, fmt.Errorf("credential file %s has no %s", path, key)
		}
	}
	return aws.Credentials{
		AccessKeyID:     lines[awsReaderKeyID].value,
		SecretAccessKey: lines[awsReaderSecret].value,
		SessionToken:    lines[awsReaderToken].value,
	}, nil
}

// credentialLine is one KEY=VALUE line of a credential file.
type credentialLine struct {
	line  int
	value string
}

// parseCredentialLines reads KEY=VALUE lines, skipping blanks and #
// comments, and refuses a malformed line, a key not in allowed and a key
// set twice. Required keys and empty values are the caller's rules. No
// refusal holds anything from a line: a wrapped secret, a reversed
// VALUE=KEY line or a key glued to its value would put a secret in it.
func parseCredentialLines(path string, data []byte, allowed []string) (map[string]credentialLine, error) {
	lines := map[string]credentialLine{}
	for i, text := range strings.Split(string(data), "\n") {
		text = strings.TrimSpace(text)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		if !ok {
			return nil, fmt.Errorf("credential file %s line %d is not KEY=VALUE", path, i+1)
		}
		if !slices.Contains(allowed, key) {
			return nil, unknownCredentialKey(path, i+1, key, allowed)
		}
		if _, seen := lines[key]; seen {
			return nil, fmt.Errorf("credential file %s sets %s twice", path, key)
		}
		lines[key] = credentialLine{line: i + 1, value: value}
	}
	return lines, nil
}

// unknownCredentialKey refuses a key that is not in allowed. It never
// prints key: it names only an awsCredentialKeyNames entry, taken from
// that list.
func unknownCredentialKey(path string, line int, key string, allowed []string) error {
	if slices.Contains(allowed, strings.TrimSpace(key)) {
		return fmt.Errorf("credential file %s line %d: no spaces around '='", path, line)
	}
	only := strings.Join(allowed, ", ")
	if i := slices.Index(awsCredentialKeyNames, key); i >= 0 {
		return fmt.Errorf("credential file %s line %d has unknown key %q; allowed keys: %s", path, line, awsCredentialKeyNames[i], only)
	}
	return fmt.Errorf("credential file %s line %d has an unrecognised key; allowed keys: %s", path, line, only)
}

// ownerOnlyFileCheckedForTest runs between readOwnerOnlyFile's checks and
// its read, so a test can swap the file there. Test-only: nothing else
// sets it, and a test that sets it must not call t.Parallel, since every
// credential read in the package runs it.
var ownerOnlyFileCheckedForTest = func() {}

// readOwnerOnlyFile opens path once and checks and reads that handle, so
// nothing swapped in after the check is read. O_NOFOLLOW refuses a
// symlink as the final path element (ELOOP); O_NONBLOCK keeps a FIFO from
// blocking the open before checkOwnerOnlyFile refuses it. It reads at
// most credentialFileMaxBytes and refuses a longer file.
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
	if err := checkOwnerOnlyFile(path, info); err != nil {
		return nil, err
	}
	ownerOnlyFileCheckedForTest()
	data, err := io.ReadAll(io.LimitReader(f, credentialFileMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("credential file %s: %w", path, err)
	}
	if len(data) > credentialFileMaxBytes {
		return nil, fmt.Errorf("credential file %s is over %d bytes", path, credentialFileMaxBytes)
	}
	return data, nil
}

// checkOwnerOnlyFile passes only a regular file with no group or other
// permission, owned by this process's user. A stat that does not say who
// owns the file is refused.
func checkOwnerOnlyFile(path string, info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("credential file %s is not a regular file (%s)", path, info.Mode().Type())
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("credential file %s has mode %04o; it must grant no group or other permission (chmod 600)", path, perm)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return fmt.Errorf("credential file %s: cannot tell who owns it", path)
	}
	if uid := os.Getuid(); int(st.Uid) != uid {
		return fmt.Errorf("credential file %s is owned by uid %d, not by you (uid %d)", path, st.Uid, uid)
	}
	return nil
}
