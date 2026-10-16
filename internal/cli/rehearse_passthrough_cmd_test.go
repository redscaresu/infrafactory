package cli

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// passthroughAllowed is the list every refused action's message names.
const passthroughAllowed = "the rehearsal passes only apply, destroy, import, plan, state mv"

// passthroughRoot is the root command with the passthrough family built
// from p.
func passthroughRoot(p rehearsePassthrough) *cobra.Command {
	root := NewRootCmd()
	group, _, _ := root.Find([]string{"rehearse"})
	cmds := newRehearsePassthroughCmds(p)
	for _, c := range cmds {
		old, _, _ := group.Find([]string{c.Name()})
		group.RemoveCommand(old)
	}
	group.AddCommand(cmds...)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	return root
}

// Every tofu action outside the list, under state too, is a usage error
// that names it, and tofu never runs.
func TestRehearsePassthroughRefusesUnlistedActions(t *testing.T) {
	t.Parallel()
	for _, action := range []string{
		"init", "validate", "refresh", "output", "show", "taint", "untaint", "force-unlock", "workspace",
		"providers", "get", "graph", "fmt", "test", "login", "logout", "version", "console",
		"state list", "state rm", "state pull", "state push", "state show", "state replace-provider",
	} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			stub := &passthroughStub{}
			root := passthroughRoot(rehearsePassthrough{runner: stub, wd: t.TempDir()})
			root.SetArgs(append(append([]string{"rehearse"}, strings.Fields(action)...), prepareID))

			err := root.Execute()

			require.Error(t, err)
			assert.Equal(t, ExitCodeUsage, ExitCodeForError(err))
			assert.Contains(t, err.Error(), fmt.Sprintf("unknown command %q: %s", action, passthroughAllowed))
			assert.Empty(t, stub.calls)
		})
	}
}

// The family's refusals through the root: a refused tofu flag is exit 1,
// an unlisted action and --output json are usage errors, and `state` alone
// prints help. Tofu never runs.
func TestRehearsePassthroughCLIRefusals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		args       []string
		code       int
		want       string
		wantStdout string
	}{
		{args: []string{"plan", prepareID, "--", "-var=x=y"}, code: ExitCodeRuntime, want: "tofu plan: -var is refused"},
		{args: []string{"plan", prepareID, "--output", "json", "--", "-out=p"}, code: ExitCodeUsage, want: "--output json is refused: tofu plan writes its own output"},
		{args: []string{"refresh", prepareID}, code: ExitCodeUsage, want: `unknown command "refresh": ` + passthroughAllowed},
		{args: []string{"state", "rm", prepareID}, code: ExitCodeUsage, want: `unknown command "state rm": ` + passthroughAllowed},
		{args: []string{"state"}, code: ExitCodeSuccess, wantStdout: "infrafactory rehearse state [command]"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			t.Parallel()
			repo, _ := regatePreparedLive(t, passthroughToken)
			stub := &passthroughStub{}
			root := passthroughRoot(rehearsePassthrough{runner: stub, wd: repo})
			stdout := &bytes.Buffer{}
			root.SetOut(stdout)
			root.SetArgs(append([]string{"rehearse"}, tc.args...))

			err := root.Execute()

			assert.Equal(t, tc.code, ExitCodeForError(err), "%v", err)
			if tc.want != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.want)
			}
			assert.Contains(t, stdout.String(), tc.wantStdout)
			assert.Empty(t, stub.calls)
		})
	}
}

