package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// prepareKeys are prepareSchema's endpoints keys, sorted.
var prepareKeys = []string{"accessanalyzer", "ec2", "elbv2", "route53", "s3", "sts"}

// After the second init, prepare runs the schema call with the second
// init's env, then resolves the binary and starts fakeaws, and leaves the
// record and both overrides at 0600.
func TestRehearsePrepareStandsTheWallsUp(t *testing.T) {
	t.Parallel()
	var events []string
	var flagPath string
	var opts rehearsalFakeawsOpts
	p := rehearsePrepare{
		fakeawsBinPath: "/flag/fakeaws",
		fakeawsBin: func(_ context.Context, _ harness.CommandRunner, path string) (string, error) {
			events, flagPath = append(events, "resolver"), path
			return prepareStubBin, nil
		},
		startFakeaws: func(_ context.Context, o rehearsalFakeawsOpts) (rehearsalFakeaws, func() error, error) {
			events, opts = append(events, "starter"), o
			return prepareStubFakeaws, func() error { t.Error("a prepare that succeeded stopped fakeaws"); return nil }, nil
		},
	}
	repo, run := initRun(t, p, prepareLock, func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
		events = append(events, strings.Join(c.Args, " "))
		return harness.CommandResult{}, nil
	})
	require.NoError(t, run.err)

	rehearsal, err := filepath.EvalSymlinks(preparedDir(repo))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(run.calls), 2)
	second, schema := run.calls[len(run.calls)-2], run.calls[len(run.calls)-1]
	assert.True(t, isSecondInit(second), "the call before the schema call is %v", second.Args)
	assert.Equal(t, []string{"providers", "schema", "-json"}, schema.Args)
	assert.True(t, schema.ReplaceEnv, "the schema call runs with an inherited environment")
	assert.Equal(t, second.Env, schema.Env)
	assert.Equal(t, second.Dir, schema.Dir)
	assert.Equal(t, []string{"providers schema -json", "resolver", "starter"}, events[len(events)-3:])
	assert.Equal(t, "/flag/fakeaws", flagPath)
	assert.Equal(t, rehearsalFakeawsOpts{bin: prepareStubBin, dir: filepath.Join(rehearsal, "fakeaws"), account: "123456789012", region: "eu-west-1"}, opts)

	root := filepath.Join(rehearsal, "work", "envs", "prod")
	for _, path := range []string{filepath.Join(rehearsal, rehearsalRecordFile), filepath.Join(root, ourOverride), filepath.Join(root, rehearsalAWSOverride)} {
		info, err := os.Stat(path)
		if assert.NoError(t, err) {
			assert.Equal(t, fs.FileMode(0o600), info.Mode(), path)
		}
	}
	record, err := readRehearsalRecord(rehearsal)
	require.NoError(t, err)
	assert.Equal(t, rehearsalRecord{
		Schema: rehearsalRecordSchema, Account: "123456789012", Region: "eu-west-1", Pin: rehearsalFakeawsPin,
		PID: prepareStubFakeaws.PID, Port: prepareStubFakeaws.Port, Token: prepareStubFakeaws.Token,
		Roots: []rehearsalRoot{{WorkDir: "work", Root: filepath.Join("work", "envs", "prod"), EndpointKeys: prepareKeys, AWSAliases: []string{""}}},
	}, record)
	override, err := os.ReadFile(filepath.Join(root, rehearsalAWSOverride))
	require.NoError(t, err)
	assert.Equal(t, string(rehearsalProviderOverride("http://127.0.0.1:4566", "eu-west-1", prepareKeys, []string{""})), string(override))
}

