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
	"time"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/runstore"
	"github.com/spf13/cobra"
)

func TestRunCommandConvergesOnFirstIteration(t *testing.T) {
	h := newCommandTestHarness(t)
	opts := isolatedRunOpts(h, nil)
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static:     &fakeStaticHarness{result: &harness.StaticResult{Stages: []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}}, PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`)}},
		MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`)}},
		Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{Destroy: harness.StageResult{Stage: "destroy"}, OrphanCount: 0}},
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

	store := runstore.NewFilesystemStore(h.RunstoreRoot())
	runs, err := store.ListRuns("example-scenario")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "success" {
		t.Fatalf("expected one successful run, got: %+v", runs)
	}
	iterationPath := filepath.Join(h.RunstoreRoot(), "example-scenario", runs[0].RunID, "iterations", "1", "iteration.json")
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

	runPath := filepath.Join(h.RunstoreRoot(), "example-scenario", runs[0].RunID, "run.json")
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

	appLogPath := filepath.Join(h.RunstoreRoot(), "example-scenario", runs[0].RunID, "app.log")
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

	generatedPath := filepath.Join(h.RunstoreRoot(), "example-scenario", runs[0].RunID, "generated", "main.tf")
	generatedPayload, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("read generated run file: %v", err)
	}
	if !strings.Contains(string(generatedPayload), "terraform {}") {
		t.Fatalf("unexpected generated run file: %s", string(generatedPayload))
	}

	iterationGeneratedPath := filepath.Join(h.RunstoreRoot(), "example-scenario", runs[0].RunID, "iterations", "1", "generated", "main.tf")
	iterationGeneratedPayload, err := os.ReadFile(iterationGeneratedPath)
	if err != nil {
		t.Fatalf("read iteration generated run file: %v", err)
	}
	if !strings.Contains(string(iterationGeneratedPayload), "terraform {}") {
		t.Fatalf("unexpected iteration generated run file: %s", string(iterationGeneratedPayload))
	}
}

func TestRunCommandPersistsRunningMetadataBeforeCompletion(t *testing.T) {
	h := newCommandTestHarness(t)

	block := make(chan struct{})
	opts := isolatedRunOpts(h, nil)
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			<-block
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static:     &fakeStaticHarness{result: &harness.StaticResult{Stages: []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}}, PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`)}},
		MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`)}},
		Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{Destroy: harness.StageResult{Stage: "destroy"}, OrphanCount: 0}},
	}

	cmd := newRunCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	store := runstore.NewFilesystemStore(h.RunstoreRoot())
	var runningMeta runstore.RunMetadata
	found := false
	for i := 0; i < 100; i++ {
		runs, err := store.ListRuns("example-scenario")
		if err == nil && len(runs) == 1 {
			runningMeta = runs[0]
			found = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !found {
		t.Fatal("expected running metadata to be persisted before completion")
	}
	if runningMeta.Status != "running" {
		t.Fatalf("expected running status before completion, got %+v", runningMeta)
	}

	close(block)
	if err := <-done; err != nil {
		t.Fatalf("execute run command: %v", err)
	}
}

func TestRunCommandPersistsPlanAndBaselineArtifacts(t *testing.T) {
	h := newCommandTestHarness(t)

	mockState := &fakeRunMockStateClient{statePayload: []byte(`{"instance":{"servers":[{"id":"srv-1"}]}}`)}
	opts := isolatedRunOpts(h, nil)
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static: &fakeStaticHarness{result: &harness.StaticResult{
			Stages: []harness.StageResult{
				{Stage: "init"},
				{Stage: "validate"},
				{Stage: "plan", Stdout: "Plan: 1 to add, 0 to change, 0 to destroy.\n"},
				{Stage: "show"},
			},
			PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`),
		}},
		MockState:  mockState,
		MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`)}},
		Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{Destroy: harness.StageResult{Stage: "destroy"}, OrphanCount: 0}},
	}

	cmd := newRunCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run command: %v", err)
	}

	store := runstore.NewFilesystemStore(h.RunstoreRoot())
	runs, err := store.ListRuns("example-scenario")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run, got %+v", runs)
	}

	planPayload, err := store.ReadRunArtifact("example-scenario", runs[0].RunID, "plan.txt")
	if err != nil {
		t.Fatalf("read plan artifact: %v", err)
	}
	if string(planPayload) != "Plan: 1 to add, 0 to change, 0 to destroy.\n" {
		t.Fatalf("unexpected plan artifact: %q", string(planPayload))
	}

	if _, err := store.ReadRunArtifact("example-scenario", runs[0].RunID, "baseline_state.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no baseline artifact for clean run, got err=%v", err)
	}
}

