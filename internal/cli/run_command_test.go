package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/runstore"
	"github.com/spf13/cobra"
)

func TestRunCommandConvergesOnFirstIteration(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)
	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
			}),
			Static:     &fakeStaticHarness{result: &harness.StaticResult{Stages: []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}}, PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`)}},
			MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`)}},
			Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{Destroy: harness.StageResult{Stage: "destroy"}, OrphanCount: 0}},
		},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run command: %v", err)
	}
	checks := []string{
		"Command: run",
		"Status: success",
		"- run/iteration_1_generate: pass",
		"- run/iteration_1_validate: pass",
		"- run/iteration_1_test: pass",
	}
	for _, check := range checks {
		if !strings.Contains(stdout.String(), check) {
			t.Fatalf("expected output to contain %q, got:\n%s", check, stdout.String())
		}
	}

	store := runstore.NewFilesystemStore(runstoreRoot)
	runs, err := store.ListRuns("example-scenario")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "success" {
		t.Fatalf("expected one successful run, got: %+v", runs)
	}
	iterationPath := filepath.Join(runstoreRoot, "example-scenario", runs[0].RunID, "iterations", "1", "iteration.json")
	if _, err := os.Stat(iterationPath); err != nil {
		t.Fatalf("expected iteration artifact: %v", err)
	}
	iterationPayload, err := os.ReadFile(iterationPath)
	if err != nil {
		t.Fatalf("read iteration artifact: %v", err)
	}
	var iteration struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(iterationPayload, &iteration); err != nil {
		t.Fatalf("decode iteration artifact: %v", err)
	}
	if iteration.Schema != runstore.RunIterationSchemaVersion {
		t.Fatalf("expected iteration schema %q, got %q", runstore.RunIterationSchemaVersion, iteration.Schema)
	}

	runPath := filepath.Join(runstoreRoot, "example-scenario", runs[0].RunID, "run.json")
	runPayload, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatalf("read run metadata artifact: %v", err)
	}
	var runMeta struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(runPayload, &runMeta); err != nil {
		t.Fatalf("decode run metadata artifact: %v", err)
	}
	if runMeta.Schema != runstore.RunMetadataSchemaVersion {
		t.Fatalf("expected run metadata schema %q, got %q", runstore.RunMetadataSchemaVersion, runMeta.Schema)
	}
}

func TestRunCommandStopsAtMaxIterations(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)
	iter := 0
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Agent.MaxIterations = 2
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				iter++
				if iter == 1 {
					return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
				}
				return nil, errors.New("seed failed on second iteration")
			}),
			Static:     &fakeStaticHarness{err: &harness.StageError{StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}}, Err: errors.New("validate failed")}},
			MockDeploy: &fakeMockDeployHarness{},
			Destroy:    &fakeDestroyHarness{},
		},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run failure")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T (%v)", err, err)
	}
	if cliErr.Op != "run" || cliErr.Code != errorCodeCommandFailed {
		t.Fatalf("expected run/%s CLI error, got op=%q code=%q", errorCodeCommandFailed, cliErr.Op, cliErr.Code)
	}
	if !strings.Contains(stdout.String(), "check=max_iterations") {
		t.Fatalf("expected max-iterations failure marker, got:\n%s", stdout.String())
	}

	store := runstore.NewFilesystemStore(runstoreRoot)
	runs, err := store.ListRuns("example-scenario")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "failed" {
		t.Fatalf("expected one failed run, got: %+v", runs)
	}
}

func TestRunCommandStopsOnStuckDetection(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Agent.MaxIterations = 4
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return nil, errors.New("seed failed")
			}),
			Static:     &fakeStaticHarness{},
			MockDeploy: &fakeMockDeployHarness{},
			Destroy:    &fakeDestroyHarness{},
		},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run failure")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T (%v)", err, err)
	}
	if cliErr.Op != "run" || cliErr.Code != errorCodeCommandFailed {
		t.Fatalf("expected run/%s CLI error, got op=%q code=%q", errorCodeCommandFailed, cliErr.Op, cliErr.Code)
	}
	if !strings.Contains(stdout.String(), "check=stuck") {
		t.Fatalf("expected stuck-detection failure marker, got:\n%s", stdout.String())
	}

	store := runstore.NewFilesystemStore(runstoreRoot)
	runs, err := store.ListRuns("example-scenario")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "failed" {
		t.Fatalf("expected one failed run, got: %+v", runs)
	}
}

