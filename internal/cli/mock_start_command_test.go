package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/spf13/cobra"
)

type fakeMockStarter struct {
	err     error
	calls   int
	mockway config.MockwayConfig
	lastCtx context.Context
}

func (f *fakeMockStarter) Start(ctx context.Context, cfg config.MockwayConfig) error {
	f.calls++
	f.mockway = cfg
	f.lastCtx = ctx
	return f.err
}

func TestMockStartCommandSuccess(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	starter := &fakeMockStarter{}
	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockStart: starter,
		},
	}

	cmd := newMockStartCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute mock start: %v", err)
	}
	if starter.calls != 1 {
		t.Fatalf("expected starter call count 1, got %d", starter.calls)
	}
	if starter.mockway.URL != "http://localhost:8080" {
		t.Fatalf("expected mockway URL from config, got %q", starter.mockway.URL)
	}
	if !strings.Contains(stdout.String(), "Status: success") {
		t.Fatalf("expected success output, got:\n%s", stdout.String())
	}
}

func TestMockStartCommandPreflightFailure(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	starter := &fakeMockStarter{err: errors.New("docker binary not found in PATH")}
	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockStart: starter,
		},
	}

	cmd := newMockStartCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected mock start failure")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T (%v)", err, err)
	}
	if cliErr.Op != "mock start" || cliErr.Code != errorCodeCommandFailed {
		t.Fatalf("expected mock start/%s CLI error, got op=%q code=%q", errorCodeCommandFailed, cliErr.Op, cliErr.Code)
	}
	if ExitCodeForError(err) != ExitCodeRuntime {
		t.Fatalf("expected runtime exit code, got %d", ExitCodeForError(err))
	}
	if !strings.Contains(stdout.String(), "- mock/preflight: fail") {
		t.Fatalf("expected preflight failure stage in output, got:\n%s", stdout.String())
	}
}

func TestMockStartCommandPropagatesContext(t *testing.T) {
	t.Parallel()

	type contextKey string
	const key contextKey = "ctx-key"

	h := newCommandTestHarness(t)
	starter := &fakeMockStarter{}
	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockStart: starter,
		},
	}

	cmd := newMockStartCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", h.ConfigPath})

	commandCtx := context.WithValue(context.Background(), key, "mock-start")
	if err := cmd.ExecuteContext(commandCtx); err != nil {
		t.Fatalf("execute mock start with context: %v", err)
	}
	if starter.lastCtx == nil {
		t.Fatal("expected starter context capture")
	}
	if got := starter.lastCtx.Value(key); got != "mock-start" {
		t.Fatalf("expected propagated context value %q, got %#v", "mock-start", got)
	}
}

func newMockStartCommandForTest(opts runtimeOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "start",
		RunE: withRuntimeWithOptions("mock start", opts, runMockStartCommand),
	}
	cmd.Flags().String("config", config.DefaultPath, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	return cmd
}