func TestRunCommandLLMRawCaptureDisabledByDefault(t *testing.T) {
	h := newCommandTestHarness(t)
	opts := isolatedRunOpts(h, nil)
	opts.deps = RuntimeDependencies{
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
	}

	cmd := newRunCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run command: %v", err)
	}

	store := runstore.NewFilesystemStore(h.RunstoreRoot())
	runs, err := store.ListRuns("example-scenario")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run, got %+v", runs)
	}
	matches, err := filepath.Glob(filepath.Join(h.RunstoreRoot(), "example-scenario", runs[0].RunID, "iterations", "1", "llm_raw_*.json"))
	if err != nil {
		t.Fatalf("glob llm raw artifacts: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no llm raw artifacts by default, got %v", matches)
	}
	promptMatches, err := filepath.Glob(filepath.Join(h.RunstoreRoot(), "example-scenario", runs[0].RunID, "iterations", "1", "llm_prompt_*.json"))
	if err != nil {
		t.Fatalf("glob llm prompt artifacts: %v", err)
	}
	if len(promptMatches) != 0 {
		t.Fatalf("expected no llm prompt artifacts by default, got %v", promptMatches)
	}
}

func TestRunCommandLLMRawCaptureWritesPhaseArtifactsWhenEnabled(t *testing.T) {
	h := newCommandTestHarness(t)
	t.Setenv(llmRawCaptureEnvVar, "1")
	opts := isolatedRunOpts(h, nil)
	opts.deps = RuntimeDependencies{
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
	}

	cmd := newRunCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run command: %v", err)
	}

	store := runstore.NewFilesystemStore(h.RunstoreRoot())
	runs, err := store.ListRuns("example-scenario")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run, got %+v", runs)
	}
	artifactPath := filepath.Join(h.RunstoreRoot(), "example-scenario", runs[0].RunID, "iterations", "1", "llm_raw_generate_hcl.json")
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

	promptArtifactPath := filepath.Join(h.RunstoreRoot(), "example-scenario", runs[0].RunID, "iterations", "1", "llm_prompt_generate_hcl.json")
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

func TestRunCommandPersistsRepairIterationsMaxInMetadata(t *testing.T) {
	h := newCommandTestHarness(t)

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 3
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static:     &fakeStaticHarness{result: &harness.StaticResult{Stages: []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}}, PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`)}},
		MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`)}},
		Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{Destroy: harness.StageResult{Stage: "destroy"}, OrphanCount: 0}},
	}

	cmd := newRunCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run command: %v", err)
	}

	store := runstore.NewFilesystemStore(h.RunstoreRoot())
	runs, err := store.ListRuns("example-scenario")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run, got %d", len(runs))
	}
	if runs[0].RepairIterationsMax != 3 {
		t.Fatalf("expected repair_iterations_max 3, got %d", runs[0].RepairIterationsMax)
	}
}

func TestRunCommandStopsAfterFirstSuccess(t *testing.T) {
	h := newCommandTestHarness(t)

	genCalls := 0
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 2
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			genCalls++
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static:     &fakeStaticHarness{result: &harness.StaticResult{Stages: []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}}, PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`)}},
		MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`)}},
		Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{Destroy: harness.StageResult{Stage: "destroy"}, OrphanCount: 0}},
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
	iter := 0
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 2
		return cfg
	})
	opts.deps = RuntimeDependencies{
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
	if !strings.Contains(stdout.String(), "check=repair_budget_exhausted") {
		t.Fatalf("expected repair_budget_exhausted failure marker, got:\n%s", stdout.String())
	}

	store := runstore.NewFilesystemStore(h.RunstoreRoot())
	runs, err := store.ListRuns("example-scenario")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "failed" {
		t.Fatalf("expected one failed run, got: %+v", runs)
	}

	iterationPath := filepath.Join(h.RunstoreRoot(), "example-scenario", runs[0].RunID, "iterations", "2", "iteration.json")
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
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 4
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return nil, errors.New("seed failed")
		}),
		Static:     &fakeStaticHarness{},
		MockDeploy: &fakeMockDeployHarness{},
		Destroy:    &fakeDestroyHarness{},
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

	store := runstore.NewFilesystemStore(h.RunstoreRoot())
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

	requests := make([]generator.Request, 0, 2)
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 3
		return cfg
	})
	opts.deps = RuntimeDependencies{
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
	opts := isolatedRunOpts(h, nil)

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

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 5
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return nil, generator.NewGenerateError(generator.ErrTransportFailed, "generate_hcl", errors.New("transport timeout"))
		}),
		Static:     &fakeStaticHarness{},
		MockDeploy: &fakeMockDeployHarness{},
		Destroy:    &fakeDestroyHarness{},
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

	store := runstore.NewFilesystemStore(h.RunstoreRoot())
	runs, err := store.ListRuns("example-scenario")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run, got %+v", runs)
	}
	iterationPath := filepath.Join(h.RunstoreRoot(), "example-scenario", runs[0].RunID, "iterations", "2", "iteration.json")
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

