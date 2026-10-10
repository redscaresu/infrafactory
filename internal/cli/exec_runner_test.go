package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
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
// file a child creates once its SIGINT handler is installed.
const (
	execHelperEnv      = "INFRAFACTORY_EXEC_HELPER"
	execHelperReadyEnv = "INFRAFACTORY_EXEC_HELPER_READY"
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
// "unguarded" is "parent" with no signal guard, as live teardown has;
// "tree" is "ignore" with an "ignore" grandchild sharing its stdout, as a
// provider plugin might.
func TestExecRunnerHelperProcess(t *testing.T) {
	role := os.Getenv(execHelperEnv)
	if role == "" {
		return
	}
	switch role {
	case "parent":
		// A guard, as runUnderSignals is: the first signal cancels ctx.
		caught := make(chan os.Signal, 4)
		signal.Notify(caught, guardSignals...)
		ctx, cancel := context.WithCancel(context.Background())
		unguard := holdSignalGuard()
		go func() {
			<-caught
			cancel()
		}()
		result, err := execCommandRunner{}.Run(ctx, helperCommand("count"))
		unguard()
		time.Sleep(500 * time.Millisecond) // for any signal Run raised again
		// Canceled only when the cancel signalled the child itself.
		fmt.Printf("%s canceled=%t raised=%d", result.Stdout, errors.Is(err, context.Canceled), len(caught))
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

	assert.Equal(t, "interrupts=1 canceled=true raised=0", stdout.String())
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