// A failure or a cancel after the start stops fakeaws once, before the
// rehearsal is removed: no success, and no half-made rehearsal. When the
// override write fails, stop finds the record already holding pid, port
// and token, and no provider override.
func TestRehearsePrepareStopsFakeawsAfterTheStart(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// started runs in the stub starter, with the rehearsal and the cancel.
		started func(t *testing.T, rehearsal string, cancel context.CancelFunc)
		// stopped runs in the stub's stop, with the rehearsal.
		stopped func(t *testing.T, rehearsal string)
		want    string
	}{
		{"the override write fails", func(t *testing.T, rehearsal string, _ context.CancelFunc) {
			root := filepath.Join(rehearsal, "work", "envs", "prod")
			require.NoError(t, os.Chmod(root, 0o500))
			t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
		}, func(t *testing.T, rehearsal string) {
			root := filepath.Join(rehearsal, "work", "envs", "prod")
			info, err := os.Stat(filepath.Join(rehearsal, rehearsalRecordFile))
			if assert.NoError(t, err, "no record when stop ran") {
				assert.Equal(t, fs.FileMode(0o600), info.Mode())
			}
			record, err := readRehearsalRecord(rehearsal)
			if assert.NoError(t, err) {
				assert.Equal(t, prepareStubFakeaws.PID, record.PID)
				assert.Equal(t, prepareStubFakeaws.Port, record.Port)
				assert.Equal(t, prepareStubFakeaws.Token, record.Token)
			}
			assert.NoFileExists(t, filepath.Join(root, rehearsalAWSOverride))
			// So the rehearsal can be removed.
			require.NoError(t, os.Chmod(root, 0o700))
		}, "permission denied"},
		{"a cancel", func(_ *testing.T, _ string, cancel context.CancelFunc) { cancel() }, func(*testing.T, string) {}, "prepare is cancelled: context canceled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var cancel context.CancelFunc
			stops := 0
			p := rehearsePrepare{
				notify: func(ctx context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
					ctx, cancel = context.WithCancel(ctx)
					return ctx, cancel
				},
				startFakeaws: func(_ context.Context, o rehearsalFakeawsOpts) (rehearsalFakeaws, func() error, error) {
					rehearsal := filepath.Dir(o.dir)
					tc.started(t, rehearsal, cancel)
					return prepareStubFakeaws, func() error {
						stops++
						tc.stopped(t, rehearsal)
						return nil
					}, nil
				},
			}
			repo, run := initRun(t, p, prepareLock, func(context.Context, harness.Command) (harness.CommandResult, error) {
				return harness.CommandResult{}, nil
			})

			require.Error(t, run.err)
			assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
			assert.Equal(t, 1, stops, "fakeaws was not stopped exactly once")
			assert.Contains(t, run.stdout, "Status: failed\nRefused: ")
			assert.Contains(t, run.stdout, tc.want)
			assert.NotContains(t, run.stdout, "Rehearsal:")
			assert.NoDirExists(t, preparedDir(repo))
		})
	}
}

// A fakeaws that cannot be stopped keeps the rehearsal, so `end` can stop
// it, but only once the record is written: a rehearsal with no record is
// removed as any other. The refusal names the stop failure and, in both
// outputs, the kept rehearsal's id and record.
func TestRehearsePrepareKeepsTheRehearsalWhenFakeawsWillNotStop(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// started runs in the stub starter, with the rehearsal and the cancel.
		started func(t *testing.T, rehearsal string, cancel context.CancelFunc)
		want    string
		kept    bool
	}{
		{"a cancel after the record", func(_ *testing.T, _ string, cancel context.CancelFunc) { cancel() },
			"prepare is cancelled: context canceled; and fakeaws's stop failed: pid 4242 port 4566; " + errRehearsalKept.Error(), true},
		{"the record write fails", func(t *testing.T, rehearsal string, _ context.CancelFunc) {
			require.NoError(t, os.Chmod(rehearsal, 0o500))
			t.Cleanup(func() { _ = os.Chmod(rehearsal, 0o700) })
		}, "permission denied; and fakeaws's stop failed: pid 4242 port 4566", false},
	} {
		for _, mode := range []OutputMode{OutputModeHuman, OutputModeJSON} {
			t.Run(tc.name+" "+string(mode), func(t *testing.T) {
				t.Parallel()
				var cancel context.CancelFunc
				p := rehearsePrepare{
					notify: func(ctx context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
						ctx, cancel = context.WithCancel(ctx)
						return ctx, cancel
					},
					startFakeaws: func(_ context.Context, o rehearsalFakeawsOpts) (rehearsalFakeaws, func() error, error) {
						rehearsal := filepath.Dir(o.dir)
						tc.started(t, rehearsal, cancel)
						return prepareStubFakeaws, func() error {
							// So a removal can remove it.
							require.NoError(t, os.Chmod(rehearsal, 0o700))
							return errors.New("stop failed: pid 4242 port 4566")
						}, nil
					},
				}
				repo := lockRepo(t, prepareLock, "")
				run := runPrepareWith(t, stubWalls(p), repo, func(context.Context, harness.Command) (harness.CommandResult, error) {
					return harness.CommandResult{}, nil
				}, "--output", string(mode))

				require.Error(t, run.err)
				assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
				refusal := strings.TrimPrefix(run.stdout, "Command: rehearse prepare\nStatus: failed\nRefused: ")
				if mode == OutputModeJSON {
					var result rehearsePrepareResult
					require.NoError(t, json.Unmarshal([]byte(run.stdout), &result), run.stdout)
					assert.Equal(t, CommandStatusFailed, result.Status)
					assert.Empty(t, result.ID)
					refusal = result.Refusal
				}
				assert.NotContains(t, run.stdout, "Rehearsal:")
				if !tc.kept {
					assert.Contains(t, refusal, tc.want)
					assert.NotContains(t, refusal, errRehearsalKept.Error())
					assert.NoDirExists(t, preparedDir(repo))
					return
				}
				rehearsal, err := filepath.EvalSymlinks(preparedDir(repo))
				require.NoError(t, err, "the rehearsal is gone")
				assert.Contains(t, refusal, tc.want+": rehearsal "+prepareID+", record "+filepath.Join(rehearsal, rehearsalRecordFile))
				record, err := readRehearsalRecord(rehearsal)
				require.NoError(t, err, "the record is gone")
				assert.Equal(t, prepareStubFakeaws.PID, record.PID)
				assert.Equal(t, prepareStubFakeaws.Port, record.Port)
				assert.Equal(t, prepareStubFakeaws.Token, record.Token)
			})
		}
	}
}

