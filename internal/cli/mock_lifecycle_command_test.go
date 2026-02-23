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

type fakeMockLifecycle struct {
	stopErr     error
	statusErr   error
	logsErr     error
	statusValue string
	logsValue   string

	stopCalls   int
	statusCalls int
	logsCalls   int
	stopCtx     context.Context
	statusCtx   context.Context
	logsCtx     context.Context
}

func (f *fakeMockLifecycle) Stop(ctx context.Context, _ config.MockwayConfig) error {
	f.stopCalls++
	f.stopCtx = ctx
	return f.stopErr
}

func (f *fakeMockLifecycle) Status(ctx context.Context, _ config.MockwayConfig) (string, error) {
	f.statusCalls++
	f.statusCtx = ctx
	return f.statusValue, f.statusErr
}

func (f *fakeMockLifecycle) Logs(ctx context.Context, _ config.MockwayConfig) (string, error) {
	f.logsCalls++
	f.logsCtx = ctx
	return f.logsValue, f.logsErr
}

func TestMockStopCommandSuccess(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	lifecycle := &fakeMockLifecycle{}
	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockStop: lifecycle,
		},
	}

	cmd := newMockStopCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute mock stop: %v", err)
	}
	if lifecycle.stopCalls != 1 {
		t.Fatalf("expected stop call count 1, got %d", lifecycle.stopCalls)
	}
	if !strings.Contains(stdout.String(), "Status: success") {
		t.Fatalf("expected success output, got:\n%s", stdout.String())
	}
}

func TestMockStopCommandFailure(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	lifecycle := &fakeMockLifecycle{stopErr: errors.New("container not running")}
	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockStop: lifecycle,
		},
	}

	cmd := newMockStopCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stdout.String(), "- mock/preflight: fail") {
		t.Fatalf("expected preflight failure output, got:\n%s", stdout.String())
	}
}

func TestMockStatusCommandSuccess(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	lifecycle := &fakeMockLifecycle{statusValue: "Up 3 minutes"}
	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockStatus: lifecycle,
		},
	}

	cmd := newMockStatusCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute mock status: %v", err)
	}
	if lifecycle.statusCalls != 1 {
		t.Fatalf("expected status call count 1, got %d", lifecycle.statusCalls)
	}
	if !strings.Contains(stdout.String(), "- mock/status: pass (Up 3 minutes)") {
		t.Fatalf("expected status detail output, got:\n%s", stdout.String())
	}
}

func TestMockLogsCommandSuccess(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	lifecycle := &fakeMockLifecycle{logsValue: "line1\nline2"}
	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockLogs: lifecycle,
		},
	}

	cmd := newMockLogsCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute mock logs: %v", err)
	}
	if lifecycle.logsCalls != 1 {
		t.Fatalf("expected logs call count 1, got %d", lifecycle.logsCalls)
	}
	if !strings.Contains(stdout.String(), "- mock/logs: pass (line1") {
		t.Fatalf("expected logs detail output, got:\n%s", stdout.String())
	}
}

func TestMockStatusCommandFailure(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	lifecycle := &fakeMockLifecycle{statusErr: errors.New("docker not available")}
	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockStatus: lifecycle,
		},
	}

	cmd := newMockStatusCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stdout.String(), "- mock/preflight: fail") {
		t.Fatalf("expected preflight failure output, got:\n%s", stdout.String())
	}
}

func TestMockLifecycleCommandsPropagateContext(t *testing.T) {
	t.Parallel()

	type contextKey string
	const key contextKey = "ctx-key"

	h := newCommandTestHarness(t)
	lifecycle := &fakeMockLifecycle{
		statusValue: "ok",
		logsValue:   "ok",
	}
	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockStop:   lifecycle,
			MockStatus: lifecycle,
			MockLogs:   lifecycle,
		},
	}

	commands := map[string]*cobra.Command{
		"stop":   newMockStopCommandForTest(opts),
		"status": newMockStatusCommandForTest(opts),
		"logs":   newMockLogsCommandForTest(opts),
	}
	for name, cmd := range commands {
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"--config", h.ConfigPath})
		commandCtx := context.WithValue(context.Background(), key, name)
		if err := cmd.ExecuteContext(commandCtx); err != nil {
			t.Fatalf("execute mock %s with context: %v", name, err)
		}
	}

	if got := lifecycle.stopCtx.Value(key); got != "stop" {
		t.Fatalf("expected stop context value %q, got %#v", "stop", got)
	}
	if got := lifecycle.statusCtx.Value(key); got != "status" {
		t.Fatalf("expected status context value %q, got %#v", "status", got)
	}
	if got := lifecycle.logsCtx.Value(key); got != "logs" {
		t.Fatalf("expected logs context value %q, got %#v", "logs", got)
	}
}

func newMockStopCommandForTest(opts runtimeOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "stop",
		RunE: withRuntimeWithOptions("mock stop", opts, runMockStopCommand),
	}
	cmd.Flags().String("config", config.DefaultPath, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	return cmd
}

func newMockStatusCommandForTest(opts runtimeOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "status",
		RunE: withRuntimeWithOptions("mock status", opts, runMockStatusCommand),
	}
	cmd.Flags().String("config", config.DefaultPath, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	return cmd
}

func newMockLogsCommandForTest(opts runtimeOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "logs",
		RunE: withRuntimeWithOptions("mock logs", opts, runMockLogsCommand),
	}
	cmd.Flags().String("config", config.DefaultPath, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	return cmd
}
