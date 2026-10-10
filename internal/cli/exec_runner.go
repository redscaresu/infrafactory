package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/redscaresu/infrafactory/internal/harness"
)

type execCommandRunner struct{}

// gcpAuthEnvPrefixes lists env vars that trigger terraform-provider-google's
// Application Default Credentials probing. When the parent process (the
// developer's shell, CI runner, or a previous `gcloud auth login`) has
// any of these set, the v5 SDK skips the access_token short-circuit and
// instead probes the metadata server / token-exchange endpoint, which 401s
// against fakegcp's bearer-token middleware and surfaces as
// `ACCESS_TOKEN_TYPE_UNSUPPORTED`. Stripping them at the harness boundary
// guarantees the LLM's providers.tf credentials field is what's used.
//
// Surfaced 2026-06-02 by the GCP investigation agent — even with all 18
// _custom_endpoint flags set correctly, gcp-cloud-run's
// google_project_service preflight escaped to real cloudresourcemanager.
// Same pattern as the CLAUDECODE strip in claude_adapter.go.
var gcpAuthEnvPrefixes = []string{
	"GOOGLE_APPLICATION_CREDENTIALS",
	"GOOGLE_CREDENTIALS",
	"GOOGLE_CLOUD_KEYFILE_JSON",
	"GOOGLE_OAUTH_ACCESS_TOKEN",
	"CLOUDSDK_",
	"GCLOUD_",
}

func stripGCPAuthEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		idx := strings.IndexByte(entry, '=')
		if idx < 0 {
			out = append(out, entry)
			continue
		}
		key := entry[:idx]
		drop := false
		for _, prefix := range gcpAuthEnvPrefixes {
			if key == prefix || strings.HasPrefix(key, prefix) {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, entry)
		}
	}
	return out
}

// stripEnvKeys removes the named keys from env. Entries are matched
// exactly, or as a prefix when they end in `*` (e.g. `SCW_DEFAULT_*`).
//
// This is the generic form of stripGCPAuthEnv above: that one is an
// unconditional, global strip because the Google provider must never
// see ADC vars on any command, whereas StripEnv is per-command because
// Layer 2 legitimately needs SCW_API_URL set while Layer 3 must not
// see it at all.
func stripEnvKeys(env []string, keys []string) []string {
	if len(keys) == 0 {
		return env
	}
	out := make([]string, 0, len(env))
	for _, entry := range env {
		idx := strings.IndexByte(entry, '=')
		if idx < 0 {
			out = append(out, entry)
			continue
		}
		if envKeyMatches(entry[:idx], keys) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func envKeyMatches(key string, patterns []string) bool {
	for _, pattern := range patterns {
		if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
			if strings.HasPrefix(key, prefix) {
				return true
			}
			continue
		}
		if key == pattern {
			return true
		}
	}
	return false
}

// guardSignals are the signals a signal guard turns into a context
// cancel, so tofu, in its own process group, hears of them only through
// Run's Cancel. SIGQUIT is left out: it asks for a goroutine dump now.
var guardSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}

// signalGuards counts the guards catching guardSignals right now. Signal
// handling is process-wide, so this is too: each guard receives its own
// copy of a signal.
var signalGuards atomic.Int32

// holdSignalGuard records a guard catching guardSignals until the
// returned func is called.
func holdSignalGuard() func() {
	signalGuards.Add(1)
	return sync.OnceFunc(func() { signalGuards.Add(-1) })
}

// raiseWait bounds how long release waits for a signal it raised again
// to end the process. Delivery to itself is asynchronous, and Run must
// not return meanwhile: the caller would run on, and the next tofu's
// Run would catch the signal.
const raiseWait = time.Second

// catchSignalsDuringRun turns a signal into a cancel of ctx while tofu
// runs, so tofu gets its one SIGINT through Cancel whatever the caller
// does about signals. Where no guard holds the signal (live teardown,
// Layer 2, the UI), release raises it again once tofu has stopped,
// ending the process as it would have ended without this. Otherwise the
// process would die at once and leave tofu running with no parent, to
// die mid-destroy on its next write to a broken pipe. Where a guard
// holds it, raising it again would reach that guard as a second signal,
// which abandons a teardown.
func catchSignalsDuringRun(ctx context.Context) (context.Context, func()) {
	caught := make(chan os.Signal, 1)
	signal.Notify(caught, guardSignals...)
	ctx, cancel := context.WithCancel(ctx)
	var sig os.Signal
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		select {
		case sig = <-caught:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		signal.Stop(caught)
		cancel()
		<-watched
		if sig == nil {
			select {
			case sig = <-caught:
			default:
			}
		}
		if sig == nil || signalGuards.Load() > 0 {
			return
		}
		_ = syscall.Kill(os.Getpid(), sig.(syscall.Signal))
		time.Sleep(raiseWait)
	}
}

