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
}

func (f *fakeMockLifecycle) Stop(context.Context, config.MockwayConfig) error {
	f.stopCalls++
	return f.stopErr
}

func (f *fakeMockLifecycle) Status(context.Context, config.MockwayConfig) (string, error) {
	f.statusCalls++
	return f.statusValue, f.statusErr
}

func (f *fakeMockLifecycle) Logs(context.Context, config.MockwayConfig) (string, error) {
	f.logsCalls++
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