// Exactly one <id> goes before --, and every arg after it reaches tofu
// as written, after the injected flags.
func TestRehearsePassthroughCLIArgsAtDash(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		args []string
		argv []string // tofu's argv, or nil for a usage error
	}{
		{[]string{"plan", prepareID, "--", "-out=p", "-lock=false"}, []string{"plan", "-input=false", "-out=p", "-lock=false"}},
		{[]string{"state", "mv", prepareID, "--", "aws_vpc.a", "aws_vpc.b"}, []string{"state", "mv", "aws_vpc.a", "aws_vpc.b"}},
		{[]string{"plan", prepareID}, []string{"plan", "-input=false"}},
		{[]string{"plan"}, nil},
		{[]string{"plan", "--", prepareID}, nil},
		{[]string{"plan", prepareID, "-out=p"}, nil},
		{[]string{"plan", prepareID, "extra", "--", "-out=p"}, nil},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			t.Parallel()
			repo, r := regatePreparedLive(t, passthroughToken)
			stub := &passthroughStub{}
			root := passthroughRoot(rehearsePassthrough{runner: stub, wd: repo})
			root.SetArgs(append([]string{"rehearse"}, tc.args...))

			err := root.Execute()

			if tc.argv == nil {
				assert.Equal(t, ExitCodeUsage, ExitCodeForError(err), "%v", err)
				assert.Empty(t, stub.calls)
				return
			}
			require.NoError(t, err)
			require.Len(t, stub.calls, 1)
			assert.Equal(t, tc.argv, stub.calls[0].Args)
			dir, err := filepath.EvalSymlinks(r.dir)
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(dir, regateRoot), stub.calls[0].Dir)
		})
	}
}

// passthroughHelperRepoEnv names the repo TestRehearsePassthroughHelperProcess
// runs `rehearse plan` from.
const passthroughHelperRepoEnv = "INFRAFACTORY_PASSTHROUGH_HELPER_REPO"

// TestRehearsePassthroughHelperProcess is not a test: it is infrafactory
// running the registered `rehearse plan <id>`, with tofu replaced by
// TestExecRunnerHelperProcess's "count", which prints how many SIGINTs
// it saw and exits 0. It prints " exit=<code>" after the child's output.
func TestRehearsePassthroughHelperProcess(t *testing.T) {
	repo := os.Getenv(passthroughHelperRepoEnv)
	if repo == "" {
		return
	}
	runner := harness.CommandRunnerFunc(func(ctx context.Context, c harness.Command) (harness.CommandResult, error) {
		helper := helperCommand("count")
		c.Name, c.Args, c.Env = helper.Name, helper.Args, maps.Clone(c.Env)
		maps.Copy(c.Env, helper.Env)
		c.Env[execHelperReadyEnv] = os.Getenv(execHelperReadyEnv)
		return execCommandRunner{}.Run(ctx, c)
	})
	root := passthroughRoot(rehearsePassthrough{runner: runner, wd: repo})
	root.SetOut(os.Stdout)
	root.SetArgs([]string{"rehearse", "plan", prepareID})
	err := root.Execute()
	fmt.Printf(" exit=%d", ExitCodeForError(err))
	os.Exit(0)
}

// One Ctrl-C to the terminal's foreground group reaches tofu as exactly
// one SIGINT, and the command reports exit 1 although tofu exits 0.
func TestRehearsePassthroughCLIOneInterrupt(t *testing.T) {
	t.Parallel()
	repo, _ := regatePreparedLive(t, passthroughToken)
	ready := filepath.Join(t.TempDir(), "ready")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	parent := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRehearsePassthroughHelperProcess$")
	parent.Env = append(os.Environ(), passthroughHelperRepoEnv+"="+repo, execHelperReadyEnv+"="+ready)
	// Its own group stands in for the terminal's, so the signal misses
	// `go test`; the deadline kills that whole group. Tofu's stand-in has
	// a group of its own and holds the stdout pipe, so WaitDelay bounds
	// Wait past the kill.
	parent.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	parent.Cancel = func() error { return syscall.Kill(-parent.Process.Pid, syscall.SIGKILL) }
	parent.WaitDelay = 5 * time.Second
	var stdout bytes.Buffer
	parent.Stdout = &stdout
	require.NoError(t, parent.Start())
	waitForFile(t, ready)

	require.NoError(t, syscall.Kill(-parent.Process.Pid, syscall.SIGINT))
	require.NoError(t, parent.Wait(), stdout.String())

	assert.Equal(t, "interrupts=1 exit=1", stdout.String())
}
