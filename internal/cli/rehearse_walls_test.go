package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

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
			return prepareStubFakeaws, func() error { t.Error("a prepare that succeeded stopped fakeaws"); return nil }, o.started(prepareStubFakeaws)
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
	assert.NotNil(t, opts.started)
	assert.NotNil(t, opts.stopped)
	opts.started, opts.stopped = nil, nil
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
		Roots: []rehearsalRoot{{Copy: "work", Root: filepath.Join("work", "envs", "prod"), EndpointKeys: prepareKeys, AWSAliases: []string{""}}},
	}, record)
	override, err := os.ReadFile(filepath.Join(root, rehearsalAWSOverride))
	require.NoError(t, err)
	assert.Equal(t, string(rehearsalProviderOverride("http://127.0.0.1:4566", "eu-west-1", prepareKeys, []string{""})), string(override))
	assert.Contains(t, run.stdout, "Fakeaws URL: http://127.0.0.1:4566\n")
	assert.NotContains(t, run.stdout, prepareStubFakeaws.Token)
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
					require.NoError(t, o.started(prepareStubFakeaws))
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
// it, but only once the record is written, in the start or after it: a
// rehearsal with no record is removed as any other. The refusal names the
// stop failure and, in both outputs, the kept rehearsal's id and record.
// A start refused with a failed stop is the stub's stopFailed alone: the
// real start's wording, and that it wraps errRehearsalFakeawsStop, are
// TestRehearsalStartFakeawsRefusesATryItCannotRecord's.
func TestRehearsePrepareKeepsTheRehearsalWhenFakeawsWillNotStop(t *testing.T) {
	t.Parallel()
	stopFailed := fmt.Errorf("%w: the stub's stop", errRehearsalFakeawsStop)

	for _, tc := range []struct {
		name string
		// start is the stub starter's answer, given its opts, the rehearsal
		// and the cancel; stop fails with stopFailed.
		start func(t *testing.T, o rehearsalFakeawsOpts, rehearsal string, cancel context.CancelFunc) (rehearsalFakeaws, error)
		want  string
		kept  bool
	}{
		{"a cancel after the record", func(t *testing.T, o rehearsalFakeawsOpts, _ string, cancel context.CancelFunc) (rehearsalFakeaws, error) {
			require.NoError(t, o.started(prepareStubFakeaws))
			cancel()
			return prepareStubFakeaws, nil
		}, "prepare is cancelled: context canceled; and fakeaws's stop failed: the stub's stop; " + errRehearsalKept.Error(), true},
		{"a start refused after the record", func(t *testing.T, o rehearsalFakeawsOpts, _ string, _ context.CancelFunc) (rehearsalFakeaws, error) {
			require.NoError(t, o.started(prepareStubFakeaws))
			return rehearsalFakeaws{}, stopFailed
		}, "stop failed: the stub's stop; " + errRehearsalKept.Error(), true},
		{"the record write fails", func(t *testing.T, o rehearsalFakeawsOpts, rehearsal string, _ context.CancelFunc) (rehearsalFakeaws, error) {
			require.NoError(t, os.Chmod(rehearsal, 0o500))
			t.Cleanup(func() { _ = os.Chmod(rehearsal, 0o700) })
			require.Error(t, o.started(prepareStubFakeaws))
			// So a removal can remove it.
			require.NoError(t, os.Chmod(rehearsal, 0o700))
			return rehearsalFakeaws{}, stopFailed
		}, "stop failed: the stub's stop", false},
		{"a start refused after its stopped try's record is cleared", func(t *testing.T, o rehearsalFakeawsOpts, rehearsal string, _ context.CancelFunc) (rehearsalFakeaws, error) {
			require.NoError(t, o.started(rehearsalFakeaws{PID: 4141, Port: 4567, Token: "first-try"}))
			require.NoError(t, o.stopped())
			assert.NoFileExists(t, filepath.Join(rehearsal, rehearsalRecordFile), "the record still names a stopped try")
			return rehearsalFakeaws{}, stopFailed
		}, "stop failed: the stub's stop", false},
		{"a start that never records", func(*testing.T, rehearsalFakeawsOpts, string, context.CancelFunc) (rehearsalFakeaws, error) {
			return prepareStubFakeaws, nil
		}, "fakeaws is refused: it started, but its record was never written; and fakeaws's stop failed: the stub's stop; " +
			"fakeaws pid 4242 port 4566 is left running and no record names it: stop it by hand with kill -KILL -- -4242", false},
		{"a retry's record write fails", func(t *testing.T, o rehearsalFakeawsOpts, rehearsal string, _ context.CancelFunc) (rehearsalFakeaws, error) {
			require.NoError(t, o.started(rehearsalFakeaws{PID: 4141, Port: 4567, Token: "first-try"}))
			require.NoError(t, os.Chmod(rehearsal, 0o500))
			t.Cleanup(func() { _ = os.Chmod(rehearsal, 0o700) })
			require.Error(t, o.started(prepareStubFakeaws))
			require.NoError(t, os.Chmod(rehearsal, 0o700))
			return rehearsalFakeaws{}, stopFailed
		}, "stop failed: the stub's stop", false},
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
						f, err := tc.start(t, o, filepath.Dir(o.dir), cancel)
						return f, func() error { return stopFailed }, err
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
// passed --fakeaws-bin: unset installs the pin through the runner, and a
// set path is checked instead. Not parallel: it moves the user cache.
func TestRehearsePrepareResolvesThroughRehearsalFakeawsBinary(t *testing.T) {
	rehearsalEmptyUserCache(t)

	for _, tc := range []struct {
		name, want string
		args       []string
		// after is each runner call after the schema call, by name and args.
		after []string
	}{
		{"the pinned install", "installed no fakeaws binary", nil, []string{"go install " + rehearsalFakeawsCmd + "@" + rehearsalFakeawsPin}},
		{"a set path", "the fakeaws binary /nonexistent/fakeaws is refused", []string{"--fakeaws-bin", "/nonexistent/fakeaws"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			started := 0
			p := rehearsePrepare{
				startFakeaws: func(context.Context, rehearsalFakeawsOpts) (rehearsalFakeaws, func() error, error) {
					started++
					return prepareStubFakeaws, func() error { return nil }, nil
				},
			}
			repo := lockRepo(t, prepareLock, "")
			run := runPrepareWith(t, p, repo, func(context.Context, harness.Command) (harness.CommandResult, error) {
				return harness.CommandResult{}, nil
			}, tc.args...)

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
			{Copy: "work", Root: "work/envs/prod", EndpointKeys: prepareKeys, AWSAliases: []string{"", "west"}},
			{Copy: "head", Root: "head/envs/prod", EndpointKeys: []string{"sts"}, AWSAliases: []string{""}},
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

// A prepare SIGKILLed once fakeaws is started, while its ready check
// waits, leaves a record naming that fakeaws: `rehearse end` stops it,
// its port refuses a dial and the rehearsal is gone.
func TestRehearseEndStopsTheFakeawsOfAPrepareKilledBeforeReady(t *testing.T) {
	bin, stubDir := fakeawsStub(t, "late-healthy")
	t.Cleanup(func() {
		for _, pid := range fakeawsStubPIDs(stubDir) {
			if groupAlive(pid) {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
		}
	})
	_, repo := prepareRepo(t, nil)
	dir := preparedDir(repo)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	prepare := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRehearsalFakeawsStub$", "--", bin, dir)
	prepare.Env = append(os.Environ(), fakeawsStubEnv+"=walls")
	require.NoError(t, prepare.Start())
	waited := make(chan error, 1)
	go func() { waited <- prepare.Wait() }()

	var record rehearsalRecord
	require.Eventually(t, func() bool {
		var err error
		record, err = readRehearsalRecord(dir)
		return err == nil
	}, 20*time.Second, 20*time.Millisecond, "prepare wrote no record while fakeaws was not ready")
	// The record can come before the stub script logs its pid.
	require.Eventually(t, func() bool { return slices.Equal([]int{record.PID}, fakeawsStubPIDs(stubDir)) },
		10*time.Second, 20*time.Millisecond, "the record names another fakeaws")
	// Up but not healthy, so prepare is inside its ready check.
	client, healthz := rehearsalFakeawsClient(), rehearsalFakeawsURL(record.Port)+"/healthz"
	require.Eventually(t, func() bool {
		code, _ := rehearsalFakeawsStatus(ctx, client, healthz, "")
		return code == http.StatusServiceUnavailable
	}, 20*time.Second, 20*time.Millisecond, "fakeaws never answered /healthz")
	require.NoError(t, prepare.Process.Kill())
	<-waited
	require.True(t, groupAlive(record.PID), "fakeaws died with prepare")

	// Healthy now, so it can prove itself to end.
	mustWriteFile(t, filepath.Join(stubDir, "healthy"), "")
	run := runEnd(t, rehearseEnd{wd: repo}, prepareID)
	require.NoError(t, run.err, run.stdout)
	assert.Equal(t, "stopped", run.result.Fakeaws)
	assert.False(t, groupAlive(record.PID), "the stub's group is still alive")
	assertPortRefuses(t, record.Port)
	assert.NoDirExists(t, dir)
}
