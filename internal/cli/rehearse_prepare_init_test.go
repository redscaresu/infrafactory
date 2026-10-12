package cli

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// initRun runs prepare, built from p, on a repo whose envs/prod passes
// gate 1, with a runner that records each call and then runs tofu.
func initRun(t *testing.T, p rehearsePrepare, tofu func(context.Context, harness.Command) (harness.CommandResult, error)) (string, prepareRun) {
	t.Helper()
	repo := lockRepo(t, prepareLock, "")
	var run prepareRun
	p.runner = harness.CommandRunnerFunc(func(ctx context.Context, c harness.Command) (harness.CommandResult, error) {
		run.calls = append(run.calls, c)
		return tofu(ctx, c)
	})
	p.wd, p.newID = repo, func() string { return prepareID }
	root := prepareRoot(p)
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetArgs(append([]string{"rehearse", "prepare", "envs/prod"}, prepareFlags...))
	run.err = root.Execute()
	run.stdout = stdout.String()
	return repo, run
}

func TestRehearsePrepareFirstInitCommands(t *testing.T) {
	t.Parallel()
	var homes []string
	repo, run := initRun(t, rehearsePrepare{}, func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
		entries, err := os.ReadDir(c.Env["HOME"])
		require.NoError(t, err)
		assert.Empty(t, entries, "HOME is not empty")
		homes = append(homes, c.Env["HOME"])
		if c.Args[0] == "init" {
			// tofu writes with its own modes; prepare tightens them after.
			bin := filepath.Join(c.Env["TF_DATA_DIR"], "providers", "aws", "terraform-provider-aws")
			require.NoError(t, os.MkdirAll(filepath.Dir(bin), 0o755))
			require.NoError(t, os.WriteFile(bin, []byte("bin"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(c.Env["TF_DATA_DIR"], "providers", "LICENSE"), []byte("l"), 0o644))
		}
		return harness.CommandResult{}, nil
	})
	require.NoError(t, run.err)

	rehearsal, err := filepath.EvalSymlinks(preparedDir(repo))
	require.NoError(t, err)
	work := filepath.Join(rehearsal, "work")
	require.Len(t, run.calls, 3)
	assert.Equal(t, []string{"get", "-no-color"}, run.calls[0].Args)
	assert.Equal(t, []string{"providers", "lock", "-platform=" + runtime.GOOS + "_" + runtime.GOARCH}, run.calls[1].Args)
	assert.Equal(t, []string{"init", "-backend=false", "-input=false", "-no-color"}, run.calls[2].Args)
	for _, c := range run.calls {
		assert.Equal(t, "tofu", c.Name)
		assert.Equal(t, filepath.Join(work, "envs", "prod"), c.Dir)
		assert.True(t, c.ReplaceEnv, "%v runs with an inherited environment", c.Args)
		assert.Empty(t, c.StripEnv)
		assert.Equal(t, []string{"GIT_ALLOW_PROTOCOL", "GIT_CONFIG_NOSYSTEM", "GIT_TERMINAL_PROMPT", "HOME", "TF_DATA_DIR"}, slices.Sorted(maps.Keys(c.Env)))
		assert.Equal(t, map[string]string{
			"HOME": filepath.Join(rehearsal, "home"), "TF_DATA_DIR": filepath.Join(rehearsal, "data"),
			"GIT_CONFIG_NOSYSTEM": "1", "GIT_TERMINAL_PROMPT": "0", "GIT_ALLOW_PROTOCOL": "file",
		}, c.Env)
		rel, err := filepath.Rel(work, c.Env["TF_DATA_DIR"])
		require.NoError(t, err)
		assert.False(t, filepath.IsLocal(rel), "TF_DATA_DIR %s lies inside %s", c.Env["TF_DATA_DIR"], work)
	}
	assert.Len(t, homes, 3)
	assert.Contains(t, run.stdout, "Status: success\n")

	require.NoError(t, filepath.WalkDir(rehearsal, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		info, err := d.Info()
		require.NoError(t, err)
		want := fs.FileMode(0o600)
		switch {
		case d.IsDir():
			want = fs.ModeDir | 0o700
		case d.Name() == "terraform-provider-aws":
			want = 0o700
		}
		assert.Equal(t, want, info.Mode(), path)
		return nil
	}))
}

func TestRehearsePrepareRefusesAChangedLockVersion(t *testing.T) {
	t.Parallel()
	repo, run := initRun(t, rehearsePrepare{}, func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
		if c.Args[0] == "providers" {
			lock := lockEntry("registry.opentofu.org/hashicorp/aws", "5.99.0")
			require.NoError(t, os.WriteFile(filepath.Join(c.Dir, rehearsalLockFile), []byte(lock), 0o644))
		}
		return harness.CommandResult{}, nil
	})

	require.Error(t, run.err)
	assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
	assert.Contains(t, run.err.Error(), "changed the locked hashicorp/aws version from "+harness.AWSProviderVersion+" to 5.99.0")
	assert.NoDirExists(t, preparedDir(repo))
}

func TestRehearsePrepareTofuFailureExitsOne(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// failAt is the tofu subcommand that fails.
		failAt string
		result harness.CommandResult
		err    error
		want   string
	}{
		{"a cancel", "providers", harness.CommandResult{ExitCode: -1}, context.Canceled, "tofu providers lock -platform=" + runtime.GOOS + "_" + runtime.GOARCH + " failed: context canceled"},
		{"a tofu error", "init", harness.CommandResult{Stderr: []byte("Error: Failed to query available provider packages\n"), ExitCode: 1}, errors.New("exit status 1"),
			"tofu init -backend=false -input=false -no-color failed: exit status 1\nError: Failed to query available provider packages"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo, run := initRun(t, rehearsePrepare{}, func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
				if c.Args[0] == tc.failAt {
					return tc.result, tc.err
				}
				return harness.CommandResult{}, nil
			})

			require.Error(t, run.err)
			assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
			assert.Contains(t, run.stdout, "Status: failed\nRefused: "+tc.want)
			assert.NotContains(t, run.stdout, "Rehearsal:")
			assert.NotContains(t, run.stdout, "Work dir:")
			assert.NoDirExists(t, preparedDir(repo))
		})
	}
}

// A signal cancels the first init through prepare's guard, so the runner
// would not raise it again: prepare exits 1 with the refusal and removes
// the rehearsal. Not parallel: signalGuards is process-wide.
func TestRehearsePrepareSignalCancelsUnderAGuard(t *testing.T) {
	before := signalGuards.Load()
	var guards []int32
	caught := func(ctx context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		return ctx, cancel
	}
	repo, run := initRun(t, rehearsePrepare{notify: caught}, func(ctx context.Context, _ harness.Command) (harness.CommandResult, error) {
		guards = append(guards, signalGuards.Load())
		return harness.CommandResult{ExitCode: -1}, ctx.Err()
	})

	require.Error(t, run.err)
	assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
	assert.Equal(t, []int32{before + 1}, guards, "the tofu call ran outside a guard")
	assert.Contains(t, run.stdout, "Status: failed\nRefused: tofu get -no-color failed: context canceled")
	assert.NoDirExists(t, preparedDir(repo))
	assert.Equal(t, before, signalGuards.Load(), "the guard was not released")
}
