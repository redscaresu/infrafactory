package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
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

func TestWithEnvOverridesDeduplicatesOverriddenKeys(t *testing.T) {
	t.Parallel()

	base := []string{
		"SCW_API_URL=http://base",
		"PATH=/usr/bin",
		"SCW_API_URL=http://stale",
	}
	overrides := map[string]string{
		"SCW_API_URL": "http://override",
	}

	got := withEnvOverrides(base, overrides)
	want := []string{
		"PATH=/usr/bin",
		"SCW_API_URL=http://override",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected env output\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestWithEnvOverridesAppliesSortedOverridesAndPreservesOtherBaseEntries(t *testing.T) {
	t.Parallel()

	base := []string{
		"B=base",
		"A=base",
		"Z=base",
	}
	overrides := map[string]string{
		"A": "override-a",
		"C": "override-c",
	}

	got := withEnvOverrides(base, overrides)
	want := []string{
		"B=base",
		"Z=base",
		"A=override-a",
		"C=override-c",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected env output\nwant: %#v\ngot:  %#v", want, got)
	}
}

// TestStripGCPAuthEnvRemovesAllPrefixes guards the GCP credential-strip
// behavior added 2026-06-02. terraform-provider-google's v5 SDK probes
// the metadata server / ADC chain when any of these env vars are set,
// bypassing the access_token short-circuit and producing the misleading
// "ACCESS_TOKEN_TYPE_UNSUPPORTED" error against fakegcp. Stripping at the
// harness boundary guarantees the LLM's providers.tf credentials win.
func TestStripGCPAuthEnvRemovesAllPrefixes(t *testing.T) {
	t.Parallel()

	in := []string{
		"PATH=/usr/bin",
		"HOME=/Users/x",
		"GOOGLE_APPLICATION_CREDENTIALS=/var/keys/sa.json",
		"GOOGLE_CREDENTIALS=inline-json",
		"GOOGLE_CLOUD_KEYFILE_JSON=/etc/sa.json",
		"GOOGLE_OAUTH_ACCESS_TOKEN=ya29.xyz",
		"CLOUDSDK_CORE_PROJECT=my-proj",
		"CLOUDSDK_AUTH_ACCESS_TOKEN=abc",
		"GCLOUD_PROJECT=other-proj",
		"GOOGLE_REGION=us-central1", // not stripped — generic region setting
		"SCW_API_URL=http://mockway",
	}
	got := stripGCPAuthEnv(in)
	want := []string{
		"PATH=/usr/bin",
		"HOME=/Users/x",
		"GOOGLE_REGION=us-central1",
		"SCW_API_URL=http://mockway",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("strip mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

// TestStripEnvKeysExactAndWildcard covers the per-command strip added
// for Layer 3. Unlike stripGCPAuthEnv, which is unconditional, this one
// is opt-in per command because Layer 2 legitimately needs SCW_API_URL
// while Layer 3 must never see it.
func TestStripEnvKeysExactAndWildcard(t *testing.T) {
	t.Parallel()

	in := []string{
		"PATH=/usr/bin",
		"SCW_API_URL=http://localhost:8080",
		"SCW_ACCESS_KEY=keep-me",
		"SCW_DEFAULT_PROJECT_ID=00000000-0000-0000-0000-000000000000",
		"SCW_DEFAULT_REGION=fr-par",
		"MALFORMED_NO_EQUALS",
	}
	got := stripEnvKeys(in, []string{"SCW_API_URL", "SCW_DEFAULT_*"})
	want := []string{
		"PATH=/usr/bin",
		"SCW_ACCESS_KEY=keep-me",
		"MALFORMED_NO_EQUALS",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("strip mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestStripEnvKeysNoKeysIsIdentity(t *testing.T) {
	t.Parallel()

	in := []string{"A=1", "B=2"}
	if got := stripEnvKeys(in, nil); !slices.Equal(got, in) {
		t.Fatalf("expected identity, got %#v", got)
	}
}

// execHelperEnv selects TestExecRunnerHelperProcess's role when the test
// binary re-runs itself as a child process; execHelperReadyEnv names the
// file a child creates once its SIGINT handler is installed;
// execHelperReleaseEnv names the file a "stream" child waits for.
const (
	execHelperEnv        = "INFRAFACTORY_EXEC_HELPER"
	execHelperReadyEnv   = "INFRAFACTORY_EXEC_HELPER_READY"
	execHelperReleaseEnv = "INFRAFACTORY_EXEC_HELPER_RELEASE"
)

// helperCommand re-runs this test binary as TestExecRunnerHelperProcess
// in the given role.
func helperCommand(role string) harness.Command {
	return harness.Command{
		Name: os.Args[0],
		Args: []string{"-test.run=^TestExecRunnerHelperProcess$"},
		Env:  map[string]string{execHelperEnv: role},
	}
}

// TestExecRunnerHelperProcess is not a test: it is the child process the
// tests below start. "parent" stands in for infrafactory under a signal
// guard, which cancels the runner's context on a signal; "count" stands in for tofu, printing how
// many SIGINTs it saw; "ignore" prints each SIGINT and ignores it;
// "replace-parent" is "parent" with its child in replace mode;
// "stream-parent" is "parent" with its child writing to this stdout;
// "unguarded" is "parent" with no signal guard, as live teardown has;
// "stream" writes a line to stdout and one to stderr, then exits once the
// release file exists;
// "tree" is "ignore" with an "ignore" grandchild sharing its stdout, as a
// provider plugin might.
func TestExecRunnerHelperProcess(t *testing.T) {
	role := os.Getenv(execHelperEnv)
	if role == "" {
		return
	}
	switch role {
	case "stream":
		fmt.Println("out")
		fmt.Fprintln(os.Stderr, "err")
		release := os.Getenv(execHelperReleaseEnv)
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if _, err := os.Stat(release); err == nil {
				os.Exit(0)
			}
		}
		os.Exit(1)
	case "parent", "replace-parent", "stream-parent":
		// A guard, as runUnderSignals is: the first signal cancels ctx.
		ctx, stop := notifyGuarded(context.Background(), signal.NotifyContext)
		// Every signal this process gets: the terminal's, and any Run raised again.
		seen := make(chan os.Signal, 4)
		signal.Notify(seen, guardSignals...)
		child := helperCommand("count")
		if role == "replace-parent" {
			// Replace mode inherits nothing, so the ready file is passed by name.
			child.ReplaceEnv = true
			child.Env[execHelperReadyEnv] = os.Getenv(execHelperReadyEnv)
		}
		if role == "stream-parent" {
			child.Stdout = os.Stdout
		}
		result, err := execCommandRunner{}.Run(ctx, child)
		stop()
		time.Sleep(500 * time.Millisecond)
		// Canceled only when the cancel signalled the child itself.
		fmt.Printf("%s canceled=%t signals=%d", result.Stdout, errors.Is(err, context.Canceled), len(seen))
		if role == "stream-parent" {
			// The child wrote its count to this stdout, so the result holds none.
			fmt.Printf(" buffered=%d", len(result.Stdout))
		}
		os.Exit(0)
	case "unguarded":
		// As live teardown is: nothing catches a signal.
		_, _ = execCommandRunner{}.Run(context.Background(), helperCommand("count"))
		os.Exit(0)
	}

	interrupts := make(chan os.Signal, 8)
	signal.Notify(interrupts, os.Interrupt)
	if role == "tree" {
		role = "ignore"
		ready := os.Getenv(execHelperReadyEnv) + ".grandchild"
		grandchild := exec.Command(os.Args[0], "-test.run=^TestExecRunnerHelperProcess$")
		grandchild.Env = append(os.Environ(), execHelperEnv+"=ignore", execHelperReadyEnv+"="+ready)
		grandchild.Stdout = os.Stdout
		_ = grandchild.Start()
		for _, err := os.Stat(ready); err != nil; _, err = os.Stat(ready) {
			time.Sleep(10 * time.Millisecond)
		}
	}
	_ = os.WriteFile(os.Getenv(execHelperReadyEnv), nil, 0o600)
	deadline := time.After(10 * time.Second)
	count := 0
	for {
		select {
		case <-interrupts:
			count++
			if role == "ignore" {
				fmt.Println("interrupt")
				continue
			}
			// Any second interrupt arrives within this window.
			deadline = time.After(time.Second)
		case <-deadline:
			// To a file first: with an unguarded parent gone, the write to
			// stdout's broken pipe kills this process, as it would tofu.
			_ = os.WriteFile(os.Getenv(execHelperReadyEnv)+".count", fmt.Appendf(nil, "interrupts=%d", count), 0o600)
			fmt.Printf("interrupts=%d", count)
			os.Exit(0)
		}
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, 10*time.Second, 10*time.Millisecond, "helper never became ready")
}

// startHelperParent starts the test binary as a helper parent in its own
// process group, standing in for the terminal's foreground group, so a
// signal to that group does not reach `go test`. It returns once the
// parent's child has its SIGINT handler installed.
func startHelperParent(t *testing.T, role string) (*exec.Cmd, *bytes.Buffer, string) {
	t.Helper()
	ready := filepath.Join(t.TempDir(), "ready")
	parent := exec.Command(os.Args[0], "-test.run=^TestExecRunnerHelperProcess$")
	parent.Env = append(os.Environ(), execHelperEnv+"="+role, execHelperReadyEnv+"="+ready)
	parent.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout bytes.Buffer
	parent.Stdout = &stdout
	require.NoError(t, parent.Start())
	waitForFile(t, ready)
	return parent, &stdout, ready
}

// An interactive Ctrl-C signals the terminal's whole foreground process
// group. tofu must see exactly one SIGINT, and it must be the runner's
// cancel: a second makes tofu exit at once without saving state. The
// guard above Run handles the signal, so Run must not raise it again: a
// guard reads a second signal as "abandon the teardown".
func TestExecRunnerGroupInterruptReachesChildOnce(t *testing.T) {
	t.Parallel()

	parent, stdout, _ := startHelperParent(t, "parent")
	require.NoError(t, syscall.Kill(-parent.Process.Pid, syscall.SIGINT))
	require.NoError(t, parent.Wait())

	assert.Equal(t, "interrupts=1 canceled=true signals=1", stdout.String())
}

// A streamed command is in its own process group too: the terminal's
// Ctrl-C reaches it only as the runner's one SIGINT, and its output
// reaches the caller's writer, not the result.
func TestExecRunnerStreamingOneSIGINT(t *testing.T) {
	t.Parallel()

	parent, stdout, _ := startHelperParent(t, "stream-parent")
	require.NoError(t, syscall.Kill(-parent.Process.Pid, syscall.SIGINT))
	require.NoError(t, parent.Wait())

	assert.Equal(t, "interrupts=1 canceled=true signals=1 buffered=0", stdout.String())
}

// A replace-mode command is in its own process group too, and hears the
// terminal's Ctrl-C only as the runner's one SIGINT.
func TestExecRunnerReplaceEnvGroupInterruptReachesChildOnce(t *testing.T) {
	t.Parallel()

	parent, stdout, _ := startHelperParent(t, "replace-parent")
	require.NoError(t, syscall.Kill(-parent.Process.Pid, syscall.SIGINT))
	require.NoError(t, parent.Wait())

	assert.Equal(t, "interrupts=1 canceled=true signals=1", stdout.String())
}

// With no guard above it -- live teardown, Layer 2, the UI -- Run is the
// guard: tofu still gets one SIGINT through the cancel, and the signal
// then ends the process as it would have without Run catching it. Were
// the process to die at once, tofu would run on with no parent.
func TestExecRunnerStopsTofuBeforeAnUnguardedProcessDies(t *testing.T) {
	t.Parallel()

	// Named, not read from guardSignals: these are the signals a terminal
	// or a supervisor ends a session with, and each must be guarded.
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP} {
		t.Run(sig.String(), func(t *testing.T) {
			t.Parallel()

			parent, _, ready := startHelperParent(t, "unguarded")
			require.NoError(t, syscall.Kill(-parent.Process.Pid, sig))
			var exitErr *exec.ExitError
			require.ErrorAs(t, parent.Wait(), &exitErr, "the parent ended normally")
			status := exitErr.Sys().(syscall.WaitStatus)
			assert.True(t, status.Signaled() && status.Signal() == sig, "parent ended by %v, not by %v", exitErr, sig)

			// An orphaned child, never interrupted, writes only after 10s.
			var count []byte
			require.Eventually(t, func() bool {
				var err error
				count, err = os.ReadFile(ready + ".count")
				return err == nil
			}, 15*time.Second, 10*time.Millisecond)
			assert.Equal(t, "interrupts=1", string(count))
		})
	}
}

// A hangup the process started ignoring, as under nohup, stays ignored:
// it neither interrupts tofu nor ends the process. The SIGTERM after it
// does both.
func TestExecRunnerLeavesAnIgnoredHangupIgnored(t *testing.T) {
	t.Parallel()

	ready := filepath.Join(t.TempDir(), "ready")
	// trap '' sets SIG_IGN, which exec keeps.
	parent := exec.Command("/bin/sh", "-c", `trap '' HUP; exec "$0" "$@"`, os.Args[0], "-test.run=^TestExecRunnerHelperProcess$")
	parent.Env = append(os.Environ(), execHelperEnv+"=unguarded", execHelperReadyEnv+"="+ready)
	parent.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, parent.Start())
	waitForFile(t, ready)

	require.NoError(t, syscall.Kill(-parent.Process.Pid, syscall.SIGHUP))
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, syscall.Kill(-parent.Process.Pid, syscall.SIGTERM))

	var exitErr *exec.ExitError
	require.ErrorAs(t, parent.Wait(), &exitErr, "the parent ended normally")
	status := exitErr.Sys().(syscall.WaitStatus)
	assert.True(t, status.Signaled() && status.Signal() == syscall.SIGTERM, "parent ended by %v, not by SIGTERM", exitErr)
	count, err := os.ReadFile(ready + ".count")
	require.NoError(t, err)
	assert.Equal(t, "interrupts=1", string(count))
}

// A cancelled context with no terminal involved -- CI, a timeout --
// interrupts the child, then kills it once cancelKillFallback passes.
func TestExecRunnerCancelInterruptsThenKills(t *testing.T) {
	saved := cancelKillFallback
	cancelKillFallback = 200 * time.Millisecond
	t.Cleanup(func() { cancelKillFallback = saved })

	ready := filepath.Join(t.TempDir(), "ready")
	t.Setenv(execHelperReadyEnv, ready)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Not waitForFile: require may not stop a test from a goroutine.
		// A child that never readies exits 0 unsignalled, failing below.
		for _, err := os.Stat(ready); err != nil; _, err = os.Stat(ready) {
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()

	result, err := execCommandRunner{}.Run(ctx, helperCommand("ignore"))

	require.Error(t, err)
	assert.Equal(t, "interrupt\n", string(result.Stdout))
	assert.Equal(t, -1, result.ExitCode, "killed by signal, not exited")
}

// The cancel's signals reach the child's descendants too: a grandchild
// that outlived the kill would hold the output pipe, and Run would wait
// for it (here, until it gives up after 10s).
func TestExecRunnerCancelSignalsTheChildsGroup(t *testing.T) {
	saved := cancelKillFallback
	cancelKillFallback = 200 * time.Millisecond
	t.Cleanup(func() { cancelKillFallback = saved })

	ready := filepath.Join(t.TempDir(), "ready")
	t.Setenv(execHelperReadyEnv, ready)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for _, err := os.Stat(ready); err != nil; _, err = os.Stat(ready) {
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()

	start := time.Now()
	result, err := execCommandRunner{}.Run(ctx, helperCommand("tree"))

	require.Error(t, err)
	assert.Equal(t, "interrupt\ninterrupt\n", string(result.Stdout), "child and grandchild each interrupted once")
	assert.Less(t, time.Since(start), 5*time.Second, "the grandchild outlived the kill")
}

// A streamed command's output reaches the caller's writers while it
// runs: through a copy for a plain writer, and for an *os.File, which is
// what production passes, as the child's own descriptor. Run returns once
// the child exits, and the result holds neither stream.
func TestExecRunnerStreamsBeforeExit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		pipe func(t *testing.T) (io.Reader, io.Writer)
	}{
		{"writer", func(t *testing.T) (io.Reader, io.Writer) {
			r, w := io.Pipe()
			t.Cleanup(func() { _ = w.Close() })
			return r, w
		}},
		{"os.Pipe", func(t *testing.T) (io.Reader, io.Writer) {
			r, w, err := os.Pipe()
			require.NoError(t, err)
			t.Cleanup(func() { _ = w.Close(); _ = r.Close() })
			return r, w
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stdoutR, stdoutW := tc.pipe(t)
			stderrR, stderrW := tc.pipe(t)
			release := filepath.Join(t.TempDir(), "release")
			cmd := helperCommand("stream")
			cmd.Env[execHelperReleaseEnv] = release
			cmd.Stdout, cmd.Stderr = stdoutW, stderrW
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			type ran struct {
				result harness.CommandResult
				err    error
			}
			done := make(chan ran, 1)
			go func() {
				result, err := execCommandRunner{}.Run(ctx, cmd)
				done <- ran{result, err}
			}()

			assert.Equal(t, "out", readLine(t, stdoutR))
			assert.Equal(t, "err", readLine(t, stderrR))
			select {
			case <-done:
				t.Fatal("Run returned before the child exited")
			default:
			}

			require.NoError(t, os.WriteFile(release, nil, 0o600))
			select {
			case got := <-done:
				require.NoError(t, got.err)
				assert.Empty(t, got.result.Stdout)
				assert.Empty(t, got.result.Stderr)
			case <-time.After(10 * time.Second):
				t.Fatal("Run did not return once the child exited")
			}
		})
	}
}

// readLine reads one line from r, failing the test after 10s.
func readLine(t *testing.T, r io.Reader) string {
	t.Helper()
	line := make(chan string, 1)
	go func() {
		s, _ := bufio.NewReader(r).ReadString('\n')
		line <- s
	}()
	select {
	case s := <-line:
		return strings.TrimSuffix(s, "\n")
	case <-time.After(10 * time.Second):
		t.Fatal("no line within 10s")
		return ""
	}
}

// A cancelled command never reports success, streamed or not: tofu
// answers its one SIGINT by saving state and exiting 0, and Run still
// returns the cancel.
func TestExecRunnerStreamingCancelNeverSucceeds(t *testing.T) {
	t.Parallel()

	ready := filepath.Join(t.TempDir(), "ready")
	cmd := helperCommand("count")
	cmd.Env[execHelperReadyEnv] = ready
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for ctx.Err() == nil {
			if _, err := os.Stat(ready); err == nil {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	result, err := execCommandRunner{}.Run(ctx, cmd)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, result.ExitCode, "the child trapped SIGINT and exited 0")
	assert.Equal(t, "interrupts=1", stdout.String())
}

// Every guard registers with signalGuards while it catches signals, so
// Run never raises a signal again into it. Not parallel: the count is
// process-wide.
func TestSignalGuardsAreCounted(t *testing.T) {
	before := signalGuards.Load()
	var during []int32
	count := func() { during = append(during, signalGuards.Load()) }

	_, err := runUnderSignals(guardCmd(&strings.Builder{}), signalRuntime(), layer3AWS, notifyFromParent, func(context.Context) error {
		count()
		return nil
	})
	require.NoError(t, err)
	_, err = runDeployApply(guardCmd(&strings.Builder{}), context.Background(), notifyFromParent, func(context.Context) (*harness.SandboxDeployResult, error) {
		count()
		return nil, nil
	})
	require.NoError(t, err)

	assert.Equal(t, []int32{before + 1, before + 1}, during, "runUnderSignals, runDeployApply")
	assert.Equal(t, before, signalGuards.Load(), "released")
}

// envBin is exec'd directly to print the environment it was given, exactly:
// `sh -c env` would add PWD, SHLVL and _.
const envBin = "/usr/bin/env"

// envNulArg makes envBin end each entry with NUL, not newline, so a value
// holding a newline stays one entry.
const envNulArg = "-0"

// runEnv runs cmd, an envBin command, and returns the entries it printed.
func runEnv(t *testing.T, cmd harness.Command) []string {
	t.Helper()
	cmd.Args = []string{envNulArg}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := execCommandRunner{}.Run(ctx, cmd)
	require.NoError(t, err, "%s", result.Stderr)
	return envEntries(string(result.Stdout))
}

// envEntries splits what envBin printed with envNulArg into its entries.
func envEntries(out string) []string {
	return strings.FieldsFunc(out, func(r rune) bool { return r == 0 })
}

// assertEnv checks the entries a child printed against want, in order. A
// failure names keys and never prints a value: got can hold this
// process's environment, and its secrets would land in the log.
func assertEnv(t *testing.T, want, got []string) {
	t.Helper()
	if !assert.Equal(t, envKeys(want), envKeys(got), "env keys") {
		return
	}
	for i, entry := range got {
		key, _, _ := strings.Cut(entry, "=")
		assert.True(t, entry == want[i], "env %s: the value differs", key)
	}
}

func envKeys(entries []string) []string {
	keys := make([]string, len(entries))
	for i, entry := range entries {
		keys[i], _, _ = strings.Cut(entry, "=")
	}
	return keys
}

// Replace mode passes PATH and the named variables, and nothing else of
// this process's environment: each of these would change what tofu, the
// provider, git or ssh does, or where it sends a request.
func TestExecRunnerReplaceEnvPassesOnlyPathAndEnv(t *testing.T) {
	for _, key := range []string{
		"HOME", "TF_CLI_CONFIG_FILE", "TF_REATTACH_PROVIDERS", "TF_LOG_PATH",
		"TF_PLUGIN_CACHE_DIR", "TF_CLI_ARGS", "TF_CLI_ARGS_init", "TF_VAR_region",
		"TF_ENCRYPTION", "TF_TOKEN_app_terraform_io", "AWS_IGNORE_CONFIGURED_ENDPOINT_URLS",
		"AWS_SESSION_TOKEN", "AWS_PROFILE", "SSH_AUTH_SOCK", "GIT_CONFIG_GLOBAL",
		"HTTPS_PROXY", "NO_PROXY",
	} {
		t.Setenv(key, "planted")
	}

	got := runEnv(t, harness.Command{
		Name:       envBin,
		Env:        map[string]string{"AWS_REGION": "us-east-1", "TF_IN_AUTOMATION": "1"},
		ReplaceEnv: true,
	})

	assertEnv(t, []string{"PATH=" + os.Getenv("PATH"), "AWS_REGION=us-east-1", "TF_IN_AUTOMATION=1"}, got)
}

// With no PATH to pass and nothing named, the child's environment is
// empty, not this process's: os/exec reads a nil Env as "inherit".
func TestExecRunnerReplaceEnvWithNothingIsEmpty(t *testing.T) {
	t.Setenv("PATH", "") // restores PATH after the test
	require.NoError(t, os.Unsetenv("PATH"))

	assertEnv(t, nil, runEnv(t, harness.Command{Name: envBin, ReplaceEnv: true}))
}

func TestExecRunnerReplaceEnvPathFromEnvWins(t *testing.T) {
	require.NotEqual(t, "/from-env", os.Getenv("PATH"))

	got := runEnv(t, harness.Command{Name: envBin, Env: map[string]string{"PATH": "/from-env"}, ReplaceEnv: true})

	assertEnv(t, []string{"PATH=/from-env"}, got)
}

// A streamed command's environment is replace mode's too: PATH and Env,
// and nothing else of this process's.
func TestExecRunnerStreamingReplaceEnv(t *testing.T) {
	t.Setenv("TF_CLI_ARGS", "planted")
	var stdout bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := execCommandRunner{}.Run(ctx, harness.Command{
		Name:       envBin,
		Args:       []string{envNulArg},
		Env:        map[string]string{"AWS_REGION": "us-east-1"},
		ReplaceEnv: true,
		Stdout:     &stdout,
	})

	require.NoError(t, err)
	assertEnv(t, []string{"PATH=" + os.Getenv("PATH"), "AWS_REGION=us-east-1"}, envEntries(stdout.String()))
}

// Without ReplaceEnv the child inherits as it always has: this process's
// environment in order, minus GCP auth and StripEnv, then Env sorted.
func TestExecRunnerInheritedEnvIsUnchanged(t *testing.T) {
	t.Setenv("GOOGLE_CREDENTIALS", "planted")
	t.Setenv("CLOUDSDK_CORE_PROJECT", "planted")
	t.Setenv("AWS_PROFILE", "planted")
	t.Setenv("AWS_SESSION_TOKEN", "planted")
	t.Setenv("INFRAFACTORY_KEPT", "kept")
	t.Setenv("INFRAFACTORY_OVERRIDDEN", "parent")
	env := map[string]string{"INFRAFACTORY_OVERRIDDEN": "env", "INFRAFACTORY_ADDED": "env"}

	var want []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		gcpAuth := slices.ContainsFunc(gcpAuthEnvPrefixes, func(prefix string) bool { return strings.HasPrefix(key, prefix) })
		if gcpAuth || strings.HasPrefix(key, "AWS_") || env[key] != "" {
			continue
		}
		want = append(want, entry)
	}
	want = append(want, "INFRAFACTORY_ADDED=env", "INFRAFACTORY_OVERRIDDEN=env")
	require.True(t, slices.Contains(want, "INFRAFACTORY_KEPT=kept"), "want lost INFRAFACTORY_KEPT")

	got := runEnv(t, harness.Command{Name: envBin, Env: env, StripEnv: []string{"AWS_*"}})

	assertEnv(t, want, got)
}

// Replace mode has nothing to strip, so a command asking for both is a
// mistake, refused before anything runs.
func TestExecRunnerRefusesReplaceEnvWithStripEnv(t *testing.T) {
	t.Parallel()

	started := filepath.Join(t.TempDir(), "started")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := execCommandRunner{}.Run(ctx, harness.Command{
		Name:       "/usr/bin/touch",
		Args:       []string{started},
		ReplaceEnv: true,
		StripEnv:   []string{"AWS_*"},
	})

	require.ErrorIs(t, err, errReplaceEnvWithStripEnv)
	assert.Equal(t, -1, result.ExitCode)
	assert.NoFileExists(t, started, "the command ran")
}

// runTofu hands tofu exactly the env it is given: the planted HOME and
// TF_CLI_ARGS of this process never reach it.
func TestRunTofuPassesExactlyItsEnv(t *testing.T) {
	bin := t.TempDir()
	require.NoError(t, os.Symlink(envBin, filepath.Join(bin, "tofu")))
	t.Setenv("PATH", bin)
	t.Setenv("HOME", "/planted-home")
	t.Setenv("TF_CLI_ARGS", "-planted")
	env := []string{"AWS_REGION=us-east-1", "PATH=" + bin, "TF_IN_AUTOMATION=1"}

	stdout, stderr, err := runTofu(t.TempDir(), env, envNulArg)

	require.NoError(t, err, stderr)
	assertEnv(t, env, envEntries(stdout))
}