func TestRunCommandPassesPreviousIterationFailuresAsGenerateFeedback(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)

	requests := make([]generator.Request, 0, 2)
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Agent.MaxIterations = 3
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(_ context.Context, req generator.Request) (*generator.GeneratedCode, error) {
				requests = append(requests, req)
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
			}),
			Static: &fakeStaticHarness{
				err: &harness.StageError{
					StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}},
					Err:         errors.New("validate failed"),
				},
			},
			MockDeploy: &fakeMockDeployHarness{},
			Destroy:    &fakeDestroyHarness{},
		},
	}

	cmd := newRunCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected run failure")
	}
	if len(requests) < 2 {
		t.Fatalf("expected at least two generate calls, got %d", len(requests))
	}
	if requests[0].Iteration != 1 {
		t.Fatalf("expected first iteration=1, got %d", requests[0].Iteration)
	}
	if len(requests[0].FeedbackJSON) != 0 {
		t.Fatalf("expected no feedback on first iteration, got %s", string(requests[0].FeedbackJSON))
	}
	if requests[1].Iteration != 2 {
		t.Fatalf("expected second iteration=2, got %d", requests[1].Iteration)
	}
	if len(requests[1].FeedbackJSON) == 0 {
		t.Fatal("expected feedback payload on second iteration")
	}
	if !strings.Contains(string(requests[1].FeedbackJSON), `"check":"validate"`) {
		t.Fatalf("expected validate failure in feedback payload, got %s", string(requests[1].FeedbackJSON))
	}
}

func TestRunCommandDefaultRuntimeUsesConcreteGeneratorDependency(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)
	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--max-iterations", "1"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run failure")
	}
	if strings.Contains(stdout.String(), "generator dependency unavailable") {
		t.Fatalf("expected concrete generator failure path, got:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "default seed generator for agent type") {
		t.Fatalf("expected concrete adapter path (not stub), got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "prompt render failed") {
		t.Fatalf("expected concrete adapter failure detail, got:\n%s", stdout.String())
	}
}

func TestRunCommandFailsWhenSandboxLayerEnabled(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)
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
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
			}),
		},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run failure")
	}
	if !strings.Contains(stdout.String(), "- sandbox_deploy/blocked: skip") {
		t.Fatalf("expected sandbox blocked stage, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), sandboxRealDeploySkippedMessage) {
		t.Fatalf("expected cost-skip message in output, got:\n%s", stdout.String())
	}
}

func TestRunCommandHonorsDisabledStaticMockAndDestructionLayers(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Validation.Layers.Static.Enabled = false
			cfg.Validation.Layers.MockDeploy.Enabled = false
			cfg.Validation.Layers.Destruction.Enabled = false
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
			}),
		},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--max-iterations", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected run success with disabled layers, got: %v", err)
	}
	for _, check := range []string{
		"Status: success",
		"- run/iteration_1_generate: pass",
		"- run/iteration_1_validate: pass",
		"- run/iteration_1_test: pass",
	} {
		if !strings.Contains(stdout.String(), check) {
			t.Fatalf("expected output to contain %q, got:\n%s", check, stdout.String())
		}
	}
}

func TestRunCommandPropagatesCriteriaFailuresForConvergence(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)
	scenarioPath := writeCriteriaScenario(t, h.WorkspaceDir, "success", "pass")

	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Agent.MaxIterations = 3
			cfg.Validation.Layers.Destruction.Enabled = false
			cfg.ConstraintPolicies = map[string]string{
				"encryption_at_rest": filepath.Join("..", "harness", "testdata", "state-policy", "policy.rego"),
			}
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
			}),
			Static: &fakeStaticHarness{
				result: &harness.StaticResult{
					Stages:   []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}},
					PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`),
				},
			},
			MockDeploy: &fakeMockDeployHarness{
				result: &harness.MockDeployResult{
					Apply: harness.StageResult{Stage: "apply"},
					StateSnapshot: []byte(`{
  "connectivity": {"compute->database:5432": false},
  "http_probe": {"load_balancer:80": true},
  "rdb": {"public_endpoint": false}
}`),
				},
			},
			Destroy: &fakeDestroyHarness{},
		},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run failure")
	}
	if !strings.Contains(stdout.String(), "check=connectivity") {
		t.Fatalf("expected criteria-level connectivity failure in run output, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "check=stuck") {
		t.Fatalf("expected stuck detection marker in run output, got:\n%s", stdout.String())
	}
}

func TestRunCommandExecutesCriteriaOnlyHoldoutsAfterConvergence(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)
	writeCriteriaOnlyHoldout(t, filepath.Join(h.WorkspaceDir, "scenarios", "holdout", "holdout-pass.yaml"), h.ScenarioPath, `  - type: destruction
    expect: no_orphans
