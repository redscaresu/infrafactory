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

	appLogPath := filepath.Join(runstoreRoot, "example-scenario", runs[0].RunID, "app.log")
	appLog, err := os.ReadFile(appLogPath)
	if err != nil {
		t.Fatalf("read run app log: %v", err)
	}
	if !strings.Contains(string(appLog), `"event":"terminal_reason"`) {
		t.Fatalf("expected terminal_reason log entry, got:\n%s", string(appLog))
	}
	if !strings.Contains(string(appLog), `"event":"stage_start"`) {
		t.Fatalf("expected stage_start log entry, got:\n%s", string(appLog))
	}
}

func TestRunCommandLLMRawCaptureDisabledByDefault(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)
	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{
					Files: map[string][]byte{"main.tf": []byte("terraform {}\n")},
					Metadata: generator.GenerationMetadata{
						Phases: []generator.PhaseResult{
							{Name: generator.PhaseGenerateHCL, Output: []byte("token: should-not-be-written")},
						},
					},
				}, nil
			}),
			Static:     &fakeStaticHarness{result: &harness.StaticResult{Stages: []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}}, PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`)}},
			MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`)}},
			Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{Destroy: harness.StageResult{Stage: "destroy"}, OrphanCount: 0}},
		},
	}

	cmd := newRunCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run command: %v", err)
	}

	store := runstore.NewFilesystemStore(runstoreRoot)
	runs, err := store.ListRuns("example-scenario")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run, got %+v", runs)
	}
	matches, err := filepath.Glob(filepath.Join(runstoreRoot, "example-scenario", runs[0].RunID, "iterations", "1", "llm_raw_*.json"))
	if err != nil {
		t.Fatalf("glob llm raw artifacts: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no llm raw artifacts by default, got %v", matches)
	}
	promptMatches, err := filepath.Glob(filepath.Join(runstoreRoot, "example-scenario", runs[0].RunID, "iterations", "1", "llm_prompt_*.json"))
	if err != nil {
		t.Fatalf("glob llm prompt artifacts: %v", err)
	}
	if len(promptMatches) != 0 {
		t.Fatalf("expected no llm prompt artifacts by default, got %v", promptMatches)
	}
}

func TestRunCommandLLMRawCaptureWritesPhaseArtifactsWhenEnabled(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)
	t.Setenv(llmRawCaptureEnvVar, "1")
	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{
					Files: map[string][]byte{"main.tf": []byte("terraform {}\n")},
					Metadata: generator.GenerationMetadata{
						Phases: []generator.PhaseResult{
							{Name: generator.PhaseGenerateHCL, Prompt: []byte("feedback token: prompt-should-be-redacted"), Output: []byte("token: should-be-redacted")},
						},
					},
				}, nil
			}),
			Static:     &fakeStaticHarness{result: &harness.StaticResult{Stages: []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}}, PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`)}},
			MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`)}},
			Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{Destroy: harness.StageResult{Stage: "destroy"}, OrphanCount: 0}},
		},
	}

	cmd := newRunCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run command: %v", err)
	}

	store := runstore.NewFilesystemStore(runstoreRoot)
	runs, err := store.ListRuns("example-scenario")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run, got %+v", runs)
	}
	artifactPath := filepath.Join(runstoreRoot, "example-scenario", runs[0].RunID, "iterations", "1", "llm_raw_generate_hcl.json")
	payload, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read llm raw artifact: %v", err)
	}
	var artifact llmRawResponseArtifact
	if err := json.Unmarshal(payload, &artifact); err != nil {
		t.Fatalf("decode llm raw artifact: %v", err)
	}
	if artifact.Phase != generator.PhaseGenerateHCL {
		t.Fatalf("expected phase %q, got %q", generator.PhaseGenerateHCL, artifact.Phase)
	}
	if strings.Contains(artifact.Content, "should-be-redacted") {
		t.Fatalf("expected redacted llm raw artifact content, got %q", artifact.Content)
	}

	promptArtifactPath := filepath.Join(runstoreRoot, "example-scenario", runs[0].RunID, "iterations", "1", "llm_prompt_generate_hcl.json")
	promptPayload, err := os.ReadFile(promptArtifactPath)
	if err != nil {
		t.Fatalf("read llm prompt artifact: %v", err)
	}
	var promptArtifact llmPromptArtifact
	if err := json.Unmarshal(promptPayload, &promptArtifact); err != nil {
		t.Fatalf("decode llm prompt artifact: %v", err)
	}
	if promptArtifact.Phase != generator.PhaseGenerateHCL {
		t.Fatalf("expected prompt phase %q, got %q", generator.PhaseGenerateHCL, promptArtifact.Phase)
	}
	if strings.Contains(promptArtifact.Content, "prompt-should-be-redacted") {
		t.Fatalf("expected redacted llm prompt artifact content, got %q", promptArtifact.Content)
	}
}

