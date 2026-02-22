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

type fakeMockDeployHarness struct {
	result *harness.MockDeployResult
	err    error
	calls  int
}

func (f *fakeMockDeployHarness) Run(context.Context, string, map[string]string) (*harness.MockDeployResult, error) {
	f.calls++
	return f.result, f.err
}

type fakeDestroyHarness struct {
	result *harness.DestroyResult
	err    error
	calls  int
}

func (f *fakeDestroyHarness) Run(context.Context, string, map[string]string) (*harness.DestroyResult, error) {
	f.calls++
	return f.result, f.err
}

func TestTestCommandSuccess(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	mockDeploy := &fakeMockDeployHarness{
		result: &harness.MockDeployResult{
			Apply:         harness.StageResult{Stage: "apply"},
			StateSnapshot: []byte(`{"mock":true}`),
		},
	}
	destroy := &fakeDestroyHarness{
		result: &harness.DestroyResult{
			Destroy:       harness.StageResult{Stage: "destroy"},
			StateSnapshot: []byte(`{"mock":true}`),
			OrphanCount:   0,
		},
	}

	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockDeploy: mockDeploy,
			Destroy:    destroy,
		},
	}

	cmd := newTestCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute test command: %v", err)
	}
	if mockDeploy.calls != 1 || destroy.calls != 1 {
		t.Fatalf("expected one deploy and one destroy call, got deploy=%d destroy=%d", mockDeploy.calls, destroy.calls)
	}
	if !strings.Contains(stdout.String(), "Status: success") {
		t.Fatalf("expected success output, got:\n%s", stdout.String())
	}
}

func TestTestCommandMockDeployFailure(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	mockDeploy := &fakeMockDeployHarness{
		err: &harness.MockDeployError{
			Stage: "apply",
			Apply: harness.StageResult{Stage: "apply", Cmd: []string{"tofu", "apply"}},
			Err:   errors.New("apply failed"),
		},
	}
	destroy := &fakeDestroyHarness{}

	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockDeploy: mockDeploy,
			Destroy:    destroy,
		},
	}

	cmd := newTestCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected failure")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T (%v)", err, err)
	}
	if cliErr.Op != "test" || cliErr.Code != errorCodeCommandFailed {
		t.Fatalf("expected test/%s CLI error, got op=%q code=%q", errorCodeCommandFailed, cliErr.Op, cliErr.Code)
	}
	if destroy.calls != 0 {
		t.Fatalf("expected destroy not to run after deploy failure, got %d", destroy.calls)
	}
	if !strings.Contains(stdout.String(), "- mock_deploy/apply: fail") {
		t.Fatalf("expected apply failure stage in output, got:\n%s", stdout.String())
	}
	if ExitCodeForError(err) != ExitCodeRuntime {
		t.Fatalf("expected runtime exit code, got %d", ExitCodeForError(err))
	}
}

func TestTestCommandDestroyFailure(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	mockDeploy := &fakeMockDeployHarness{
		result: &harness.MockDeployResult{
			Apply:         harness.StageResult{Stage: "apply"},
			StateSnapshot: []byte(`{"mock":true}`),
		},
	}
	destroy := &fakeDestroyHarness{
		err: &harness.DestroyError{
			Stage:   "orphan_check",
			Destroy: harness.StageResult{Stage: "destroy", Cmd: []string{"tofu", "destroy"}},
			Err:     errors.New("detected 1 orphaned resources"),
		},
	}

	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockDeploy: mockDeploy,
			Destroy:    destroy,
		},
	}

	cmd := newTestCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected failure")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T (%v)", err, err)
	}
	if cliErr.Op != "test" || cliErr.Code != errorCodeCommandFailed {
		t.Fatalf("expected test/%s CLI error, got op=%q code=%q", errorCodeCommandFailed, cliErr.Op, cliErr.Code)
	}
	if !strings.Contains(stdout.String(), "- destruction/orphan_check: fail") {
		t.Fatalf("expected orphan check failure stage in output, got:\n%s", stdout.String())
	}
}

