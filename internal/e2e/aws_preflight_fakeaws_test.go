package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// The identity fakeaws's STS answers for every caller: its one synthetic
// account and fixed IAM user (fakeaws handlers/sts.go,
// CRITICAL[sts-caller-identity-fake-account]).
const (
	fakeawsAccount   = "000000000000"
	fakeawsPrincipal = "arn:aws:iam::000000000000:user/fakeaws"
)

// TestAWSPreflightAgainstFakeaws runs the Layer 3 identity check through
// the real sealed env and SDK client against fakeaws STS, so a change on
// either side of the wire fails here rather than against real AWS. The
// required CI job runs it by name and fails if it does not report PASS.
func TestAWSPreflightAgainstFakeaws(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)

	credFile := filepath.Join(t.TempDir(), "layer3-aws.env")
	require.NoError(t, os.WriteFile(credFile, []byte("AWS_ACCESS_KEY_ID=fakeaws-key-id\nAWS_SECRET_ACCESS_KEY=fakeaws-secret\n"), 0o600))
	env, err := harness.AWSSealedEnv(credFile, "eu-west-2")
	require.NoError(t, err)

	verify := func(account, principal string) error {
		return harness.VerifyAWSIdentity(context.Background(), env, harness.NewLoopbackOnlyHTTPClient(), mock.URL+"/sts", account, principal)
	}

	t.Run("fakeaws identity passes", func(t *testing.T) {
		assert.NoError(t, verify(fakeawsAccount, fakeawsPrincipal))
	})

	t.Run("another account refuses", func(t *testing.T) {
		err := verify("111111111111", fakeawsPrincipal)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `account "000000000000"`)
		assert.Contains(t, err.Error(), `configured account "111111111111"`)
	})

	t.Run("another principal refuses", func(t *testing.T) {
		err := verify(fakeawsAccount, "arn:aws:iam::000000000000:user/admin")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `principal "`+fakeawsPrincipal+`"`)
		assert.Contains(t, err.Error(), `configured principal "arn:aws:iam::000000000000:user/admin"`)
	})
}