`, "holdout-pass")

	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Paths.Scenarios = filepath.Join(h.WorkspaceDir, "scenarios")
			cfg.Validation.Layers.Static.Enabled = false
			cfg.Validation.Layers.MockDeploy.Enabled = false
			cfg.Validation.Layers.Destruction.Enabled = false
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
			}),
			MockDeploy: &fakeMockDeployHarness{},
			Destroy:    &fakeDestroyHarness{},
		},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--max-iterations", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected run success, got: %v", err)
	}
	for _, check := range []string{
		"- holdout/discovery: pass (1 holdouts)",
		"- holdout/holdout-pass: pass",
	} {
		if !strings.Contains(stdout.String(), check) {
			t.Fatalf("expected holdout stage %q, got:\n%s", check, stdout.String())
		}
	}
}

func TestRunCommandCriteriaOnlyHoldoutsReuseTrainingOutputDir(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)
	writeCriteriaOnlyHoldout(t, filepath.Join(h.WorkspaceDir, "scenarios", "holdout", "holdout-pass.yaml"), h.ScenarioPath, `  - type: destruction
    expect: no_orphans
`, "holdout-pass")

	mockDeploy := &fakeMockDeployHarness{
		result: &harness.MockDeployResult{
			Apply:         harness.StageResult{Stage: "apply"},
			StateSnapshot: []byte(`{}`),
		},
	}
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Paths.Scenarios = filepath.Join(h.WorkspaceDir, "scenarios")
			cfg.Validation.Layers.Static.Enabled = false
			cfg.Validation.Layers.MockDeploy.Enabled = true
			cfg.Validation.Layers.Destruction.Enabled = false
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
			}),
			MockDeploy: mockDeploy,
			Destroy:    &fakeDestroyHarness{},
		},
	}

	cmd := newRunCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--max-iterations", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected run success, got: %v", err)
	}
	if len(mockDeploy.dirs) != 2 {
		t.Fatalf("expected two mock deploy calls (training + holdout), got %d", len(mockDeploy.dirs))
	}
	expected := filepath.Join("output", "example-scenario")
	if mockDeploy.dirs[0] != expected || mockDeploy.dirs[1] != expected {
		t.Fatalf("expected both mock deploy calls to use %q, got %#v", expected, mockDeploy.dirs)
	}
}

func TestRunCommandAutoPassesDeferredDNSHoldoutWithoutFeedbackInjection(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)
	writeCriteriaOnlyHoldout(t, filepath.Join(h.WorkspaceDir, "scenarios", "holdout", "holdout-block.yaml"), h.ScenarioPath, `  - type: dns_resolution
    domain: "{{scenario_name}}.example.com"
    expect: resolves
`, "holdout-block")

	genCalls := 0
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Paths.Scenarios = filepath.Join(h.WorkspaceDir, "scenarios")
			cfg.Validation.Layers.Static.Enabled = false
			cfg.Validation.Layers.MockDeploy.Enabled = false
			cfg.Validation.Layers.Destruction.Enabled = false
			cfg.Agent.MaxIterations = 3
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				genCalls++
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
			}),
			MockDeploy: &fakeMockDeployHarness{},
			Destroy:    &fakeDestroyHarness{},
		},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected run success, got: %v", err)
	}
	if genCalls != 1 {
		t.Fatalf("expected exactly one training iteration before holdout evaluation, got %d generator calls", genCalls)
	}
	if !strings.Contains(stdout.String(), dnsResolutionAutoPassMessage()) {
		t.Fatalf("expected holdout dns_resolution auto-pass message, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Status: success") {
		t.Fatalf("expected successful run status, got:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "Status: failed") {
		t.Fatalf("expected successful run status, got:\n%s", stdout.String())
	}
}

func newRunCommandForTest(opts runtimeOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "run <scenario>",
		Args: requireScenarioArg,
		RunE: withRuntimeWithOptions("run", opts, runRunCommand),
	}
	cmd.Flags().String("config", config.DefaultPath, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	cmd.Flags().Int("max-iterations", 0, "")
	return cmd
}

func writeCriteriaOnlyHoldout(t *testing.T, path string, trainingPath string, criteria string, scenarioName string) {
	t.Helper()

	content := `scenario: ` + scenarioName + `
version: "1.0"
cloud: scaleway
description: criteria-only holdout fixture
type: holdout
references: ` + trainingPath + `
acceptance_criteria:
` + criteria
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir holdout dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write holdout fixture: %v", err)
	}
}
