package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Distinctive values planted in reader key files. No refusal may contain
// any of them, or any fragmentLen-character piece of one.
const (
	readerKeyID       = "AKIAQZ7RDR00READER01"
	readerSecret      = "rDr/s3cr3t+Zq9Vx4Lw8"
	readerToken       = "IQoJb3JpZ2luX2Vj/Pl4nted+T0k=="
	readerWrappedTail = "Wr4pPedT0kenTail+/xQ=="
	readerReversed    = "R3v/ersedS3cret+Kq8Lp"
	readerReversedID  = "AKIAREVERSEDKEYID777"
	readerProfile     = "Pr0f1leV4lue+zz"
	readerDuplicate   = "D2pl/icateSecret+99"
	fragmentLen       = 8
)

var readerPlanted = []string{readerKeyID, readerSecret, readerToken, readerWrappedTail, readerReversed, readerReversedID, readerProfile, readerDuplicate}

const validReaderCreds = "AWS_ACCESS_KEY_ID=" + readerKeyID + "\nAWS_SECRET_ACCESS_KEY=" + readerSecret + "\n"

func readerCredFile(t *testing.T, content string) string {
	t.Helper()
	return writeAWSCredFile(t, content, 0o600)
}

// assertNoPlantedFragment fails when msg holds any planted value or any
// fragmentLen-character piece of one.
func assertNoPlantedFragment(t *testing.T, msg string) {
	t.Helper()
	for _, planted := range readerPlanted {
		for i := 0; i+fragmentLen <= len(planted); i++ {
			assert.NotContains(t, msg, planted[i:i+fragmentLen], "a refusal must never echo a value from the file")
		}
	}
}

func TestAWSReaderConfigRefuses(t *testing.T) {
	t.Parallel()

	withToken := validReaderCreds + "AWS_SESSION_TOKEN=" + readerToken + "\n"
	cases := []struct {
		name     string
		credFile func(t *testing.T) string
		reason   string
	}{
		{"symlink", func(t *testing.T) string {
			link := filepath.Join(t.TempDir(), "link.env")
			require.NoError(t, os.Symlink(readerCredFile(t, withToken), link))
			return link
		}, "not a regular file"},
		{"directory", func(t *testing.T) string { return t.TempDir() }, "not a regular file"},
		{"mode 0640", func(t *testing.T) string { return writeAWSCredFile(t, withToken, 0o640) }, "mode 0640"},
		{"mode 0604", func(t *testing.T) string { return writeAWSCredFile(t, withToken, 0o604) }, "mode 0604"},
		{"unknown key", func(t *testing.T) string {
			return readerCredFile(t, withToken+"AWS_PROFILE="+readerProfile+"\n")
		}, `line 4 has unknown key "AWS_PROFILE"`},
		{"lower-case unknown key", func(t *testing.T) string {
			return readerCredFile(t, withToken+"aws_profile="+readerProfile+"\n")
		}, "line 4 has an unrecognised key"},
		{"duplicate key", func(t *testing.T) string {
			return readerCredFile(t, withToken+"AWS_SECRET_ACCESS_KEY="+readerDuplicate+"\n")
		}, "sets AWS_SECRET_ACCESS_KEY twice"},
		{"missing secret", func(t *testing.T) string {
			return readerCredFile(t, "AWS_ACCESS_KEY_ID="+readerKeyID+"\nAWS_SESSION_TOKEN="+readerToken+"\n")
		}, "has no AWS_SECRET_ACCESS_KEY"},
		{"empty value", func(t *testing.T) string {
			return readerCredFile(t, "AWS_ACCESS_KEY_ID=\nAWS_SECRET_ACCESS_KEY="+readerSecret+"\n")
		}, "line 1 has no value for AWS_ACCESS_KEY_ID"},
		{"empty AWS_SESSION_TOKEN", func(t *testing.T) string {
			return readerCredFile(t, validReaderCreds+"AWS_SESSION_TOKEN=\n")
		}, "line 3 has no value for AWS_SESSION_TOKEN"},
		{"wrapped token line", func(t *testing.T) string {
			return readerCredFile(t, withToken+readerWrappedTail+"\n")
		}, "line 4 has an unrecognised key"},
		{"reversed secret line", func(t *testing.T) string {
			return readerCredFile(t, "AWS_ACCESS_KEY_ID="+readerKeyID+"\n"+readerReversed+"=AWS_SECRET_ACCESS_KEY\n")
		}, "line 2 has an unrecognised key"},
		{"reversed key id line", func(t *testing.T) string {
			return readerCredFile(t, readerReversedID+"=AWS_ACCESS_KEY_ID\n")
		}, "line 1 has an unrecognised key"},
		{"not KEY=VALUE", func(t *testing.T) string {
			return readerCredFile(t, withToken+"garbage\n")
		}, "line 4 is not KEY=VALUE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := tc.credFile(t)
			cfg, err := AWSReaderConfig(path, "eu-west-2")
			require.Error(t, err)
			assert.Nil(t, cfg.Credentials)
			assert.Contains(t, err.Error(), tc.reason)
			assertNoPlantedFragment(t, strings.ReplaceAll(err.Error(), path, ""))
		})
	}
}

func TestAWSReaderConfigRefusesMalformedRegions(t *testing.T) {
	t.Parallel()

	for _, region := range []string{"", "x@evil.example/", "us-east-1.evil.com"} {
		cfg, err := AWSReaderConfig(readerCredFile(t, validReaderCreds), region)
		require.Error(t, err, region)
		assert.Nil(t, cfg.Credentials, region)
		assert.Contains(t, err.Error(), "not an AWS region name", region)
	}
}

// The token reaches the wire only through the signer, so the test signs
// a real STS request through the built config and reads the header.
func TestAWSReaderConfigSignsWithTheSessionTokenOnlyWhenTheFileHasOne(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, content, token string
	}{
		{"with a session token", validReaderCreds + "AWS_SESSION_TOKEN=" + readerToken + "\n", readerToken},
		{"without a session token", validReaderCreds, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := AWSReaderConfig(readerCredFile(t, tc.content), "eu-west-2")
			require.NoError(t, err)
			assert.Equal(t, "eu-west-2", cfg.Region)

			creds, err := cfg.Credentials.Retrieve(context.Background())
			require.NoError(t, err)
			assert.Equal(t, readerKeyID, creds.AccessKeyID)
			assert.Equal(t, readerSecret, creds.SecretAccessKey)
			assert.Equal(t, tc.token, creds.SessionToken)

			doer := &capturingDoer{}
			client := sts.NewFromConfig(cfg, func(o *sts.Options) {
				o.BaseEndpoint = aws.String("https://sts.eu-west-2.amazonaws.com")
				o.HTTPClient = doer
			})
			_, err = client.GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})
			require.NoError(t, err)
			require.Len(t, doer.requests, 1)

			header := doer.requests[0].Header
			assert.Contains(t, header.Get("Authorization"), "Credential="+readerKeyID+"/")
			if tc.token == "" {
				assert.NotContains(t, header, "X-Amz-Security-Token")
				return
			}
			assert.Equal(t, tc.token, header.Get("X-Amz-Security-Token"))
		})
	}
}