// A schema with no sts key refuses the prepare before the binary is
// resolved or fakeaws started.
func TestRehearsePrepareRefusesASchemaWithoutSTS(t *testing.T) {
	t.Parallel()
	resolved, started := 0, 0
	p := rehearsePrepare{
		fakeawsBin: func(context.Context, harness.CommandRunner, string) (string, error) {
			resolved++
			return prepareStubBin, nil
		},
		startFakeaws: func(context.Context, rehearsalFakeawsOpts) (rehearsalFakeaws, func() error, error) {
			started++
			return prepareStubFakeaws, func() error { return nil }, nil
		},
	}
	repo, run := initRun(t, p, prepareLock, func(_ context.Context, c harness.Command) (harness.CommandResult, error) {
		if isSchemaCall(c) {
			return harness.CommandResult{Stdout: []byte(strings.Replace(prepareSchema, `"sts"`, `"not_sts"`, 1))}, nil
		}
		return harness.CommandResult{}, nil
	})

	require.Error(t, run.err)
	assert.Equal(t, ExitCodeRuntime, ExitCodeForError(run.err))
	assert.Contains(t, run.stdout, "endpoints block names no sts key")
	assert.Zero(t, resolved, "the binary was resolved")
	assert.Zero(t, started, "fakeaws was started")
	assert.NoDirExists(t, preparedDir(repo))
}

// With no resolver set, prepare resolves through rehearsalFakeawsBinary,
// passed fakeawsBinPath: "" installs the pin through the runner, and a set
// path is checked instead. Not parallel: it moves the user cache.
func TestRehearsePrepareResolvesThroughRehearsalFakeawsBinary(t *testing.T) {
	rehearsalEmptyUserCache(t)

	for _, tc := range []struct {
		name, binPath, want string
		// after is each runner call after the schema call, by name and args.
		after []string
	}{
		{"the pinned install", "", "installed no fakeaws binary", []string{"go install " + rehearsalFakeawsCmd + "@" + rehearsalFakeawsPin}},
		{"a set path", "relative/fakeaws", "the fakeaws binary relative/fakeaws is refused: it is not an absolute path", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			started := 0
			p := rehearsePrepare{
				fakeawsBinPath: tc.binPath,
				startFakeaws: func(context.Context, rehearsalFakeawsOpts) (rehearsalFakeaws, func() error, error) {
					started++
					return prepareStubFakeaws, func() error { return nil }, nil
				},
			}
			repo := lockRepo(t, prepareLock, "")
			run := runPrepareWith(t, p, repo, func(context.Context, harness.Command) (harness.CommandResult, error) {
				return harness.CommandResult{}, nil
			})

			require.Error(t, run.err)
			assert.Contains(t, run.stdout, tc.want)
			assert.Zero(t, started, "fakeaws was started")
			schemaAt := slices.IndexFunc(run.calls, isSchemaCall)
			require.NotEqual(t, -1, schemaAt, "no schema call")
			var after []string
			for _, c := range run.calls[schemaAt+1:] {
				after = append(after, c.Name+" "+strings.Join(c.Args, " "))
			}
			assert.Equal(t, tc.after, after)
			assert.NoDirExists(t, preparedDir(repo))
		})
	}
}

// The reader reads back what the writer wrote, every root included.
func TestRehearsalRecordRoundTrips(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	want := rehearsalRecord{
		Schema: rehearsalRecordSchema, Account: "123456789012", Region: "eu-west-1", Pin: rehearsalFakeawsPin,
		PID: 4242, Port: 4566, Token: "token",
		Roots: []rehearsalRoot{
			{WorkDir: "work", Root: "work/envs/prod", EndpointKeys: prepareKeys, AWSAliases: []string{"", "west"}},
			{WorkDir: "head", Root: "head/envs/prod", EndpointKeys: []string{"sts"}, AWSAliases: []string{""}},
		},
	}

	require.NoError(t, want.write(dir))

	info, err := os.Stat(filepath.Join(dir, rehearsalRecordFile))
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), info.Mode())
	got, err := readRehearsalRecord(dir)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestReadRehearsalRecordRefuses(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct{ content, want string }{
		"an unparseable file": {`{"schema": "infrafactory.rehearsal.v1", "pid": `, "does not parse"},
		"another schema":      {`{"schema": "infrafactory.rehearsal.v0", "pid": 1}`, `its schema is "infrafactory.rehearsal.v0"`},
	} {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, rehearsalRecordFile), []byte(tc.content), 0o600))

		_, err := readRehearsalRecord(dir)

		if assert.Error(t, err, name) {
			assert.Contains(t, err.Error(), tc.want, name)
		}
	}
}