func TestTestCommandAutoPassesDeferredDNSCriteriaHumanOutput(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	scenarioPath := writeUnsupportedCriteriaScenario(t, h.WorkspaceDir)

	mockDeploy := &fakeMockDeployHarness{}
	destroy := &fakeDestroyHarness{}
	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockDeploy: mockDeploy,
			Destroy:    destroy,
		},
	}

	cmd := newTestCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if mockDeploy.calls != 1 || destroy.calls != 1 {
		t.Fatalf("expected deploy/destroy calls for auto-pass criteria path, got deploy=%d destroy=%d", mockDeploy.calls, destroy.calls)
	}
	if !strings.Contains(stdout.String(), "- criteria/support_matrix: skip") {
		t.Fatalf("expected support-matrix skip stage in output, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), dnsResolutionAutoPassMessage()) {
		t.Fatalf("expected dns_resolution auto-pass message in output, got:\n%s", stdout.String())
	}
}

func TestTestCommandAutoPassesDeferredDNSCriteriaJSONOutput(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	scenarioPath := writeUnsupportedCriteriaScenario(t, h.WorkspaceDir)

	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockDeploy: &fakeMockDeployHarness{},
			Destroy:    &fakeDestroyHarness{},
		},
	}

	cmd := newTestCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath, "--output", string(OutputModeJSON)})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	checks := []string{
		`"schema": "` + OutputSchemaVersion + `"`,
		`"status": "success"`,
		`"stage": "support_matrix"`,
		`currently automatically passes due to lack of real world cloud provider`,
	}
	for _, check := range checks {
		if !strings.Contains(stdout.String(), check) {
			t.Fatalf("expected output to contain %q, got:\n%s", check, stdout.String())
		}
	}
}

func TestTestCommandExecutesCriteriaDrivenTopologyAndPolicyChecks(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	scenarioPath := writeCriteriaScenario(t, h.WorkspaceDir, "success", "pass")
	mockDeploy := &fakeMockDeployHarness{
		result: &harness.MockDeployResult{
			Apply: harness.StageResult{Stage: "apply"},
			StateSnapshot: []byte(`{
  "connectivity": {"compute->database:5432": true},
  "http_probe": {"load_balancer:80": true},
  "rdb": {"public_endpoint": false}
}`),
		},
	}
	destroy := &fakeDestroyHarness{
		result: &harness.DestroyResult{
			Destroy:       harness.StageResult{Stage: "destroy"},
			StateSnapshot: []byte(`{}`),
			OrphanCount:   0,
		},
	}
	policyPath := filepath.Join("..", "harness", "testdata", "state-policy", "policy.rego")
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.ConstraintPolicies = map[string]string{
				"encryption_at_rest": policyPath,
			}
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockDeploy: mockDeploy,
			Destroy:    destroy,
		},
	}

	cmd := newTestCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	for _, check := range []string{
		"- mock_deploy/topology: pass",
		"- mock_deploy/state_policy: pass",
	} {
		if !strings.Contains(stdout.String(), check) {
			t.Fatalf("expected output to contain %q, got:\n%s", check, stdout.String())
		}
	}
}

func TestTestCommandReportsCriteriaDrivenTopologyAndPolicyFailures(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	scenarioPath := writeCriteriaScenario(t, h.WorkspaceDir, "success", "pass")
	mockDeploy := &fakeMockDeployHarness{
		result: &harness.MockDeployResult{
			Apply: harness.StageResult{Stage: "apply"},
			StateSnapshot: []byte(`{
  "connectivity": {"compute->database:5432": false},
  "http_probe": {"load_balancer:80": true},
  "rdb": {"public_endpoint": true}
}`),
		},
	}
	destroy := &fakeDestroyHarness{
		result: &harness.DestroyResult{
			Destroy:       harness.StageResult{Stage: "destroy"},
			StateSnapshot: []byte(`{}`),
			OrphanCount:   0,
		},
	}
	policyPath := filepath.Join("..", "harness", "testdata", "state-policy", "policy.rego")
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.ConstraintPolicies = map[string]string{
				"encryption_at_rest": policyPath,
			}
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockDeploy: mockDeploy,
			Destroy:    destroy,
		},
	}

	cmd := newTestCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stdout.String(), "- mock_deploy/topology: fail") {
		t.Fatalf("expected topology failure stage, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "- mock_deploy/state_policy: fail") {
		t.Fatalf("expected state_policy failure stage, got:\n%s", stdout.String())
	}
	for _, check := range []string{
		"check=connectivity",
		"policy=encryption_at_rest",
	} {
		if !strings.Contains(stdout.String(), check) {
			t.Fatalf("expected output to contain %q, got:\n%s", check, stdout.String())
		}
	}
}

