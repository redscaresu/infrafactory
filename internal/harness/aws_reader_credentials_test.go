package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

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
	readerGluedID     = "AKIAGLUEDKEYID000888"
	readerSpaced      = "Sp4ced/S3cret+Mm2Q"
	fragmentLen       = 8
)

var readerPlanted = []string{readerKeyID, readerSecret, readerToken, readerWrappedTail, readerReversed, readerReversedID, readerProfile, readerDuplicate, readerGluedID, readerSpaced}

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
		}, "line 4 has an unrecognised key"},
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

// A FIFO at the key file's path is refused, not waited on: the open must
// not block for a writer before the regular-file check runs.
func TestAWSReaderConfigRefusesAFIFOWithoutBlocking(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "reader.env")
	require.NoError(t, syscall.Mkfifo(path, 0o600))

	done := make(chan error, 1)
	go func() {
		_, err := AWSReaderConfig(path, "eu-west-2")
		done <- err
	}()
	select {
	case err := <-done:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a regular file")
	case <-time.After(5 * time.Second):
		// Opening the write end releases the blocked open.
		if w, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			_ = w.Close()
		}
		t.Fatal("opening a FIFO blocked: the reader must not wait for a writer")
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

// foreignOwned is a real file's stat that says another user owns it.
type foreignOwned struct{ os.FileInfo }

func (f foreignOwned) Sys() any {
	st := *f.FileInfo.Sys().(*syscall.Stat_t)
	st.Uid++
	return &st
}

// ownerUnknown is a real file's stat that does not say who owns it.
type ownerUnknown struct{ os.FileInfo }

func (ownerUnknown) Sys() any { return nil }

// Another user's file cannot be made without root, so the owner check
// is tested on a real 0600 file's stat with only its owner changed.
func TestCheckOwnerOnlyFileRefusesAFileYouDoNotOwn(t *testing.T) {
	t.Parallel()

	path := readerCredFile(t, validReaderCreds)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, checkOwnerOnlyFile(path, info), "your own 0600 file passes")

	err = checkOwnerOnlyFile(path, foreignOwned{info})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is owned by uid")

	err = checkOwnerOnlyFile(path, ownerUnknown{info})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot tell who owns it")
}

func TestReadOwnerOnlyFileRefusesAFileOverTheCap(t *testing.T) {
	t.Parallel()

	data, err := readOwnerOnlyFile(readerCredFile(t, strings.Repeat("#", credentialFileMaxBytes)))
	require.NoError(t, err)
	assert.Len(t, data, credentialFileMaxBytes)

	data, err = readOwnerOnlyFile(readerCredFile(t, strings.Repeat("#", credentialFileMaxBytes+1)))
	require.Error(t, err)
	assert.Nil(t, data)
	assert.Contains(t, err.Error(), "is over 4096 bytes")
}
