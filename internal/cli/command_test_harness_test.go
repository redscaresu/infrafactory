package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type CommandTestHarness struct {
	T            *testing.T
	WorkspaceDir string
	ConfigPath   string
	ScenarioPath string
}

type CommandRunResult struct {
	Stdout string
	Stderr string
	Err    error
}

func newCommandTestHarness(t *testing.T) *CommandTestHarness {
	t.Helper()

	workspace := t.TempDir()
	configPath := filepath.Join(workspace, "infrafactory.yaml")
	scenarioPath := filepath.Join(workspace, "scenarios", "training", "example.yaml")

	mustWriteFile(t, configPath, `version: "1.0"
agent:
  type: claude-code
mockway:
  url: http://localhost:8080
`)
	mustWriteFile(t, scenarioPath, `scenario: example-scenario
version: "1.0"
cloud: scaleway
description: example
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`)

	return &CommandTestHarness{
		T:            t,
		WorkspaceDir: workspace,
		ConfigPath:   configPath,
		ScenarioPath: scenarioPath,
	}
}

func (h *CommandTestHarness) Run(args ...string) CommandRunResult {
	h.T.Helper()

	root := NewRootCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)

	err := root.Execute()
	return CommandRunResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
		Err:    err,
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %q: %v", path, err)
	}
}

func TestCommandTestHarnessBuildsDeterministicFixtures(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	if _, err := os.Stat(h.ConfigPath); err != nil {
		t.Fatalf("expected config fixture to exist: %v", err)
	}
	if _, err := os.Stat(h.ScenarioPath); err != nil {
		t.Fatalf("expected scenario fixture to exist: %v", err)
	}
}

func TestCommandTestHarnessCapturesCommandOutputsAndError(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	result := h.Run("generate", h.ScenarioPath, "--config", h.ConfigPath)
	if result.Err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(result.Err, ErrNotImplemented) {
		t.Fatalf("expected concrete generator failure, got: %v", result.Err)
	}
	var cliErr *CLIError
	if !errors.As(result.Err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T (%v)", result.Err, result.Err)
	}
	if result.Stdout != "" {
		t.Fatalf("expected empty stdout for failed command, got: %q", result.Stdout)
	}
	if result.Stderr != "" {
		t.Fatalf("expected empty stderr for failed command, got: %q", result.Stderr)
	}
}
