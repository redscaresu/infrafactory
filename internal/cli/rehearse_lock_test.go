package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

const lockName = "envs/prod/.terraform.lock.hcl"

// lockRepo is a repo whose envs/prod passes gate 1, with lock as its lock
// ("" for none) and extra beside main.tf.
func lockRepo(t *testing.T, lock, extra string) string {
	t.Helper()
	files := map[string]string{"envs/prod/main.tf": prepareProvider + extra}
	if lock != "" {
		files[lockName] = lock
	}
	_, repo := prepareRepo(t, files)
	return repo
}

func TestRehearsePrepareRefusesTheLockByName(t *testing.T) {
	t.Parallel()
	aws := func(version string) string { return lockEntry("registry.opentofu.org/hashicorp/aws", version) }

	for _, tc := range []struct {
		name, lock, want string
	}{
		{"a missing lock", "", lockName + ": cannot read it"},
		{"an unparseable lock", "provider \"registry.opentofu.org/hashicorp/aws\" {\n", lockName + ": does not parse"},
		{"a lock without aws", lockEntry("registry.opentofu.org/hashicorp/random", "3.6.0"), lockName + ": refused: it has no hashicorp/aws entry"},
		{"a lock naming hashicorp/http", prepareLock + lockEntry("registry.opentofu.org/hashicorp/http", "3.4.0"), `the lock names provider "registry.opentofu.org/hashicorp/http"`},
		{"a lock from another registry", lockEntry("example.com/hashicorp/aws", harness.AWSProviderVersion), `the lock names provider "example.com/hashicorp/aws"`},
		{"aws entries at different versions", aws(harness.AWSProviderVersion) + lockEntry("registry.terraform.io/hashicorp/aws", "5.99.0"), "pin different versions (5.100.0, 5.99.0)"},
		{"a later aws release", aws("6.0.0"), "hashicorp/aws 6.0.0 is later than " + harness.AWSProviderVersion + " or of another major"},
		{"an earlier major", aws("4.67.0"), "hashicorp/aws 4.67.0 is later than " + harness.AWSProviderVersion + " or of another major"},
		{"a pre-release", aws("5.100.0-beta1"), `hashicorp/aws "5.100.0-beta1" is not an exact X.Y.Z release`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := lockRepo(t, tc.lock, "")

			run := runPrepare(repo, append([]string{"envs/prod"}, prepareFlags...)...)

			require.ErrorIs(t, run.err, errRehearsalRefuses)
			assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
			assert.Contains(t, run.err.Error(), tc.want)
			assert.Empty(t, run.calls)
			assert.NoDirExists(t, preparedDir(repo))
		})
	}
}

// Under a lock pinning another aws version, an argument the table does not
// name is still refused, and the refusal names the lock's version.
func TestRehearsePrepareRefusesAnUnknownArgumentNamingTheLockVersion(t *testing.T) {
	t.Parallel()
	repo := lockRepo(t, lockEntry("registry.opentofu.org/hashicorp/aws", "5.99.0"),
		"provider \"aws\" {\n  alias       = \"other\"\n  region      = \"eu-west-1\"\n  made_up_arg = 1\n}\n")

	run := runPrepare(repo, append([]string{"envs/prod"}, prepareFlags...)...)

	require.ErrorIs(t, run.err, errRehearsalRefuses)
	assert.Contains(t, run.err.Error(), "provider.aws.made_up_arg: refused: the gate's table of hashicorp/aws "+harness.AWSProviderVersion+
		" provider arguments does not name it, so it is refused under any lock version, the lock's 5.99.0 included")
	assert.Empty(t, run.calls)
}

func TestRehearsePrepareReportsTheLock(t *testing.T) {
	t.Parallel()
	const terraformWarning = lockName + ": the hashicorp/aws entry is registry.terraform.io's, which Terraform wrote: moved, import, removed and ephemeral semantics may differ under tofu"
	const olderWarning = lockName + ": the lock pins hashicorp/aws 5.99.0, not " + harness.AWSProviderVersion + ", the release fakeaws is proven against: a failure under it is a fakeaws gap, to file as a fakeaws story"

	for _, tc := range []struct {
		name, lock, version string
		warnings            []string
	}{
		{"a registry.terraform.io lock", lockEntry("registry.terraform.io/hashicorp/aws", harness.AWSProviderVersion), harness.AWSProviderVersion, []string{terraformWarning}},
		{"an earlier aws release", lockEntry("registry.opentofu.org/hashicorp/aws", "5.99.0"), "5.99.0", []string{olderWarning}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			human := runPrepare(lockRepo(t, tc.lock, ""), append([]string{"envs/prod"}, prepareFlags...)...)
			require.NoError(t, human.err)
			assert.Contains(t, human.stdout, "AWS provider: hashicorp/aws "+tc.version+"\n")
			for _, warning := range tc.warnings {
				assert.Contains(t, human.stdout, "Warning: "+warning+"\n")
			}

			jsonRun := runPrepare(lockRepo(t, tc.lock, ""), append([]string{"envs/prod", "--output", "json"}, prepareFlags...)...)
			require.NoError(t, jsonRun.err)
			var result rehearsePrepareResult
			require.NoError(t, json.Unmarshal([]byte(jsonRun.stdout), &result))
			assert.Equal(t, tc.version, result.AWSVersion)
			assert.Equal(t, tc.warnings, result.Warnings)
		})
	}
}
