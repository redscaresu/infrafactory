package cli

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"maps"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// passthroughToken takes exactly the recorded token, as fakeaws does.
func passthroughToken(auth, token string) bool { return auth == "Bearer "+token }

// passthroughStub is a runner that records each Command and answers the
// nth call (from 1) with answer.
type passthroughStub struct {
	calls  []harness.Command
	answer func(ctx context.Context, n int) (harness.CommandResult, error)
}

func (s *passthroughStub) Run(ctx context.Context, c harness.Command) (harness.CommandResult, error) {
	s.calls = append(s.calls, c)
	if s.answer == nil {
		return harness.CommandResult{}, nil
	}
	return s.answer(ctx, len(s.calls))
}

// The run reaches the runner once, in the root's work dir, under exactly
// the walls' environment in replace mode, with the injected flags before
// the user's, the caller's writers and the 2-hour deadline.
func TestRehearsePassthroughRunCommand(t *testing.T) {
	t.Parallel()
	repo, r := regatePreparedLive(t, passthroughToken, "--var-file", "prod.tfvars")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	var deadline time.Time
	stub := &passthroughStub{answer: func(ctx context.Context, _ int) (harness.CommandResult, error) {
		deadline, _ = ctx.Deadline()
		return harness.CommandResult{}, nil
	}}

	err := rehearsePassthrough{runner: stub, wd: repo, stdout: stdout, stderr: stderr}.run(context.Background(), prepareID, "plan", []string{"-out=p"})

	require.NoError(t, err)
	require.Len(t, stub.calls, 1)
	// The run finds the rehearsal from the resolved repo (rehearsalRepo).
	dir, err := filepath.EvalSymlinks(r.dir)
	require.NoError(t, err)
	c := stub.calls[0]
	assert.Equal(t, "tofu", c.Name)
	assert.Equal(t, []string{"plan", "-input=false", "-var-file=" + filepath.Join(dir, rehearsalVarFile), "-out=p"}, c.Args)
	assert.Equal(t, filepath.Join(dir, regateRoot), c.Dir)
	assert.True(t, c.ReplaceEnv)
	assert.Empty(t, c.StripEnv)
	assert.Equal(t, rehearsalWallsEnv(dir, rehearsalFakeawsURL(r.record.Port), r.record.Region), c.Env)
	assert.Same(t, stdout, c.Stdout)
	assert.Same(t, stderr, c.Stderr)
	assert.WithinDuration(t, time.Now().Add(rehearsalPassthroughTimeout), deadline, time.Minute)
}

// A zero rehearsePassthrough hands tofu this process's stdout and
// stderr: a nil writer would drop tofu's output, and its exit code is
// then reported with nothing printed.
func TestRehearsePassthroughRunDefaultsItsWriters(t *testing.T) {
	t.Parallel()
	repo, _ := regatePreparedLive(t, passthroughToken)
	stub := &passthroughStub{}

	require.NoError(t, rehearsePassthrough{runner: stub, wd: repo}.run(context.Background(), prepareID, "plan", nil))

	require.Len(t, stub.calls, 1)
	assert.Same(t, os.Stdout, stub.calls[0].Stdout)
	assert.Same(t, os.Stderr, stub.calls[0].Stderr)
}

// Each of infrafactory's overrides, one byte off what prepare wrote, is
// refused before tofu runs.
func TestRehearsePassthroughRunRefusesANonIdenticalOverride(t *testing.T) {
	t.Parallel()
	for _, name := range []string{ourOverride, rehearsalAWSOverride} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo, r := regatePreparedLive(t, passthroughToken)
			flipFirstByte(t, filepath.Join(r.dir, regateRoot, name))
			stub := &passthroughStub{}

			err := rehearsePassthrough{runner: stub, wd: repo}.run(context.Background(), prepareID, "plan", nil)

			require.ErrorIs(t, err, errRehearsalRefuses)
			assert.Contains(t, err.Error(), "envs/prod/"+name+": refused: it is not byte-identical to the override infrafactory writes")
			assert.Equal(t, ExitCodeRuntime, ExitCodeForError(err))
			assert.Empty(t, stub.calls)
		})
	}
}