// cancelKillFallback is how long a cancelled command may ignore SIGINT
// before it is killed. Long enough for tofu to flush state, short enough
// that a hung provider cannot stall a teardown indefinitely.
// A var only so a test can shorten it.
var cancelKillFallback = 20 * time.Second

func (execCommandRunner) Run(ctx context.Context, cmd harness.Command) (harness.CommandResult, error) {
	ctx, release := catchSignalsDuringRun(ctx)
	defer release()
	execCmd := exec.CommandContext(ctx, cmd.Name, cmd.Args...)
	// Interrupt, not kill. The default CommandContext cancel sends
	// SIGKILL, which stops `tofu apply` mid-flight: a resource whose
	// create call has returned but whose state has not been flushed is
	// then live in the account and absent from terraform-live.tfstate --
	// so teardown destroys from an incomplete state and the project
	// delete fails on the resource nothing recorded. SIGINT lets tofu
	// finish writing state before exiting.
	//
	// The child runs in its own process group. An interactive Ctrl-C
	// signals the terminal's whole foreground group; were tofu in it, the
	// terminal's SIGINT plus this Cancel's would be two interrupts, and
	// tofu answers a second one by exiting immediately without saving
	// state. Outside that group, the terminal reaches only infrafactory,
	// whose context cancel sends tofu its one interrupt. Both signals go
	// to the group, as the terminal's did: a provider plugin that
	// survived a kill of tofu alone would hold the output pipe and keep
	// Run waiting.
	//
	// SIGINT first, SIGKILL if it is ignored. WaitDelay would be the
	// obvious way to bound this, but its timer also starts when the child
	// exits NORMALLY -- so a provider plugin still holding the output pipe
	// would turn a successful apply into exec.ErrWaitDelay with truncated
	// output. Arming the fallback inside Cancel scopes it to cancellation
	// only, where a tofu that ignores SIGINT would otherwise hang forever
	// and stop deploy ever reaching registration. It is stopped once Run
	// returns, so it never signals a group id the system has reused.
	var killFallback *time.Timer
	execCmd.Cancel = func() error {
		pgid := execCmd.Process.Pid
		if err := syscall.Kill(-pgid, syscall.SIGINT); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		killFallback = time.AfterFunc(cancelKillFallback, func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })
		return nil
	}
	execCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	execCmd.Dir = cmd.Dir
	execCmd.Env = withEnvOverrides(stripEnvKeys(stripGCPAuthEnv(os.Environ()), cmd.StripEnv), cmd.Env)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	err := execCmd.Run()
	// Run has waited for Cancel, so killFallback is safe to read here.
	if killFallback != nil {
		killFallback.Stop()
	}
	// ProcessState is nil when the command never started (binary not on
	// PATH, bad working directory). -1 keeps that distinguishable from a
	// real exit status, and from the 0 a zero-value struct would carry.
	exitCode := -1
	if execCmd.ProcessState != nil {
		exitCode = execCmd.ProcessState.ExitCode()
	}
	return harness.CommandResult{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: exitCode,
	}, err
}

func withEnvOverrides(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}

	overridden := make(map[string]struct{}, len(overrides))
	pairs := make([]string, 0, len(overrides))
	for key, value := range overrides {
		overridden[key] = struct{}{}
		pairs = append(pairs, key+"="+value)
	}
	sort.Strings(pairs)

	out := make([]string, 0, len(base)+len(pairs))
	for _, entry := range base {
		key := entry
		if idx := bytes.IndexByte([]byte(entry), '='); idx >= 0 {
			key = entry[:idx]
		}
		if _, ok := overridden[key]; ok {
			continue
		}
		out = append(out, entry)
	}
	out = append(out, pairs...)

	return out
}
