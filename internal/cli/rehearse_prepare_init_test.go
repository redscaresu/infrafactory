package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// initRun runs prepare, built from p, on a repo whose envs/prod passes
// gate 1 and whose lock is lock, with a runner that records each call and
// then runs tofu.
func initRun(t *testing.T, p rehearsePrepare, lock string, tofu func(context.Context, harness.Command) (harness.CommandResult, error)) (string, prepareRun) {
	t.Helper()
	repo := lockRepo(t, lock, "")
	return repo, initRunIn(t, p, repo, tofu)
}

// initRunIn runs prepare, built from p, on envs/prod in repo, with a
// runner that records each call and then runs tofu.
func initRunIn(t *testing.T, p rehearsePrepare, repo string, tofu func(context.Context, harness.Command) (harness.CommandResult, error)) prepareRun {
	t.Helper()
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
	return run
}

// isFirstInit and isSecondInit name the init commands of the two inits.
func isFirstInit(c harness.Command) bool {
	return slices.Equal(c.Args, []string{"init", "-backend=false", "-input=false", "-no-color"})
}

func isSecondInit(c harness.Command) bool { return slices.Contains(c.Args, "-reconfigure") }

func TestRehearsePrepareFirstInitCommands(t *testing.T) {
	t.Parallel()
	var homes []string
	repo, run := initRun(t, rehearsePrepare{}, prepareLock, func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
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
	require.Len(t, run.calls, 4, "the first init's three tofu commands and the second init")
	assert.Equal(t, []string{"get", "-no-color"}, run.calls[0].Args)
	assert.Equal(t, []string{"providers", "lock", "-platform=" + runtime.GOOS + "_" + runtime.GOARCH}, run.calls[1].Args)
	assert.Equal(t, []string{"init", "-backend=false", "-input=false", "-no-color"}, run.calls[2].Args)
	for _, c := range run.calls[:3] {
		assert.Equal(t, "tofu", c.Name)
		assert.Equal(t, filepath.Join(work, "envs", "prod"), c.Dir)
		assert.True(t, c.ReplaceEnv, "%v runs with an inherited environment", c.Args)
		assert.Empty(t, c.StripEnv)
		assert.Equal(t, []string{"GIT_ALLOW_PROTOCOL", "GIT_CONFIG_NOSYSTEM", "GIT_TERMINAL_PROMPT", "HOME", "TF_DATA_DIR"}, slices.Sorted(maps.Keys(c.Env)))
		assert.Equal(t, map[string]string{
			"HOME": filepath.Join(rehearsal, "home"), "TF_DATA_DIR": filepath.Join(rehearsal, "data"),
			"GIT_CONFIG_NOSYSTEM": "1", "GIT_TERMINAL_PROMPT": "0", "GIT_ALLOW_PROTOCOL": "none",
		}, c.Env)
		rel, err := filepath.Rel(work, c.Env["TF_DATA_DIR"])
		require.NoError(t, err)
		assert.False(t, filepath.IsLocal(rel), "TF_DATA_DIR %s lies inside %s", c.Env["TF_DATA_DIR"], work)
	}
	assert.Len(t, homes, 4)
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

// The first init's providers lock may not move any provider's locked
// version, however its entries are spelled.
func TestRehearsePrepareRefusesAChangedLockVersion(t *testing.T) {
	t.Parallel()
	random := func(host, version string) string { return lockEntry(host+"/hashicorp/random", version) }

	for _, tc := range []struct {
		name, before, after, want string
	}{
		{"aws", prepareLock, lockEntry("registry.opentofu.org/hashicorp/aws", "5.99.0"), "hashicorp/aws from " + harness.AWSProviderVersion + " to 5.99.0"},
		{"random from a registry.terraform.io entry", prepareLock + random("registry.terraform.io", "3.6.0"),
			prepareLock + random("registry.opentofu.org", "3.7.0"), "hashicorp/random from 3.6.0 to 3.7.0"},
		{"a provider the user's lock did not pin", prepareLock, prepareLock + random("registry.opentofu.org", "3.7.0"), "hashicorp/random from none to 3.7.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo, run := initRun(t, rehearsePrepare{}, tc.before, func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
				if c.Args[0] == "providers" {
					require.NoError(t, os.WriteFile(filepath.Join(c.Dir, rehearsalLockFile), []byte(tc.after), 0o644))
				}
				return harness.CommandResult{}, nil
			})

			require.Error(t, run.err)
			assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
			assert.Contains(t, run.err.Error(), "the first init is refused: it changed the locked provider versions: "+tc.want)
			assert.NoDirExists(t, preparedDir(repo))
		})
	}
}