func TestTestCommandSkipsWhenMockLayerDisabled(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	mockDeploy := &fakeMockDeployHarness{}
	destroy := &fakeDestroyHarness{}
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Validation.Layers.MockDeploy.Enabled = false
			cfg.Validation.Layers.Destruction.Enabled = true
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockDeploy: mockDeploy,
			Destroy:    destroy,
		},
	}

	cmd := newTestCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if mockDeploy.calls != 0 || destroy.calls != 0 {
		t.Fatalf("expected no deploy/destroy calls, got deploy=%d destroy=%d", mockDeploy.calls, destroy.calls)
	}
	if !strings.Contains(stdout.String(), "- mock_deploy/disabled: skip") {
		t.Fatalf("expected mock disabled stage, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "- destruction/blocked: skip") {
		t.Fatalf("expected destruction blocked stage, got:\n%s", stdout.String())
	}
}

func TestTestCommandSkipsDestructionWhenLayerDisabled(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	mockDeploy := &fakeMockDeployHarness{
		result: &harness.MockDeployResult{
			Apply:         harness.StageResult{Stage: "apply"},
			StateSnapshot: []byte(`{}`),
		},
	}
	destroy := &fakeDestroyHarness{}
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Validation.Layers.Destruction.Enabled = false
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockDeploy: mockDeploy,
			Destroy:    destroy,
		},
	}

	cmd := newTestCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if destroy.calls != 0 {
		t.Fatalf("expected destroy not called, got %d", destroy.calls)
	}
	if !strings.Contains(stdout.String(), "- destruction/disabled: skip") {
		t.Fatalf("expected destruction disabled stage, got:\n%s", stdout.String())
	}
}

func TestTestCommandFailsWhenSandboxLayerEnabled(t *testing.T) {
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
		deps: RuntimeDependencies{
			MockDeploy: &fakeMockDeployHarness{},
			Destroy:    &fakeDestroyHarness{},
		},
	}

	cmd := newTestCommandForTest(opts)
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
	if !strings.Contains(stdout.String(), sandboxRealDeploySkippedMessage) {
		t.Fatalf("expected cost-skip message in output, got:\n%s", stdout.String())
	}
}

func newTestCommandForTest(opts runtimeOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "test <scenario>",
		Args: requireScenarioArg,
		RunE: withRuntimeWithOptions("test", opts, runTestCommand),
	}
	cmd.Flags().String("config", config.DefaultPath, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	return cmd
}

func writeUnsupportedCriteriaScenario(t *testing.T, workspace string) string {
	t.Helper()

	path := filepath.Join(workspace, "scenarios", "training", "unsupported-dns.yaml")
	content := `scenario: unsupported-dns
version: "1.0"
cloud: scaleway
description: unsupported dns criterion fixture
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: dns_resolution
    domain: "{{scenario_name}}.example.com"
    expect: resolves
`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir scenario dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write scenario fixture: %v", err)
	}
	return path
}

func writeCriteriaScenario(t *testing.T, workspace string, connectivityExpect string, policyExpect string) string {
	t.Helper()

	path := filepath.Join(workspace, "scenarios", "training", "criteria-eval.yaml")
	content := `scenario: criteria-eval
version: "1.0"
cloud: scaleway
description: criteria-evaluator fixture
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: connectivity
    from: compute
    to: database
    port: 5432
    expect: ` + connectivityExpect + `
  - type: policy
    check: encryption_at_rest
    target: database
    expect: ` + policyExpect + `
  - type: destruction
    expect: no_orphans
`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir scenario dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write scenario fixture: %v", err)
	}
	return path
}