func TestRunCommandStopsAfterFirstSuccess(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)

	genCalls := 0
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Agent.RepairIterationsMax = 2
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				genCalls++
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
	if genCalls != 1 {
		t.Fatalf("expected single generation pass on success, got %d", genCalls)
	}
	if strings.Contains(stdout.String(), "iteration_2_") {
		t.Fatalf("expected no second iteration stages in output, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "- run/terminal_reason: pass (target_reached)") {
		t.Fatalf("expected canonical target_reached terminal reason stage, got:\n%s", stdout.String())
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
			cfg.Agent.RepairIterationsMax = 2
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
	if !strings.Contains(stdout.String(), "check=stuck") {
		t.Fatalf("expected stuck failure marker, got:\n%s", stdout.String())
	}

	store := runstore.NewFilesystemStore(runstoreRoot)
	runs, err := store.ListRuns("example-scenario")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "failed" {
		t.Fatalf("expected one failed run, got: %+v", runs)
	}

	iterationPath := filepath.Join(runstoreRoot, "example-scenario", runs[0].RunID, "iterations", "2", "iteration.json")
	iterationPayload, err := os.ReadFile(iterationPath)
	if err != nil {
		t.Fatalf("read iteration artifact: %v", err)
	}
	var artifact struct {
		FailureSummary []string `json:"failure_summary"`
	}
	if err := json.Unmarshal(iterationPayload, &artifact); err != nil {
		t.Fatalf("decode iteration artifact: %v", err)
	}
	if len(artifact.FailureSummary) == 0 {
		t.Fatalf("expected non-empty failure_summary in iteration artifact, got %+v", artifact)
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
			cfg.Agent.RepairIterationsMax = 4
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
	if strings.Contains(stdout.String(), "check=repair_budget_exhausted") {
		t.Fatalf("expected singular terminal reason for stuck stop, got:\n%s", stdout.String())
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
			cfg.Agent.RepairIterationsMax = 3
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
	if !strings.Contains(string(requests[1].FeedbackJSON), `"failure_class":"iac_validation"`) {
		t.Fatalf("expected failure_class tag in feedback payload, got %s", string(requests[1].FeedbackJSON))
	}
	if strings.Contains(string(requests[1].FeedbackJSON), `"check":"stuck"`) {
		t.Fatalf("expected terminal control failures to be excluded from iteration feedback payload, got %s", string(requests[1].FeedbackJSON))
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
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--repair-iterations-max", "1"})

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

func TestRunCommandStopsEarlyOnTransportDominatedFailures(t *testing.T) {
	h := newCommandTestHarness(t)
	runstoreRoot := filepath.Join(h.WorkspaceDir, ".infrafactory", "runs")
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", runstoreRoot)

	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Agent.RepairIterationsMax = 5
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return nil, generator.NewGenerateError(generator.ErrTransportFailed, "generate_hcl", errors.New("transport timeout"))
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
	if !strings.Contains(stdout.String(), "check=transport_runtime_dominated") {
		t.Fatalf("expected transport dominated terminal marker, got:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "iteration_4") {
		t.Fatalf("expected early stop before additional retries, got:\n%s", stdout.String())
	}

	store := runstore.NewFilesystemStore(runstoreRoot)
	runs, err := store.ListRuns("example-scenario")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run, got %+v", runs)
	}
	iterationPath := filepath.Join(runstoreRoot, "example-scenario", runs[0].RunID, "iterations", "2", "iteration.json")
	iterationPayload, err := os.ReadFile(iterationPath)
	if err != nil {
		t.Fatalf("read iteration artifact: %v", err)
	}
	var artifact struct {
		TransportDiagnostics []map[string]string `json:"transport_diagnostics"`
	}
	if err := json.Unmarshal(iterationPayload, &artifact); err != nil {
		t.Fatalf("decode iteration artifact: %v", err)
	}
	if len(artifact.TransportDiagnostics) == 0 {
		t.Fatalf("expected transport diagnostics in artifact, got %+v", artifact)
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
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--repair-iterations-max", "1"})

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
			cfg.Agent.RepairIterationsMax = 3
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
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--repair-iterations-max", "1"})

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
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--repair-iterations-max", "1"})

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
			cfg.Agent.RepairIterationsMax = 3
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
		t.Fatalf("expected holdout evaluation after first successful training pass, got %d generator calls", genCalls)
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

func TestResolveRunControlsUsesConfigDefaultsWhenFlagsUnset(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().Int("repair-iterations-max", 0, "")

	cfg := config.Default()
	cfg.Agent.RepairIterationsMax = 7

	controls, err := resolveRunControls(cmd, &CommandRuntime{Config: cfg})
	if err != nil {
		t.Fatalf("resolve controls: %v", err)
	}
	if controls.RepairIterationsMax != 7 {
		t.Fatalf("expected repair max 7, got %d", controls.RepairIterationsMax)
	}
}

func TestResolveRunControlsUsesFlagOverridesWhenProvided(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().Int("repair-iterations-max", 0, "")
	if err := cmd.Flags().Set("repair-iterations-max", "2"); err != nil {
		t.Fatalf("set flag: %v", err)
	}

	cfg := config.Default()
	cfg.Agent.RepairIterationsMax = 8

	controls, err := resolveRunControls(cmd, &CommandRuntime{Config: cfg})
	if err != nil {
		t.Fatalf("resolve controls: %v", err)
	}
	if controls.RepairIterationsMax != 2 {
		t.Fatalf("expected repair max override 2, got %d", controls.RepairIterationsMax)
	}
}

func TestResolveRunControlsRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		configureCmd func(*cobra.Command)
		cfg          config.Config
		expected     string
	}{
		{
			name:         "invalid repair max from config",
			configureCmd: func(_ *cobra.Command) {},
			cfg: func() config.Config {
				cfg := config.Default()
				cfg.Agent.RepairIterationsMax = 0
				return cfg
			}(),
			expected: "repair iterations max must be >= 1",
		},
		{
			name: "invalid repair max from flag",
			configureCmd: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("repair-iterations-max", "-1")
			},
			cfg:      config.Default(),
			expected: "repair iterations max must be >= 1",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cmd := &cobra.Command{Use: "run"}
			cmd.Flags().Int("repair-iterations-max", 0, "")
			tc.configureCmd(cmd)

			_, err := resolveRunControls(cmd, &CommandRuntime{Config: tc.cfg})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.expected) {
				t.Fatalf("expected error to contain %q, got %v", tc.expected, err)
			}
		})
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
	cmd.Flags().Int("repair-iterations-max", 0, "")
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
