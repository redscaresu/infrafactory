package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

const prepareID = "rehearsal-id"

var prepareFlags = []string{"--account", "123456789012", "--region", "eu-west-1"}

// prepareProvider is the root provider "aws" block goal 11 requires, at
// prepareFlags' region.
const prepareProvider = "provider \"aws\" {\n  region = \"eu-west-1\"\n}\n"

// prepareRoot is the root command with prepare built from p.
func prepareRoot(p rehearsePrepare) *cobra.Command {
	root := NewRootCmd()
	group, _, _ := root.Find([]string{"rehearse"})
	prepare, _, _ := group.Find([]string{"prepare"})
	group.RemoveCommand(prepare)
	group.AddCommand(newRehearsePrepareCmd(p))
	root.SetErr(&bytes.Buffer{})
	return root
}

type prepareRun struct {
	stdout string
	calls  []harness.Command
	err    error
}

// runPrepare runs `rehearse prepare args` from wd with a recording runner
// and the id prepareID.
func runPrepare(wd string, args ...string) prepareRun {
	var run prepareRun
	runner := harness.CommandRunnerFunc(func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
		run.calls = append(run.calls, c)
		return harness.CommandResult{}, nil
	})
	root := prepareRoot(rehearsePrepare{runner: runner, wd: wd, newID: func() string { return prepareID }})
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetArgs(append([]string{"rehearse", "prepare"}, args...))
	run.err = root.Execute()
	run.stdout = stdout.String()
	return run
}

// prepareRepo writes files under <tmp>/repo, which holds a .git dir, and
// returns tmp and the repo.
func prepareRepo(t *testing.T, files map[string]string) (string, string) {
	t.Helper()
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))
	for name, content := range files {
		mustWriteFile(t, filepath.Join(repo, name), content)
	}
	return tmp, repo
}

func preparedDir(repo string) string {
	return filepath.Join(repo, ".infrafactory", "rehearsals", prepareID)
}

func TestRehearsePrepareUsageErrorsExitTwo(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"a missing --account", []string{"envs/prod", "--region", "eu-west-1"}},
		{"an 11-digit account", []string{"envs/prod", "--account", "12345678901", "--region", "eu-west-1"}},
		{"a missing --region", []string{"envs/prod", "--account", "123456789012"}},
		{"a bad region", []string{"envs/prod", "--account", "123456789012", "--region", "eu-west"}},
		{"two dirs", append([]string{"envs/prod", "envs/stage"}, prepareFlags...)},
		{"an --allow-module flag", append([]string{"envs/prod", "--allow-module", "x"}, prepareFlags...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, repo := prepareRepo(t, map[string]string{"envs/prod/main.tf": "resource \"terraform_data\" \"x\" {}\n"})

			run := runPrepare(repo, tc.args...)

			var cliErr *CLIError
			require.ErrorAs(t, run.err, &cliErr)
			assert.Equal(t, errorCodeUsage, cliErr.Code)
			assert.Equal(t, ExitCodeUsage, ExitCodeForError(run.err))
			assert.NoDirExists(t, filepath.Join(repo, ".infrafactory"))
		})
	}
}