// The re-gate runs before every call: an override edited during call 1
// refuses call 2.
func TestRehearsePassthroughRunRegatesBeforeEveryCall(t *testing.T) {
	t.Parallel()
	repo, r := regatePreparedLive(t, passthroughToken)
	stub := &passthroughStub{answer: func(context.Context, int) (harness.CommandResult, error) {
		flipFirstByte(t, filepath.Join(r.dir, regateRoot, rehearsalAWSOverride))
		return harness.CommandResult{}, nil
	}}
	p := rehearsePassthrough{runner: stub, wd: repo}

	require.NoError(t, p.run(context.Background(), prepareID, "plan", nil))
	err := p.run(context.Background(), prepareID, "plan", nil)

	require.ErrorIs(t, err, errRehearsalRefuses)
	assert.Contains(t, err.Error(), rehearsalAWSOverride+": refused: it is not byte-identical")
	assert.Len(t, stub.calls, 1)
}

// Every refusal before tofu is exit 1 with the runner never called.
func TestRehearsePassthroughRunRefusesBeforeTofu(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		accept func(auth, token string) bool
		setup  func(t *testing.T, repo string, r openRehearsal)
		ctx    func() context.Context
		args   []string
		want   string
	}{
		{name: "a refused record", setup: func(t *testing.T, _ string, r openRehearsal) {
			require.NoError(t, os.WriteFile(filepath.Join(r.dir, rehearsalRecordFile), []byte(`{"schema":"other"}`), 0o600))
		}, want: `its schema is "other"`},
		{name: "two roots", setup: func(t *testing.T, repo string, r openRehearsal) {
			record := r.record
			record.Roots = append(record.Roots, rehearsalRoot{Copy: "work", Root: "work/envs/stage"})
			rewriteRecord(t, repo, r, record)
		}, want: `its record names 2 roots ["work/envs/prod" "work/envs/stage"], and passthrough runs in exactly one`},
		{name: "no root", setup: func(t *testing.T, repo string, r openRehearsal) {
			record := r.record
			record.Roots = nil
			rewriteRecord(t, repo, r, record)
		}, want: "its record names 0 roots [], and passthrough runs in exactly one"},
		{name: "a fakeaws that fails the proof", accept: func(string, string) bool { return true },
			want: "GET /mock/requests without its token answered 200, not 401"},
		{name: "a cancelled ctx", ctx: func() context.Context {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx
		}, want: "tofu plan is cancelled before it runs: context canceled"},
		{name: "a refused tofu flag", args: []string{"-var=x=y"}, want: "tofu plan: -var is refused"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			accept := passthroughToken
			if tc.accept != nil {
				accept = tc.accept
			}
			repo, r := regatePreparedLive(t, accept)
			if tc.setup != nil {
				tc.setup(t, repo, r)
			}
			ctx := context.Background()
			if tc.ctx != nil {
				ctx = tc.ctx()
			}
			stub := &passthroughStub{}

			err := rehearsePassthrough{runner: stub, wd: repo}.run(ctx, prepareID, "plan", tc.args)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, ExitCodeRuntime, ExitCodeForError(err))
			assert.Empty(t, stub.calls)
		})
	}
}

// A live dead proxy is refused before tofu runs. Not parallel: it moves
// rehearsalDeadProxy, after prepare has dialled it.
func TestRehearsePassthroughRunRefusesALiveDeadProxy(t *testing.T) {
	repo, _ := regatePreparedLive(t, passthroughToken)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })
	before := rehearsalDeadProxy
	rehearsalDeadProxy = "http://" + listener.Addr().String()
	t.Cleanup(func() { rehearsalDeadProxy = before })
	stub := &passthroughStub{}

	err = rehearsePassthrough{runner: stub, wd: repo}.run(context.Background(), prepareID, "plan", nil)

	require.ErrorIs(t, err, errRehearsalProxyLive)
	assert.Equal(t, ExitCodeRuntime, ExitCodeForError(err))
	assert.Empty(t, stub.calls)
}

