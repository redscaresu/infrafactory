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
	"github.com/redscaresu/infrafactory/internal/scenario"
	"github.com/spf13/cobra"
)

type fakeMockDeployHarness struct {
	result  *harness.MockDeployResult
	err     error
	calls   int
	dirs    []string
	lastCtx context.Context
}

func (f *fakeMockDeployHarness) Run(ctx context.Context, workDir string, _ map[string]string) (*harness.MockDeployResult, error) {
	f.calls++
	f.dirs = append(f.dirs, workDir)
	f.lastCtx = ctx
	return f.result, f.err
}

type fakeDestroyHarness struct {
	result  *harness.DestroyResult
	err     error
	calls   int
	lastCtx context.Context
}

func (f *fakeDestroyHarness) Run(ctx context.Context, _ string, _ map[string]string) (*harness.DestroyResult, error) {
	f.calls++
	f.lastCtx = ctx
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
	if !strings.Contains(stdout.String(), sandboxRealDeploySkippedMessage) {
		t.Fatalf("expected sandbox auto-pass message in output, got:\n%s", stdout.String())
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
		sandboxRealDeploySkippedMessage,
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
		"- criteria/support_matrix: skip",
		"- mock_deploy/state_policy: pass",
	} {
		if !strings.Contains(stdout.String(), check) {
			t.Fatalf("expected output to contain %q, got:\n%s", check, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "- mock_deploy/topology") {
		t.Fatalf("topology stage should not appear when connectivity auto-passes, got:\n%s", stdout.String())
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
	if !strings.Contains(stdout.String(), "- mock_deploy/state_policy: fail") {
		t.Fatalf("expected state_policy failure stage, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "policy=encryption_at_rest") {
		t.Fatalf("expected policy failure in output, got:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "- mock_deploy/topology") {
		t.Fatalf("topology stage should not appear when connectivity auto-passes, got:\n%s", stdout.String())
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

func TestEvaluateStatePolicyCriteriaResolvesConstraintPolicyPathAndPassesTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	policiesDir := filepath.Join(root, "policies")
	if err := os.MkdirAll(filepath.Join(policiesDir, "scaleway"), 0o755); err != nil {
		t.Fatalf("mkdir policies dir: %v", err)
	}
	policyPath := filepath.Join(policiesDir, "scaleway", "target.rego")
	policy := `package scaleway.target

import rego.v1

deny_state contains msg if {
	input.target != "database"
	msg := "target mismatch"
}
`
	if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
		t.Fatalf("write policy fixture: %v", err)
	}

	runtime := &CommandRuntime{
		Config: config.Config{
			Paths: config.PathsConfig{Policies: policiesDir},
			ConstraintPolicies: map[string]string{
				"target_policy": filepath.Join("scaleway", "target.rego"),
			},
		},
	}

	specs := []scenario.ExecutableCheckSpec{
		{
			Type:   "policy",
			Expect: "pass",
			Policy: &scenario.PolicyCheckSpec{
				Check:  "target_policy",
				Target: "database",
			},
		},
	}

	failures := evaluateStatePolicyCriteria(context.Background(), runtime, []byte(`{"rdb":{"instances":[]}}`), specs)
	if len(failures) != 0 {
		t.Fatalf("expected no failures, got %+v", failures)
	}
}

func TestAppendDestroyResultAvoidsConflictingPassFailStages(t *testing.T) {
	t.Parallel()

	stages, failures := appendDestroyResult(
		nil,
		nil,
		&harness.DestroyResult{Destroy: harness.StageResult{Stage: "destroy"}},
		&harness.DestroyError{Stage: "state", Err: errors.New("state fetch failed")},
	)
	if len(failures) != 1 {
		t.Fatalf("expected one failure, got %d", len(failures))
	}

	passStateStages := 0
	failStateStages := 0
	for _, stage := range stages {
		if stage.Layer == "destruction" && stage.Stage == "state" {
			if stage.Status == StageStatusPass {
				passStateStages++
			}
			if stage.Status == StageStatusFail {
				failStateStages++
			}
		}
	}
	if passStateStages != 0 || failStateStages != 1 {
		t.Fatalf("expected only one destruction/state fail stage, got stages=%+v", stages)
	}
}

func TestAppendMockDeployResultAvoidsConflictingPassFailStages(t *testing.T) {
	t.Parallel()

	stages, failures := appendMockDeployResult(
		nil,
		nil,
		&harness.MockDeployResult{Apply: harness.StageResult{Stage: "apply"}},
		&harness.MockDeployError{Stage: "state", Err: errors.New("state fetch failed")},
	)
	if len(failures) != 1 {
		t.Fatalf("expected one failure, got %d", len(failures))
	}

	passStateStages := 0
	failStateStages := 0
	for _, stage := range stages {
		if stage.Layer == "mock_deploy" && stage.Stage == "state" {
			if stage.Status == StageStatusPass {
				passStateStages++
			}
			if stage.Status == StageStatusFail {
				failStateStages++
			}
		}
	}
	if passStateStages != 0 || failStateStages != 1 {
		t.Fatalf("expected only one mock_deploy/state fail stage, got stages=%+v", stages)
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

func TestTestCommandPropagatesCommandContext(t *testing.T) {
	t.Parallel()

	type contextKey string
	const key contextKey = "ctx-key"

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
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	commandCtx := context.WithValue(context.Background(), key, "test")
	if err := cmd.ExecuteContext(commandCtx); err != nil {
		t.Fatalf("execute test with context: %v", err)
	}
	if mockDeploy.lastCtx == nil {
		t.Fatal("expected mock deploy context capture")
	}
	if got := mockDeploy.lastCtx.Value(key); got != "test" {
		t.Fatalf("expected propagated context value %q on deploy, got %#v", "test", got)
	}
	if destroy.lastCtx == nil {
		t.Fatal("expected destroy context capture")
	}
	if got := destroy.lastCtx.Value(key); got != "test" {
		t.Fatalf("expected propagated context value %q on destroy, got %#v", "test", got)
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