func TestRunCommandUsesSandboxLayerWhenEnabled(t *testing.T) {
	h := newCommandTestHarness(t)
	scenarioPath := writeUnsupportedCriteriaScenario(t, h.WorkspaceDir)
	t.Setenv("SCW_ACCESS_KEY", "real-access")
	t.Setenv("SCW_SECRET_KEY", "real-secret")
	sandboxDeploy := &fakeSandboxDeployHarness{
		result: &harness.SandboxDeployResult{
			Init:  harness.StageResult{Stage: "init"},
			Apply: harness.StageResult{Stage: "apply"},
		},
	}
	sandboxDestroy := &fakeSandboxDestroyHarness{
		result: &harness.SandboxDestroyResult{
			Destroy: harness.StageResult{Stage: "destroy"},
		},
	}
	realProbe := &fakeRealProbeHarness{
		result: &harness.RealProbeResult{},
	}
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Validation.Layers.SandboxDeploy.Enabled = true
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{
				"main.tf":    []byte("terraform {}\n"),
				"project.tf": []byte("resource \"scaleway_account_project\" \"sandbox\" { name = \"test\" }\n"),
			}}, nil
		}),
		Static: &fakeStaticHarness{result: &harness.StaticResult{
			Stages:   []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}},
			PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`),
		}},
		MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{
			Apply:         harness.StageResult{Stage: "apply"},
			StateSnapshot: []byte(`{}`),
		}},
		Destroy: &fakeDestroyHarness{result: &harness.DestroyResult{
			Destroy:       harness.StageResult{Stage: "destroy"},
			StateSnapshot: []byte(`{"instance":{"servers":[]}}`),
			OrphanCount:   0,
		}},
		SandboxDeploy:  sandboxDeploy,
		SandboxDestroy: sandboxDestroy,
		RealProbe:      realProbe,
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected run success, got %v", err)
	}
	if sandboxDeploy.calls != 1 || sandboxDestroy.calls != 1 {
		t.Fatalf("expected sandbox apply+destroy once, got deploy=%d destroy=%d", sandboxDeploy.calls, sandboxDestroy.calls)
	}
	if realProbe.calls != 1 {
		t.Fatalf("expected sandbox real probe once, got %d", realProbe.calls)
	}
	if !strings.Contains(stdout.String(), "- run/iteration_1_test: pass") {
		t.Fatalf("expected test stage pass, got:\n%s", stdout.String())
	}
}

func TestRunCommandHonorsDisabledStaticMockAndDestructionLayers(t *testing.T) {
	h := newCommandTestHarness(t)
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Validation.Layers.Static.Enabled = false
		cfg.Validation.Layers.MockDeploy.Enabled = false
		cfg.Validation.Layers.Destruction.Enabled = false
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
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
	scenarioPath := writeCriteriaScenario(t, h.WorkspaceDir, "success", "pass")

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 3
		cfg.Validation.Layers.Destruction.Enabled = false
		cfg.ConstraintPolicies = map[string]string{
			"encryption_at_rest": filepath.Join("..", "harness", "testdata", "state-policy", "policy.rego"),
		}
		return cfg
	})
	opts.deps = RuntimeDependencies{
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
  "rdb": {"public_endpoint": true}
}`),
			},
		},
		Destroy: &fakeDestroyHarness{},
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
	if !strings.Contains(stdout.String(), "policy=encryption_at_rest") {
		t.Fatalf("expected criteria-level policy failure in run output, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "check=stuck") {
		t.Fatalf("expected stuck detection marker in run output, got:\n%s", stdout.String())
	}
}