// An entry the first init drops is admitted: it brings in no version the
// user did not lock.
func TestRehearsePrepareAdmitsAnEntryTheFirstInitDrops(t *testing.T) {
	t.Parallel()
	before := prepareLock + lockEntry("registry.opentofu.org/hashicorp/random", "3.6.0")
	repo, run := initRun(t, rehearsePrepare{}, before, func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
		if c.Args[0] == "init" {
			require.NoError(t, os.WriteFile(filepath.Join(c.Dir, rehearsalLockFile), []byte(prepareLock), 0o644))
		}
		return harness.CommandResult{}, nil
	})

	require.NoError(t, run.err)
	assert.Contains(t, run.stdout, "Status: success\n")
	assert.DirExists(t, preparedDir(repo))
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
			repo, run := initRun(t, rehearsePrepare{}, prepareLock, func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
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

// A signal cancels prepare through its guard, whenever it arrives: during
// the copy and gate 1, which do not read ctx, prepare refuses before the
// first tofu; during the first init's last tofu, before the second init;
// during the second init, after the last step. Each
// exits 1 with no success report and removes the rehearsal. Not parallel:
// signalGuards is process-wide.
func TestRehearsePrepareSignalCancelsUnderAGuard(t *testing.T) {
	for _, tc := range []struct {
		name string
		// at begins the args of the tofu command whose run the signal
		// arrives in, "" for before any tofu.
		at    string
		calls int
	}{
		{"during the copy and gate 1", "", 0},
		{"during the first init's last tofu", "init -backend=false", 3},
		{"during the second init", "init -input=false -no-color -reconfigure", 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := signalGuards.Load()
			var guards []int32
			var signal context.CancelFunc
			notify := func(ctx context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				ctx, signal = context.WithCancel(ctx)
				if tc.at == "" {
					signal()
				}
				return ctx, signal
			}
			repo, run := initRun(t, rehearsePrepare{notify: notify}, prepareLock, func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
				guards = append(guards, signalGuards.Load())
				if tc.at != "" && strings.HasPrefix(strings.Join(c.Args, " "), tc.at) {
					signal()
				}
				return harness.CommandResult{}, nil
			})

			require.Error(t, run.err)
			assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
			assert.Len(t, run.calls, tc.calls)
			for _, guard := range guards {
				assert.Equal(t, before+1, guard, "a tofu call ran outside the guard")
			}
			assert.Contains(t, run.stdout, "Status: failed\nRefused: prepare is cancelled: context canceled")
			assert.NotContains(t, run.stdout, "Rehearsal:")
			assert.NoDirExists(t, preparedDir(repo))
			assert.Equal(t, before, signalGuards.Load(), "the guard was not released")
		})
	}
}

// write returns the first error writing the human report, success and
// refusal alike.
func TestRehearsePrepareResultWriteReturnsTheWriteError(t *testing.T) {
	t.Parallel()
	for _, result := range []rehearsePrepareResult{
		{Status: CommandStatusSuccess, ID: prepareID, WorkDir: "w", AWSVersion: harness.AWSProviderVersion, Warnings: []string{"w"}},
		{Status: CommandStatusFailed, Refusal: "r"},
	} {
		cmd := &cobra.Command{}
		cmd.SetOut(failingWriter{})
		assert.ErrorIs(t, result.write(cmd, OutputModeHuman), errFailingWriter, result.Status)
	}
}

var errFailingWriter = errors.New("the writer failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errFailingWriter }

// Gate 2 runs over what the first init wrote: a manifest entry fetched
// under <data>/modules, or a provisioner planted in a local module, is
// refused naming the module, and the second init never runs.
func TestRehearsePrepareGate2RefusesBeforeTheSecondInit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// plant runs in the first init's init, in the copy's root, with
		// the data dir; it returns the manifest to write there.
		plant func(t *testing.T, root, data string) string
		want  string
	}{
		{"a manifest entry under <data>/modules", func(t *testing.T, _, data string) string {
			return fmt.Sprintf(`{"Modules":[{"Key":"","Source":"","Dir":"."},{"Key":"fetched","Source":"registry.opentofu.org/x/y/aws","Dir":%q}]}`, filepath.Join(data, "modules", "fetched"))
		}, `modules.json: module "fetched": refused: dir "`},
		{"a provisioner in a local module", func(t *testing.T, root, _ string) string {
			require.NoError(t, os.WriteFile(filepath.Join(root, "..", "..", "modules", "m", "planted.tf"), []byte(provisioner), 0o644))
			return `{"Modules":[{"Key":"","Source":"","Dir":"."},{"Key":"m","Source":"../../modules/m","Dir":"../../modules/m"}]}`
		}, `module "m": modules/m/planted.tf:2: terraform_data.t.provisioner.local-exec: "provisioner" blocks are refused`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, repo := prepareRepo(t, map[string]string{
				"envs/prod/main.tf": prepareProvider + "module \"m\" {\n  source = \"../../modules/m\"\n}\n",
				lockName:            prepareLock,
				"modules/m/main.tf": "resource \"terraform_data\" \"x\" {}\n",
			})

			run := initRunIn(t, rehearsePrepare{}, repo, func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
				if isFirstInit(c) {
					data := c.Env["TF_DATA_DIR"]
					manifest := tc.plant(t, c.Dir, data)
					require.NoError(t, os.MkdirAll(filepath.Join(data, "modules"), 0o755))
					require.NoError(t, os.WriteFile(filepath.Join(data, "modules", "modules.json"), []byte(manifest), 0o644))
				}
				return harness.CommandResult{}, nil
			})

			require.ErrorIs(t, run.err, errRehearsalRefuses)
			assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
			assert.Contains(t, run.err.Error(), "gate 2, after the first init: ")
			assert.Contains(t, run.err.Error(), tc.want)
			require.Len(t, run.calls, 3)
			assert.False(t, slices.ContainsFunc(run.calls, isSecondInit), "the second init ran")
			assert.NoDirExists(t, preparedDir(repo))
		})
	}
}