func TestRehearsePrepareCopiesTheRootAndItsModules(t *testing.T) {
	t.Parallel()
	tmp, repo := prepareRepo(t, map[string]string{
		"README.md":                            "readme\n",
		".git/HEAD":                            "ref: refs/heads/main\n",
		".infrafactory/runs/x":                 "run\n",
		"envs/prod/main.tf":                    "module \"x\" {\n  source = \"../../modules/x\"\n}\n",
		"envs/prod/provider.tf":                prepareProvider,
		"envs/prod/.terraform.lock.hcl":        "# lock\n",
		"envs/prod/.terraform/modules/m.json":  "{}\n",
		"envs/prod/terraform.tfstate":          "{}\n",
		"modules/x/main.tf":                    "resource \"terraform_data\" \"x\" {}\n",
		"modules/x/terraform.tfstate.d/s/a.tf": "\n",
	})
	varFile := filepath.Join(tmp, "prod.tfvars")
	mustWriteFile(t, varFile, "size = 1\n")
	before := treeDigest(t, repo)

	run := runPrepare(repo, append([]string{"envs/prod", "--var-file", varFile, "--output", "json"}, prepareFlags...)...)
	require.NoError(t, run.err)

	assert.Equal(t, before, treeDigest(t, repo), "prepare changed the source tree")
	rehearsal := preparedDir(repo)
	work := filepath.Join(rehearsal, "work")
	var result rehearsePrepareResult
	require.NoError(t, json.Unmarshal([]byte(run.stdout), &result))
	resolvedWork, err := filepath.EvalSymlinks(work)
	require.NoError(t, err)
	assert.Equal(t, rehearsePrepareResult{Schema: rehearsePrepareSchema, Status: CommandStatusSuccess, ID: prepareID, WorkDir: filepath.Join(resolvedWork, "envs", "prod")}, result)

	for name, want := range map[string]string{
		"envs/prod/main.tf":             "module \"x\" {\n  source = \"../../modules/x\"\n}\n",
		"envs/prod/.terraform.lock.hcl": "# lock\n",
		"modules/x/main.tf":             "resource \"terraform_data\" \"x\" {}\n",
		"README.md":                     "readme\n",
	} {
		got, err := os.ReadFile(filepath.Join(work, name))
		if assert.NoError(t, err, name) {
			assert.Equal(t, want, string(got), name)
		}
	}
	for _, name := range []string{".git", ".infrafactory", "envs/prod/.terraform", "envs/prod/terraform.tfstate", "modules/x/terraform.tfstate.d"} {
		_, err := os.Lstat(filepath.Join(work, name))
		assert.ErrorIs(t, err, fs.ErrNotExist, "%s was copied", name)
	}
	vars, err := os.ReadFile(filepath.Join(rehearsal, "vars.tfvars"))
	require.NoError(t, err)
	assert.Equal(t, "size = 1\n", string(vars))
	gitignore, err := os.ReadFile(filepath.Join(repo, ".infrafactory", "rehearsals", ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, "*\n", string(gitignore))

	require.NoError(t, filepath.WalkDir(filepath.Join(repo, ".infrafactory", "rehearsals"), func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		info, err := d.Info()
		require.NoError(t, err)
		want := fs.FileMode(0o600)
		if d.IsDir() {
			want = fs.ModeDir | 0o700
		}
		assert.Equal(t, want, info.Mode(), path)
		return nil
	}))
	assert.Empty(t, run.calls)
}

// treeDigest is every entry under dir but .infrafactory, which prepare
// writes to, with its mode and content hash.
func treeDigest(t *testing.T, dir string) map[string]string {
	t.Helper()
	digest := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.Name() == ".infrafactory" {
			return filepath.SkipDir
		}
		info, err := d.Info()
		require.NoError(t, err)
		content := []byte{}
		if d.Type().IsRegular() {
			content, err = os.ReadFile(path)
			require.NoError(t, err)
		}
		digest[path] = fmt.Sprintf("%v %x", info.Mode(), sha256.Sum256(content))
		return nil
	}))
	return digest
}

func TestRehearsePrepareRefusesNamingThePath(t *testing.T) {
	t.Parallel()
	const prod = "resource \"terraform_data\" \"x\" {}\n"

	for _, tc := range []struct {
		name string
		// setup builds the case under tmp and repo, and returns
		// prepare's args before the flags and what the refusal names.
		setup func(t *testing.T, tmp, repo string) ([]string, string)
	}{
		{"a dir outside the current directory's repository", func(t *testing.T, tmp, _ string) ([]string, string) {
			outside := filepath.Join(tmp, "outside")
			mustWriteFile(t, filepath.Join(outside, "main.tf"), prod)
			return []string{outside}, outside
		}},
		{"a module source escaping the repo", func(t *testing.T, tmp, repo string) ([]string, string) {
			mustWriteFile(t, filepath.Join(tmp, "outside", "main.tf"), prod)
			mustWriteFile(t, filepath.Join(repo, "envs/prod/main.tf"), "module \"x\" {\n  source = \"../../../outside\"\n}\n")
			return []string{"envs/prod"}, `source "../../../outside" resolves to`
		}},
		{"a symlink in the tree", func(t *testing.T, _, repo string) ([]string, string) {
			mustWriteFile(t, filepath.Join(repo, "README.md"), "readme\n")
			require.NoError(t, os.Symlink(filepath.Join(repo, "README.md"), filepath.Join(repo, "envs/prod/link.md")))
			return []string{"envs/prod"}, filepath.Join("envs", "prod", "link.md") + " is refused"
		}},
		{"a symlinked .infrafactory", func(t *testing.T, tmp, repo string) ([]string, string) {
			require.NoError(t, os.MkdirAll(filepath.Join(tmp, "elsewhere"), 0o700))
			require.NoError(t, os.Symlink(filepath.Join(tmp, "elsewhere"), filepath.Join(repo, ".infrafactory")))
			return []string{"envs/prod"}, filepath.Join(repo, ".infrafactory") + " is refused"
		}},
		{"a symlinked rehearsals", func(t *testing.T, tmp, repo string) ([]string, string) {
			require.NoError(t, os.MkdirAll(filepath.Join(tmp, "elsewhere"), 0o700))
			require.NoError(t, os.MkdirAll(filepath.Join(repo, ".infrafactory"), 0o700))
			require.NoError(t, os.Symlink(filepath.Join(tmp, "elsewhere"), filepath.Join(repo, ".infrafactory", "rehearsals")))
			return []string{"envs/prod"}, filepath.Join(repo, ".infrafactory", "rehearsals") + " is refused"
		}},
		{"a .json var file", func(t *testing.T, tmp, _ string) ([]string, string) {
			varFile := filepath.Join(tmp, "prod.tfvars.json")
			mustWriteFile(t, varFile, "{}\n")
			return []string{"envs/prod", "--var-file", varFile}, varFile
		}},
		{"a missing var file, before anything is made", func(t *testing.T, tmp, _ string) ([]string, string) {
			varFile := filepath.Join(tmp, "missing.tfvars")
			return []string{"envs/prod", "--var-file", varFile}, "--var-file " + varFile + " is refused"
		}},
		{"a dir inside .infrafactory, where the rehearsals live", func(t *testing.T, _, repo string) ([]string, string) {
			mustWriteFile(t, filepath.Join(repo, ".infrafactory/main.tf"), prod)
			return []string{".infrafactory"}, "lies in .infrafactory"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tmp, repo := prepareRepo(t, map[string]string{"envs/prod/main.tf": prod})
			args, want := tc.setup(t, tmp, repo)

			run := runPrepare(repo, append(args, prepareFlags...)...)

			require.Error(t, run.err)
			assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
			assert.Contains(t, run.err.Error(), want)
			assert.NoDirExists(t, preparedDir(repo))
			assert.NoDirExists(t, filepath.Join(tmp, "elsewhere", prepareID))
			assert.NoDirExists(t, filepath.Join(tmp, "elsewhere", "rehearsals", prepareID))
		})
	}
}