func TestRunCommandExecutesCriteriaOnlyHoldoutsAfterConvergence(t *testing.T) {
	h := newCommandTestHarness(t)
	writeCriteriaOnlyHoldout(t, filepath.Join(h.WorkspaceDir, "scenarios", "holdout", "holdout-pass.yaml"), h.ScenarioPath, `  - type: destruction
    expect: no_orphans
`, "holdout-pass")

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Paths.Scenarios = filepath.Join(h.WorkspaceDir, "scenarios")
		cfg.Validation.Layers.Static.Enabled = false
		cfg.Validation.Layers.MockDeploy.Enabled = false
		cfg.Validation.Layers.Destruction.Enabled = false
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		MockDeploy: &fakeMockDeployHarness{},
		Destroy:    &fakeDestroyHarness{},
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
	writeCriteriaOnlyHoldout(t, filepath.Join(h.WorkspaceDir, "scenarios", "holdout", "holdout-pass.yaml"), h.ScenarioPath, `  - type: destruction
    expect: no_orphans
`, "holdout-pass")

	mockDeploy := &fakeMockDeployHarness{
		result: &harness.MockDeployResult{
			Apply:         harness.StageResult{Stage: "apply"},
			StateSnapshot: []byte(`{}`),
		},
	}
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Paths.Scenarios = filepath.Join(h.WorkspaceDir, "scenarios")
		cfg.Validation.Layers.Static.Enabled = false
		cfg.Validation.Layers.MockDeploy.Enabled = true
		cfg.Validation.Layers.Destruction.Enabled = false
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		MockDeploy: mockDeploy,
		Destroy:    &fakeDestroyHarness{},
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
	expected := filepath.Join(h.OutputDir(), "example-scenario")
	if mockDeploy.dirs[0] != expected || mockDeploy.dirs[1] != expected {
		t.Fatalf("expected both mock deploy calls to use %q, got %#v", expected, mockDeploy.dirs)
	}
}

func TestRunCommandAutoPassesDeferredDNSHoldoutWithoutFeedbackInjection(t *testing.T) {
	h := newCommandTestHarness(t)
	writeCriteriaOnlyHoldout(t, filepath.Join(h.WorkspaceDir, "scenarios", "holdout", "holdout-block.yaml"), h.ScenarioPath, `  - type: dns_resolution
    domain: "{{scenario_name}}.example.com"
    expect: resolves
`, "holdout-block")

	genCalls := 0
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Paths.Scenarios = filepath.Join(h.WorkspaceDir, "scenarios")
		cfg.Validation.Layers.Static.Enabled = false
		cfg.Validation.Layers.MockDeploy.Enabled = false
		cfg.Validation.Layers.Destruction.Enabled = false
		cfg.Agent.RepairIterationsMax = 3
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			genCalls++
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		MockDeploy: &fakeMockDeployHarness{},
		Destroy:    &fakeDestroyHarness{},
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
	if !strings.Contains(stdout.String(), "Layer 3 is disabled") {
		t.Fatalf("expected holdout auto-pass message, got:\n%s", stdout.String())
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
	cmd.Flags().Bool("clean", false, "")
	cmd.Flags().Bool("no-destroy", false, "")

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
	cmd.Flags().Bool("clean", false, "")
	cmd.Flags().Bool("no-destroy", false, "")
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
			cmd.Flags().Bool("clean", false, "")
			cmd.Flags().Bool("no-destroy", false, "")
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

func TestResolveRunControlsRejectsMutuallyExclusiveCleanAndNoDestroy(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().Int("repair-iterations-max", 0, "")
	cmd.Flags().Bool("clean", false, "")
	cmd.Flags().Bool("no-destroy", false, "")
	_ = cmd.Flags().Set("clean", "true")
	_ = cmd.Flags().Set("no-destroy", "true")

	_, err := resolveRunControls(cmd, &CommandRuntime{Config: config.Default()})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual exclusivity error, got %v", err)
	}
}

func TestDetectRunModeIncrementalWhenAllSignalsPresent(t *testing.T) {
	t.Parallel()

	outputDir := filepath.Join(t.TempDir(), "output", "example-scenario")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("mkdir output dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "terraform.tfstate"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write tfstate: %v", err)
	}

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:  "example-scenario",
		RunID:     "run-123",
		Status:    "success",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write run metadata: %v", err)
	}

	mode, err := detectRunMode(context.Background(), &CommandRuntime{
		Deps: RuntimeDependencies{
			MockState: &fakeRunMockStateClient{statePayload: []byte(`{"instance":{"servers":[{"id":"srv-1"}]}}`)},
		},
	}, store, "example-scenario", outputDir, runControls{})
	if err != nil {
		t.Fatalf("detect run mode: %v", err)
	}
	if mode.Mode != runModeIncremental {
		t.Fatalf("expected incremental mode, got %+v", mode)
	}
	if mode.PreviousRunID != "run-123" {
		t.Fatalf("expected previous run id run-123, got %+v", mode)
	}
}

