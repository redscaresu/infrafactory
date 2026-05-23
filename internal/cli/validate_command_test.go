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
	lastCtx context.Context
}

func (f *fakeStaticHarness) Run(ctx context.Context, workDir string, env map[string]string) (*harness.StaticResult, error) {
	f.calls++
	f.lastDir = workDir
	f.lastEnv = env
	f.lastCtx = ctx
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
	if static.lastEnv["SCW_ACCESS_KEY"] != "SCWMOCKACCESSKEY0000" {
		t.Fatalf("unexpected SCW_ACCESS_KEY: %q", static.lastEnv["SCW_ACCESS_KEY"])
	}
	if static.lastEnv["SCW_SECRET_KEY"] != "00000000-0000-0000-0000-000000000000" {
		t.Fatalf("unexpected SCW_SECRET_KEY: %q", static.lastEnv["SCW_SECRET_KEY"])
	}
	if static.lastEnv["SCW_DEFAULT_PROJECT_ID"] != "00000000-0000-0000-0000-000000000000" {
		t.Fatalf("unexpected SCW_DEFAULT_PROJECT_ID: %q", static.lastEnv["SCW_DEFAULT_PROJECT_ID"])
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

func TestValidateCommandStillRunsStaticWhenSandboxLayerEnabled(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	static := &fakeStaticHarness{
		result: &harness.StaticResult{
			Stages:   []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}},
			PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`),
		},
	}
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
		deps:           RuntimeDependencies{Static: static},
	}

	cmd := newValidateCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if static.calls != 1 {
		t.Fatalf("expected static harness call, got %d", static.calls)
	}
	if !strings.Contains(stdout.String(), "- static/init: pass") {
		t.Fatalf("expected static stages in output, got:\n%s", stdout.String())
	}
}

func TestValidateCommandPropagatesCommandContext(t *testing.T) {
	t.Parallel()

	type contextKey string
	const key contextKey = "ctx-key"

	h := newCommandTestHarness(t)
	static := &fakeStaticHarness{
		result: &harness.StaticResult{
			Stages:   []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}},
			PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`),
		},
	}

	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Validation.Layers.Static.PolicyPaths = nil
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps:           RuntimeDependencies{Static: static},
	}

	cmd := newValidateCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	commandCtx := context.WithValue(context.Background(), key, "validate")
	if err := cmd.ExecuteContext(commandCtx); err != nil {
		t.Fatalf("execute validate with context: %v", err)
	}
	if static.lastCtx == nil {
		t.Fatal("expected static harness context capture")
	}
	if got := static.lastCtx.Value(key); got != "validate" {
		t.Fatalf("expected propagated context value %q, got %#v", "validate", got)
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

func TestResolvePolicyPathsResolvesRelativeToPoliciesDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	policiesDir := filepath.Join(root, "policies")
	if err := os.MkdirAll(policiesDir, 0o755); err != nil {
		t.Fatalf("mkdir policies dir: %v", err)
	}

	absPolicy := filepath.Join(root, "abs.rego")
	if err := os.WriteFile(absPolicy, []byte("package test.abs"), 0o644); err != nil {
		t.Fatalf("write abs policy: %v", err)
	}

	relativeExisting := filepath.Join(policiesDir, "existing.rego")
	if err := os.WriteFile(relativeExisting, []byte("package test.existing"), 0o644); err != nil {
		t.Fatalf("write existing policy: %v", err)
	}

	resolved := resolvePolicyPaths(policiesDir, []string{
		absPolicy,
		relativeExisting,
		"scaleway/region_restriction.rego",
		"",
	})
	if len(resolved) != 3 {
		t.Fatalf("expected 3 resolved paths, got %d: %#v", len(resolved), resolved)
	}
	if resolved[0] != absPolicy {
		t.Fatalf("expected absolute path passthrough, got %q", resolved[0])
	}
	if resolved[1] != relativeExisting {
		t.Fatalf("expected existing relative path passthrough, got %q", resolved[1])
	}
	expectedJoined := filepath.Join(policiesDir, "scaleway/region_restriction.rego")
	if resolved[2] != expectedJoined {
		t.Fatalf("expected joined path %q, got %q", expectedJoined, resolved[2])
	}
}

// TestFilterPolicyPathsByCloudDropsOtherCloudSubdirs proves an
// aws-cloud scenario filters out ./policies/scaleway and ./policies/gcp
// while keeping common/, custom/, and the cloud's own dir — so the
// aws plan doesn't have unrelated cloud regos vacuously firing
// against it.
func TestFilterPolicyPathsByCloudDropsOtherCloudSubdirs(t *testing.T) {
	t.Parallel()

	paths := []string{
		"./policies/common",
		"./policies/scaleway",
		"./policies/gcp",
		"./policies/aws",
		"./policies/custom",
	}

	got := filterPolicyPathsByCloud(paths, "aws")
	want := []string{
		"./policies/common",
		"./policies/aws",
		"./policies/custom",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d paths after aws filter, got %d: %v", len(want), len(got), got)
	}
	for i, p := range want {
		if got[i] != p {
			t.Fatalf("idx %d: expected %q, got %q (full: %v)", i, p, got[i], got)
		}
	}
}

// TestFilterPolicyPathsByCloudEmptyCloudIsPassthrough preserves
// pre-multi-cloud behavior: scenarios that don't declare a cloud get
// every policy path unchanged.
func TestFilterPolicyPathsByCloudEmptyCloudIsPassthrough(t *testing.T) {
	t.Parallel()

	paths := []string{"./policies/common", "./policies/scaleway", "./policies/gcp"}
	got := filterPolicyPathsByCloud(paths, "")
	if len(got) != len(paths) {
		t.Fatalf("expected passthrough on empty cloud, got %v", got)
	}
}

// TestFilterPolicyPathsByCloudHandlesAbsolutePaths confirms the base-
// name match works against absolute paths (production paths arrive
// post-resolvePolicyPaths so they're typically absolute).
func TestFilterPolicyPathsByCloudHandlesAbsolutePaths(t *testing.T) {
	t.Parallel()

	paths := []string{
		"/etc/infrafactory/policies/common",
		"/etc/infrafactory/policies/scaleway",
		"/etc/infrafactory/policies/aws",
	}
	got := filterPolicyPathsByCloud(paths, "aws")
	if len(got) != 2 {
		t.Fatalf("expected 2 paths after filter, got %v", got)
	}
	for _, p := range got {
		if filepath.Base(p) == "scaleway" {
			t.Fatalf("scaleway dir should be filtered out for cloud=aws, got %v", got)
		}
	}
}
