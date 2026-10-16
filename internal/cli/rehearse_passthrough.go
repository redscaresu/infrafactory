package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// rehearsalPassthroughTimeout bounds one passthrough tofu run (AGENTS.md:
// every subprocess has a deadline). Ctrl-C is the normal stop; at the
// deadline the runner interrupts tofu's process group, then kills it.
const rehearsalPassthroughTimeout = 2 * time.Hour

// rehearsePassthrough is what one passthrough tofu action runs with. A
// zero field takes its default.
type rehearsePassthrough struct {
	// runner runs tofu: execCommandRunner{} by default.
	runner harness.CommandRunner
	// wd is the current directory: os.Getwd by default.
	wd string
	// notify catches a signal as a cancel: signal.NotifyContext by default.
	notify func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)
	// stdout and stderr get tofu's output as it is written: os.Stdout and
	// os.Stderr by default. A nil writer would buffer tofu's output and
	// drop it, and ReportError prints nothing for tofu's own exit code.
	stdout, stderr io.Writer
	// timeout bounds the tofu run: rehearsalPassthroughTimeout by default.
	timeout time.Duration
}

// run runs tofu action with the user's args in rehearsal id's only root,
// under a signal guard for the whole call. Every rehearsalOpen error is a
// refusal, a live dead proxy and a refused record included; so is a
// record without exactly one root, a fakeaws that fails rehearsalEndProof,
// a refused argv, a cancel and a refused re-gate, each before tofu runs.
// Tofu runs in the root under the walls' environment, and the rehearsal's
// modes are tightened after it, however it ended. The error is
// rehearsalPassthroughExit's, joined with a failed tighten's, which makes
// it exit 1 whatever tofu returned.
func (p rehearsePassthrough) run(ctx context.Context, id, action string, args []string) error {
	p = p.withDefaults()
	ctx, stop := notifyGuarded(ctx, p.notify)
	defer stop()
	if p.wd == "" {
		var err error
		if p.wd, err = os.Getwd(); err != nil {
			return err
		}
	}
	repo, err := rehearsalRepo(p.wd)
	if err != nil {
		return err
	}
	r, err := rehearsalOpen(repo, id)
	if err != nil {
		return fmt.Errorf("tofu %s does not run: %w", action, err)
	}
	if n := len(r.record.Roots); n != 1 {
		names := make([]string, n)
		for i, root := range r.record.Roots {
			names[i] = root.Root
		}
		return fmt.Errorf("tofu %s does not run: rehearsal %s: %w: its record names %d roots %q, and passthrough runs in exactly one", action, id, errRehearsalRecord, n, names)
	}
	root := r.record.Roots[0]
	dirs := rehearsalPassthroughDirs{root: filepath.Join(r.dir, root.Root), boundary: filepath.Join(r.dir, root.Copy), rehearsal: r.dir}
	argv, err := rehearsalPassthroughArgs(action, args, dirs)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("tofu %s is cancelled before it runs: %w", action, err)
	}
	if why := rehearsalEndProof(ctx, r.record); why != "" {
		return fmt.Errorf("tofu %s does not run: rehearsal %s's fakeaws is refused: %s", action, id, why)
	}
	if err := rehearsalRegate(r, root.Root); err != nil {
		return fmt.Errorf("tofu %s does not run: %w", action, err)
	}
	runCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	result, runErr := p.runner.Run(runCtx, harness.Command{
		Name: "tofu", Args: argv, Dir: dirs.root,
		Env:        rehearsalWallsEnv(r.dir, rehearsalFakeawsURL(r.record.Port), r.record.Region),
		ReplaceEnv: true, Stdout: p.stdout, Stderr: p.stderr,
	})
	err = rehearsalPassthroughExit(action, runCtx.Err(), result, runErr)
	if tightenErr := rehearsalTightenModes(r.dir); tightenErr != nil {
		err = errors.Join(err, fmt.Errorf("the rehearsal's modes are not tightened after tofu %s: %w", action, tightenErr))
	}
	return err
}

func (p rehearsePassthrough) withDefaults() rehearsePassthrough {
	if p.runner == nil {
		p.runner = execCommandRunner{}
	}
	if p.notify == nil {
		p.notify = signal.NotifyContext
	}
	if p.stdout == nil {
		p.stdout = os.Stdout
	}
	if p.stderr == nil {
		p.stderr = os.Stderr
	}
	if p.timeout == 0 {
		p.timeout = rehearsalPassthroughTimeout
	}
	return p
}

// rehearsalPassthroughExit is the error a passthrough run ends with
// (ADR-0042): nil when tofu exited 0, and tofu's own code through
// TofuExitError when it ran to completion. An interrupt or the deadline
// (stopped) and an exit code of -1 each give exit 1, whatever tofu
// returned: neither is the carrier.
func rehearsalPassthroughExit(action string, stopped error, result harness.CommandResult, runErr error) error {
	var err error
	switch {
	case stopped != nil:
		err = fmt.Errorf("tofu %s is stopped, exit code %d: %w", action, result.ExitCode, stopped)
	case result.ExitCode > 0:
		err = &TofuExitError{ExitCode: result.ExitCode}
	case result.ExitCode < 0 || runErr != nil:
		err = errors.Join(fmt.Errorf("tofu %s did not run to completion: exit code %d", action, result.ExitCode), runErr)
	}
	return err
}