func TestRunCommandNoDestroySkipsDestroyAndHoldouts(t *testing.T) {
	h := newCommandTestHarness(t)
	writeCriteriaOnlyHoldout(t, filepath.Join(h.WorkspaceDir, "scenarios", "holdout", "holdout-skip.yaml"), h.ScenarioPath, `  - type: destruction
    expect: no_orphans
`, "holdout-skip")

	mockDeploy := &fakeMockDeployHarness{
		result: &harness.MockDeployResult{
			Apply:         harness.StageResult{Stage: "apply"},
			StateSnapshot: []byte(`{"mock":true}`),
		},
	}
	destroy := &fakeDestroyHarness{
		result: &harness.DestroyResult{
			Destroy:       harness.StageResult{Stage: "destroy"},
			StateSnapshot: []byte(`{}`),
			OrphanCount:   0,
		},
	}

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Paths.Scenarios = filepath.Join(h.WorkspaceDir, "scenarios")
		cfg.Validation.Layers.Static.Enabled = false
		cfg.Agent.RepairIterationsMax = 1
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		MockDeploy: mockDeploy,
		Destroy:    destroy,
		MockState:  &fakeRunMockStateClient{statePayload: []byte(`{"instance":{"servers":[]}}`)},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--no-destroy"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run command: %v", err)
	}
	if destroy.calls != 0 {
		t.Fatalf("expected destroy harness to be skipped, got %d calls", destroy.calls)
	}
	if !strings.Contains(stdout.String(), "holdout/skipped: skip (skipped by --no-destroy)") {
		t.Fatalf("expected holdout skip output, got:\n%s", stdout.String())
	}
}

func TestRunCommandAutoDestroysRealResourcesOnFailure(t *testing.T) {
	h := newCommandTestHarness(t)
	outputRoot := filepath.Join(h.WorkspaceDir, "output")
	t.Setenv("SCW_ACCESS_KEY", "real-access")
	t.Setenv("SCW_SECRET_KEY", "real-secret")

	// Both iterations fail at validate (stuck detection triggers).
	// The generator includes terraform-live.tfstate in its output files so that
	// writeGeneratedFiles persists it alongside the .tf files — simulating a
	// prior sandbox deploy having left state behind.
	sandboxDestroy := &fakeSandboxDestroyHarness{
		result: &harness.SandboxDestroyResult{
			Destroy: harness.StageResult{Stage: "destroy"},
		},
	}
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Paths.Output = outputRoot
		cfg.Validation.Layers.SandboxDeploy.Enabled = true
		cfg.Agent.RepairIterationsMax = 1
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(_ context.Context, _ generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{
				"main.tf":                 []byte("terraform {}\n"),
				"project.tf":              []byte("resource \"scaleway_account_project\" \"sandbox\" { name = \"test\" }\n"),
				harness.LiveStateFilename: []byte(`{"version":4}`),
			}}, nil
		}),
		Static:         &fakeStaticHarness{err: &harness.StageError{StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}}, Err: errors.New("validate failed")}},
		MockDeploy:     &fakeMockDeployHarness{},
		Destroy:        &fakeDestroyHarness{},
		SandboxDeploy:  &fakeSandboxDeployHarness{},
		SandboxDestroy: sandboxDestroy,
		RealProbe:      &fakeRealProbeHarness{result: &harness.RealProbeResult{}},
	}

	cmd := newRunCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run failure")
	}
	if sandboxDestroy.calls != 1 {
		t.Fatalf("expected auto-destroy of real resources on failure, got %d destroy calls", sandboxDestroy.calls)
	}
}