// Tofu's own exit code passes through when it ran to completion; -1, an
// interrupt during the run and a failed tighten each give exit 1.
func TestRehearsePassthroughRunMapsTheExit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		answer func(cancel context.CancelFunc, r openRehearsal) (harness.CommandResult, error)
		want   int
	}{
		{"exit 0", func(context.CancelFunc, openRehearsal) (harness.CommandResult, error) {
			return harness.CommandResult{}, nil
		}, ExitCodeSuccess},
		{"exit 2", func(context.CancelFunc, openRehearsal) (harness.CommandResult, error) {
			return harness.CommandResult{ExitCode: 2}, errors.New("exit status 2")
		}, 2},
		{"exit -1", func(context.CancelFunc, openRehearsal) (harness.CommandResult, error) {
			return harness.CommandResult{ExitCode: -1}, errors.New(`exec: "tofu": executable file not found in $PATH`)
		}, ExitCodeRuntime},
		{"a cancel during the run, then exit 0", func(cancel context.CancelFunc, _ openRehearsal) (harness.CommandResult, error) {
			cancel()
			return harness.CommandResult{}, nil
		}, ExitCodeRuntime},
		{"a failed tighten after exit 2", func(_ context.CancelFunc, r openRehearsal) (harness.CommandResult, error) {
			if err := os.RemoveAll(r.dir); err != nil {
				return harness.CommandResult{}, err
			}
			return harness.CommandResult{ExitCode: 2}, errors.New("exit status 2")
		}, ExitCodeRuntime},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo, r := regatePreparedLive(t, passthroughToken)
			var cancel context.CancelFunc
			notify := func(ctx context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
				ctx, cancel = context.WithCancel(ctx)
				return ctx, cancel
			}
			stub := &passthroughStub{answer: func(context.Context, int) (harness.CommandResult, error) { return tc.answer(cancel, r) }}

			err := rehearsePassthrough{runner: stub, wd: repo, notify: notify}.run(context.Background(), prepareID, "plan", nil)

			assert.Equal(t, tc.want, ExitCodeForError(err), "%v", err)
			assert.Len(t, stub.calls, 1)
		})
	}
}

// The rehearsal's modes are tightened after a failed run too.
func TestRehearsePassthroughRunTightensAfterAFailedRun(t *testing.T) {
	t.Parallel()
	repo, r := regatePreparedLive(t, passthroughToken)
	wide := filepath.Join(r.dir, regateRoot, "wide")
	stub := &passthroughStub{answer: func(context.Context, int) (harness.CommandResult, error) {
		if err := os.Mkdir(wide, 0o755); err != nil {
			return harness.CommandResult{}, err
		}
		if err := os.WriteFile(filepath.Join(wide, "plan"), nil, 0o644); err != nil {
			return harness.CommandResult{}, err
		}
		return harness.CommandResult{ExitCode: 1}, errors.New("exit status 1")
	}}

	err := rehearsePassthrough{runner: stub, wd: repo}.run(context.Background(), prepareID, "plan", nil)

	assert.Equal(t, ExitCodeRuntime, ExitCodeForError(err))
	for path, want := range map[string]fs.FileMode{wide: 0o700, filepath.Join(wide, "plan"): 0o600} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, want, info.Mode().Perm(), path)
	}
}

// At the deadline the runner interrupts tofu's group, then kills it, and
// the run is exit 1. The "tree" helper ignores SIGINT and its grandchild
// holds the output pipe, so Run returns before the grandchild's own 10s
// only when the whole group is killed. Not parallel: it shortens
// cancelKillFallback.
func TestRehearsePassthroughRunDeadlineKillsTheGroup(t *testing.T) {
	saved := cancelKillFallback
	cancelKillFallback = 200 * time.Millisecond
	t.Cleanup(func() { cancelKillFallback = saved })
	repo, _ := regatePreparedLive(t, passthroughToken)
	ready := filepath.Join(t.TempDir(), "ready")
	runner := harness.CommandRunnerFunc(func(ctx context.Context, c harness.Command) (harness.CommandResult, error) {
		helper := helperCommand("tree")
		c.Name, c.Args, c.Env = helper.Name, helper.Args, maps.Clone(c.Env)
		maps.Copy(c.Env, helper.Env)
		c.Env[execHelperReadyEnv] = ready
		return execCommandRunner{}.Run(ctx, c)
	})
	stdout := &bytes.Buffer{}

	start := time.Now()
	err := rehearsePassthrough{runner: runner, wd: repo, stdout: stdout, timeout: 3 * time.Second}.run(context.Background(), prepareID, "plan", nil)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, ExitCodeRuntime, ExitCodeForError(err))
	assert.Equal(t, "interrupt\ninterrupt\n", stdout.String(), "child and grandchild each interrupted once")
	assert.Less(t, time.Since(start), 8*time.Second, "the grandchild outlived the kill")
}
