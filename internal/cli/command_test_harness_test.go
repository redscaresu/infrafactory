package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
)

type CommandTestHarness struct {
	T            *testing.T
	WorkspaceDir string
	ConfigPath   string
	ScenarioPath string
}

// OutputDir returns the workspace-scoped output dir that test
// runtimes should write generated HCL into. Replaces the
// relative-default `./output` that caused the May 2026
// parallel-subtest race (see M71 / M81).
func (h *CommandTestHarness) OutputDir() string {
	return filepath.Join(h.WorkspaceDir, "output")
}

// PitfallsDir returns the workspace-scoped pitfalls dir. The
// stray `internal/cli/pitfalls/` directory that kept appearing
// in `git status` came from tests writing to the relative-default
// `./pitfalls/` — using this helper avoids the leak.
func (h *CommandTestHarness) PitfallsDir() string {
	return filepath.Join(h.WorkspaceDir, "pitfalls")
}

// RunstoreRoot returns the workspace-scoped run-store root. Avoids
// the relative-default `./.infrafactory/runs` collision when two
// tests share the same wall-clock-second runID (M71 root cause).
func (h *CommandTestHarness) RunstoreRoot() string {
	return filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
}

// isolatedRunOpts returns a runtimeOptions pre-wired with
// workspace-scoped output/pitfalls/runstore paths so a test cannot
// share filesystem state with parallel siblings. The `customize`
// callback receives the loaded config after the path remap and may
// return a mutated copy for test-specific cfg tweaks (RepairIterations
// Max, Validation.Layers toggles, etc.). Pass nil if none are needed.
//
// Callers populate `.deps` separately. The helper does not call
// t.Setenv (which would conflict with t.Parallel) — instead it sets
// the runstoreRoot field on runtimeOptions directly.
func isolatedRunOpts(h *CommandTestHarness, customize func(config.Config) config.Config) runtimeOptions {
	return runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Paths.Output = h.OutputDir()
			cfg.Paths.Pitfalls = h.PitfallsDir()
			if customize != nil {
				cfg = customize(cfg)
			}
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		runstoreRoot:   h.RunstoreRoot(),
	}
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