func TestRunCommandNoDestroyPreservesRealResourcesOnFailure(t *testing.T) {
	h := newCommandTestHarness(t)
	outputRoot := filepath.Join(h.WorkspaceDir, "output")
	t.Setenv("SCW_ACCESS_KEY", "real-access")
	t.Setenv("SCW_SECRET_KEY", "real-secret")

	sandboxDestroy := &fakeSandboxDestroyHarness{
		result: &harness.SandboxDestroyResult{
			Destroy: harness.StageResult{Stage: "destroy"},
		},
	}
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Paths.Output = outputRoot
		cfg.Validation.Layers.SandboxDeploy.Enabled = true
		cfg.Agent.RepairIterationsMax = 1
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(_ context.Context, _ generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{
				"main.tf":                 []byte("terraform {}\n"),
				"project.tf":              []byte("resource \"scaleway_account_project\" \"sandbox\" { name = \"test\" }\n"),
				harness.LiveStateFilename: []byte(`{"version":4}`),
			}}, nil
		}),
		Static:         &fakeStaticHarness{err: &harness.StageError{StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}}, Err: errors.New("validate failed")}},
		MockDeploy:     &fakeMockDeployHarness{},
		Destroy:        &fakeDestroyHarness{},
		SandboxDeploy:  &fakeSandboxDeployHarness{},
		SandboxDestroy: sandboxDestroy,
		RealProbe:      &fakeRealProbeHarness{result: &harness.RealProbeResult{}},
	}

	cmd := newRunCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--no-destroy"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run failure")
	}
	if sandboxDestroy.calls != 0 {
		t.Fatalf("expected no auto-destroy with --no-destroy, got %d destroy calls", sandboxDestroy.calls)
	}
}

func TestRunCommandIncrementalModeSnapshotsBaselineAndPersistsMetadata(t *testing.T) {
	h := newCommandTestHarness(t)
	outputRoot := filepath.Join(h.WorkspaceDir, "output")
	outputDir := filepath.Join(outputRoot, "example-scenario")

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("mkdir output dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "terraform.tfstate"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write tfstate: %v", err)
	}

	store := runstore.NewFilesystemStore(h.RunstoreRoot())
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:  "example-scenario",
		RunID:     "run-prev",
		Status:    "success",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write prior run metadata: %v", err)
	}

	mockState := &fakeRunMockStateClient{statePayload: []byte(`{"instance":{"servers":[{"id":"srv-1"}]}}`)}
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Paths.Output = outputRoot
		cfg.Agent.RepairIterationsMax = 1
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static:     &fakeStaticHarness{result: &harness.StaticResult{Stages: []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}}, PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`)}},
		MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{"mock":true}`)}},
		Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{Destroy: harness.StageResult{Stage: "destroy"}, OrphanCount: 0}},
		MockState:  mockState,
	}

	cmd := newRunCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run command: %v", err)
	}
	if mockState.snapshotCalls != 1 {
		t.Fatalf("expected one snapshot call, got %d", mockState.snapshotCalls)
	}

	runs, err := store.ListRuns("example-scenario")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected persisted runs")
	}
	var current runstore.RunMetadata
	found := false
	for _, run := range runs {
		if run.PreviousRunID == "run-prev" {
			current = run
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected current incremental run metadata, got %+v", runs)
	}
	if !current.Incremental {
		t.Fatalf("expected incremental metadata, got %+v", current)
	}
}

type fakeRunMockStateClient struct {
	statePayload  []byte
	snapshotCalls int
	restoreCalls  int
	resetCalls    int
}

func (f *fakeRunMockStateClient) Reset(context.Context) error {
	f.resetCalls++
	return nil
}

func (f *fakeRunMockStateClient) Snapshot(context.Context) error {
	f.snapshotCalls++
	return nil
}

func (f *fakeRunMockStateClient) Restore(context.Context) error {
	f.restoreCalls++
	return nil
}

func (f *fakeRunMockStateClient) State(context.Context) ([]byte, error) {
	return f.statePayload, nil
}