func TestRehearsePrepareRefusesAnExistingID(t *testing.T) {
	t.Parallel()
	_, repo := prepareRepo(t, map[string]string{
		"envs/prod/main.tf": "resource \"terraform_data\" \"x\" {}\n",
		".infrafactory/rehearsals/" + prepareID + "/kept": "kept\n",
	})

	run := runPrepare(repo, append([]string{"envs/prod"}, prepareFlags...)...)

	require.Error(t, run.err)
	assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
	assert.Contains(t, run.err.Error(), preparedDir(repo))
	assert.FileExists(t, filepath.Join(preparedDir(repo), "kept"), "a refused prepare removed a rehearsal it did not make")
}

// Gate 1 refuses each of these on the copy, or the module walk before it,
// and no tofu runs.
func TestRehearsePrepareGateRefusesBeforeAnyTofu(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"encryption in a .tf", map[string]string{"envs/prod/versions.tf": "terraform {\n  encryption {}\n}\n"}, "versions.tf:2: terraform.encryption: refused"},
		{"a .tofu file", map[string]string{"envs/prod/main.tofu": "\n"}, "main.tofu: .tofu files are refused by name"},
		{"a .tf.json file", map[string]string{"envs/prod/main.tf.json": "{}\n"}, "main.tf.json: .tf.json files are refused by name"},
		{"cloud {}", map[string]string{"envs/prod/versions.tf": "terraform {\n  cloud {}\n}\n"}, "versions.tf:2: terraform.cloud: refused"},
		{"a git:: source", map[string]string{"envs/prod/mod.tf": "module \"x\" {\n  source = \"git::https://example.com/x.git\"\n}\n"}, `source "git::https://example.com/x.git" is refused`},
		{"a registry source with an exact version", map[string]string{"envs/prod/mod.tf": "module \"x\" {\n  source  = \"hashicorp/consul/aws\"\n  version = \"0.12.0\"\n}\n"}, `source "hashicorp/consul/aws" is refused`},
		{"an override file", map[string]string{"envs/prod/override.tf": "resource \"terraform_data\" \"y\" {}\n"}, "override.tf: an override file is refused"},
		{"a terraform.d dir", map[string]string{"envs/prod/terraform.d/plugins/x": "x\n"}, "terraform.d: a terraform.d entry is refused"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.files["envs/prod/main.tf"] = "resource \"terraform_data\" \"x\" {}\n"
			_, repo := prepareRepo(t, tc.files)

			run := runPrepare(repo, append([]string{"envs/prod"}, prepareFlags...)...)

			require.ErrorIs(t, run.err, errRehearsalRefuses)
			assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
			assert.Contains(t, run.err.Error(), tc.want)
			assert.Empty(t, run.calls)
			assert.NoDirExists(t, preparedDir(repo))
		})
	}
}