// The second init runs offline: the backend override in the copy's root at
// 0600, the state outside the copy, only the providers the first init
// installed, and replace mode with both proxies at the dead proxy.
func TestRehearsePrepareSecondInitCommand(t *testing.T) {
	t.Parallel()
	var override string
	var overrideMode fs.FileMode
	repo, run := initRun(t, rehearsePrepare{}, prepareLock, func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
		if isSecondInit(c) {
			raw, err := os.ReadFile(filepath.Join(c.Dir, ourOverride))
			require.NoError(t, err)
			info, err := os.Stat(filepath.Join(c.Dir, ourOverride))
			require.NoError(t, err)
			override, overrideMode = string(raw), info.Mode()
		}
		return harness.CommandResult{}, nil
	})
	require.NoError(t, run.err)

	rehearsal, err := filepath.EvalSymlinks(preparedDir(repo))
	require.NoError(t, err)
	work := filepath.Join(rehearsal, "work")
	state := filepath.Join(rehearsal, "state", "terraform.tfstate")
	require.Len(t, run.calls, 4)
	c := run.calls[3]
	assert.Equal(t, "tofu", c.Name)
	assert.Equal(t, filepath.Join(work, "envs", "prod"), c.Dir)
	assert.Equal(t, []string{"init", "-input=false", "-no-color", "-reconfigure", "-get=false", "-lockfile=readonly",
		"-backend-config=path=" + state, "-plugin-dir=" + filepath.Join(rehearsal, "data", "providers")}, c.Args)
	assert.True(t, c.ReplaceEnv, "the second init runs with an inherited environment")
	assert.Empty(t, c.StripEnv)
	assert.Equal(t, map[string]string{
		"HOME": filepath.Join(rehearsal, "home"), "TF_DATA_DIR": filepath.Join(rehearsal, "data"),
		"HTTPS_PROXY": "http://127.0.0.1:9", "HTTP_PROXY": "http://127.0.0.1:9",
	}, c.Env)
	assert.NotContains(t, c.Env, "NO_PROXY")
	rel, err := filepath.Rel(work, state)
	require.NoError(t, err)
	assert.False(t, filepath.IsLocal(rel), "the state %s lies inside %s", state, work)
	assert.Equal(t, LocalBackendOverride, override)
	assert.Equal(t, fs.FileMode(0o600), overrideMode)
	assert.DirExists(t, filepath.Dir(state))
	assert.Contains(t, run.stdout, "Status: success\n")
}

// A dead proxy that accepts a connection refuses the second init before
// it runs. Not parallel: it moves rehearsalDeadProxy.
func TestRehearsePrepareRefusesALiveDeadProxy(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })
	before := rehearsalDeadProxy
	rehearsalDeadProxy = "http://" + listener.Addr().String()
	t.Cleanup(func() { rehearsalDeadProxy = before })

	repo, run := initRun(t, rehearsePrepare{}, prepareLock, func(context.Context, harness.Command) (harness.CommandResult, error) {
		return harness.CommandResult{}, nil
	})

	require.Error(t, run.err)
	assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
	assert.Contains(t, run.stdout, "Refused: the second init is refused: the dead proxy "+rehearsalDeadProxy+" accepted a connection")
	assert.Len(t, run.calls, 3)
	assert.False(t, slices.ContainsFunc(run.calls, isSecondInit), "the second init ran")
	assert.NoDirExists(t, preparedDir(repo))
}

// The dead proxy check admits only a refused connection.
func TestRehearsalProxyRefuses(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	for _, tc := range []struct {
		name, proxy, want string
	}{
		{"a refused connection", "http://127.0.0.1:9", ""},
		{"a listener", "http://" + listener.Addr().String(), "accepted a connection"},
		{"no port", "http://127.0.0.1", "did not refuse a dial (dial tcp: address 127.0.0.1: missing port in address)"},
		{"no parse", "http://[::1", "does not parse"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := rehearsalProxyRefuses(context.Background(), tc.proxy)
			if tc.want == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "the second init is refused: the dead proxy "+tc.proxy+" "+tc.want)
		})
	}
}
