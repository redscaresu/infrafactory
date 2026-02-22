package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/spf13/cobra"
)

type fakeStaticHarness struct {
	result  *harness.StaticResult
	err     error
	calls   int
	lastDir string
	lastEnv map[string]string
}

func (f *fakeStaticHarness) Run(_ context.Context, workDir string, env map[string]string) (*harness.StaticResult, error) {
	f.calls++
	f.lastDir = workDir
	f.lastEnv = env
	return f.result, f.err
}

func TestValidateCommandStaticSuccess(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	outputRoot := filepath.Join(h.WorkspaceDir, "output")
	planJSON := []byte(`{"planned_values":{"root_module":{}}}`)
	static := &fakeStaticHarness{
		result: &harness.StaticResult{
			Stages: []harness.StageResult{
				{Stage: "init"},
				{Stage: "validate"},
				{Stage: "plan"},
				{Stage: "show"},
			},
			PlanJSON: planJSON,
		},
	}

	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Paths.Output = outputRoot
			cfg.Validation.Layers.Static.PolicyPaths = nil
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps:           RuntimeDependencies{Static: static},
	}

	cmd := newValidateCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute validate: %v", err)
	}
	if static.calls != 1 {
		t.Fatalf("expected one static harness call, got %d", static.calls)
	}
	if static.lastDir != filepath.Join(outputRoot, "example-scenario") {
		t.Fatalf("unexpected static work dir: %s", static.lastDir)
	}
	if static.lastEnv["SCW_API_URL"] != "http://localhost:8080" {
		t.Fatalf("unexpected SCW_API_URL: %q", static.lastEnv["SCW_API_URL"])
	}

	output := stdout.String()
	checks := []string{
		"Command: validate",
		"Status: success",
		"- static/init: pass",
		"- static/opa: pass",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("expected output to contain %q, got:\n%s", check, output)
		}
	}
}

func TestValidateCommandStageFailure(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	stageRootErr := errors.New("validate failed")
	static := &fakeStaticHarness{
		result: &harness.StaticResult{
			Stages: []harness.StageResult{{Stage: "init"}, {Stage: "validate", Cmd: []string{"tofu", "validate"}}},
		},
		err: &harness.StageError{
			StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}, Stderr: "bad config"},
			Err:         stageRootErr,
		},
	}

	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps:           RuntimeDependencies{Static: static},
	}

	cmd := newValidateCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T (%v)", err, err)
	}
	if cliErr.Op != "validate" || cliErr.Code != errorCodeCommandFailed {
		t.Fatalf("expected validate/%s CLI error, got op=%q code=%q", errorCodeCommandFailed, cliErr.Op, cliErr.Code)
	}
	if ExitCodeForError(err) != ExitCodeRuntime {
		t.Fatalf("expected runtime exit code, got %d", ExitCodeForError(err))
	}
	if !strings.Contains(stdout.String(), "Status: failed") {
		t.Fatalf("expected failed status in output, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "- static/validate check=validate") {
		t.Fatalf("expected validate stage failure in output, got:\n%s", stdout.String())
	}
}

func TestValidateCommandPolicyFailure(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	planPath := filepath.Join("..", "harness", "testdata", "opa", "plan-fail.json")
	policyPath := filepath.Join("..", "harness", "testdata", "opa", "policy.rego")
	planJSON, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read plan fixture: %v", err)
	}
	static := &fakeStaticHarness{
		result: &harness.StaticResult{
			Stages:   []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}},
			PlanJSON: planJSON,
		},
	}

	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Validation.Layers.Static.PolicyPaths = []string{policyPath}
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps:           RuntimeDependencies{Static: static},
	}

	cmd := newValidateCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected validation failure")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T (%v)", err, err)
	}
	if cliErr.Op != "validate" || cliErr.Code != errorCodeCommandFailed {
		t.Fatalf("expected validate/%s CLI error, got op=%q code=%q", errorCodeCommandFailed, cliErr.Op, cliErr.Code)
	}
	if !strings.Contains(stdout.String(), "policy=test.plan") {
		t.Fatalf("expected policy failure in output, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "- static/opa: fail") {
		t.Fatalf("expected opa stage failure in output, got:\n%s", stdout.String())
	}
}

func TestValidateCommandSkipsWhenStaticLayerDisabled(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	static := &fakeStaticHarness{}
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Validation.Layers.Static.Enabled = false
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps:           RuntimeDependencies{Static: static},
	}

	cmd := newValidateCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if static.calls != 0 {
		t.Fatalf("expected static harness not to be called, got %d", static.calls)
	}
	if !strings.Contains(stdout.String(), "- static/disabled: skip") {
		t.Fatalf("expected static disabled skip stage, got:\n%s", stdout.String())
	}
}

func TestValidateCommandFailsWhenSandboxLayerEnabled(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Validation.Layers.SandboxDeploy.Enabled = true
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps:           RuntimeDependencies{Static: &fakeStaticHarness{}},
	}

	cmd := newValidateCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stdout.String(), "- sandbox_deploy/blocked: skip") {
		t.Fatalf("expected sandbox blocked stage, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "check=sandbox_deploy") {
		t.Fatalf("expected sandbox failure detail, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), sandboxRealDeploySkippedMessage) {
		t.Fatalf("expected cost-skip message in output, got:\n%s", stdout.String())
	}
}

func newValidateCommandForTest(opts runtimeOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "validate <scenario>",
		Args: requireScenarioArg,
		RunE: withRuntimeWithOptions("validate", opts, runValidateCommand),
	}
	cmd.Flags().String("config", config.DefaultPath, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	return cmd
}