func newRunCommandForTest(opts runtimeOptions) *cobra.Command {
	if opts.deps.MockState == nil {
		opts.deps.MockState = &fakeRunMockStateClient{statePayload: []byte(`{"instance":{"servers":[]}}`)}
	}
	cmd := &cobra.Command{
		Use:  "run <scenario>",
		Args: requireScenarioArg,
		RunE: withRuntimeWithOptions("run", opts, runRunCommand),
	}
	cmd.Flags().String("config", config.DefaultPath, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	cmd.Flags().Int("repair-iterations-max", 0, "")
	cmd.Flags().Bool("clean", false, "")
	cmd.Flags().Bool("no-destroy", false, "")
	return cmd
}

func TestRunCommandHoldoutsExecuteLayer3WhenEnabled(t *testing.T) {
	h := newCommandTestHarness(t)
	t.Setenv("SCW_ACCESS_KEY", "real-access")
	t.Setenv("SCW_SECRET_KEY", "real-secret")

	// Training scenario uses dns_resolution which triggers sandbox deploy + real probes when Layer 3 is enabled.
	trainingPath := filepath.Join(h.WorkspaceDir, "scenarios", "training", "layer3-training.yaml")
	mustWriteFile(t, trainingPath, `scenario: layer3-training
version: "1.0"
cloud: scaleway
description: training scenario with dns criterion
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
  - type: dns_resolution
    domain: "{{scenario_name}}.example.com"
    expect: resolves
`)

	// Holdout scenario also has dns_resolution so it exercises Layer 3 during holdout phase.
	holdoutPath := filepath.Join(h.WorkspaceDir, "scenarios", "holdout", "holdout-layer3.yaml")
	writeCriteriaOnlyHoldout(t, holdoutPath, trainingPath, `  - type: destruction
    expect: no_orphans
  - type: dns_resolution
    domain: "{{scenario_name}}.example.com"
    expect: resolves
`, "holdout-layer3")

	sandboxDeploy := &fakeSandboxDeployHarness{
		result: &harness.SandboxDeployResult{
			Init:  harness.StageResult{Stage: "init"},
			Plan:  harness.StageResult{Stage: "plan"},
			Apply: harness.StageResult{Stage: "apply"},
		},
	}
	sandboxDestroy := &fakeSandboxDestroyHarness{
		result: &harness.SandboxDestroyResult{
			Destroy: harness.StageResult{Stage: "destroy"},
		},
	}
	realProbe := &fakeRealProbeHarness{
		result: &harness.RealProbeResult{},
	}
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Paths.Scenarios = filepath.Join(h.WorkspaceDir, "scenarios")
		cfg.Validation.Layers.SandboxDeploy.Enabled = true
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{
				"main.tf":    []byte("terraform {}\n"),
				"project.tf": []byte("resource \"scaleway_account_project\" \"sandbox\" { name = \"test\" }\n"),
			}}, nil
		}),
		Static: &fakeStaticHarness{result: &harness.StaticResult{
			Stages:   []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}},
			PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`),
		}},
		MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{
			Apply:         harness.StageResult{Stage: "apply"},
			StateSnapshot: []byte(`{}`),
		}},
		Destroy: &fakeDestroyHarness{result: &harness.DestroyResult{
			Destroy:       harness.StageResult{Stage: "destroy"},
			StateSnapshot: []byte(`{"instance":{"servers":[]}}`),
			OrphanCount:   0,
		}},
		SandboxDeploy:  sandboxDeploy,
		SandboxDestroy: sandboxDestroy,
		RealProbe:      realProbe,
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{trainingPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected run success, got %v\nstdout:\n%s", err, stdout.String())
	}

	// Sandbox deploy should be called at least twice: once during the training
	// iteration test, and once during the holdout test.
	if sandboxDeploy.calls < 2 {
		t.Fatalf("expected sandbox deploy called at least 2 times (iteration + holdout), got %d", sandboxDeploy.calls)
	}

	// Sandbox destroy should be called at least twice: once during the training
	// iteration destruction, and once during the holdout destruction.
	if sandboxDestroy.calls < 2 {
		t.Fatalf("expected sandbox destroy called at least 2 times (iteration + holdout), got %d", sandboxDestroy.calls)
	}

	// Real probe should be called at least twice: once during the training
	// iteration criteria evaluation, and once during the holdout.
	if realProbe.calls < 2 {
		t.Fatalf("expected real probe called at least 2 times (iteration + holdout), got %d", realProbe.calls)
	}

	// Verify holdout stages appear in output.
	if !strings.Contains(stdout.String(), "holdout/discovery: pass") {
		t.Fatalf("expected holdout discovery stage, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "holdout/holdout-layer3: pass") {
		t.Fatalf("expected holdout-layer3 pass stage, got:\n%s", stdout.String())
	}
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
