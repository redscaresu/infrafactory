package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/feedback"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/runstore"
	"github.com/redscaresu/infrafactory/internal/scenario"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// Run 20260927T163652Z: iteration 1 omits the private network, iteration 2
// fails elsewhere (a Layer 3 refusal at generate), and iteration 3 must still
// be told about iteration 1, or it re-derives the omission from scratch.
func TestRunCommandKeepsEarlierIterationFailuresInGenerateFeedback(t *testing.T) {
	h := newCommandTestHarness(t)

	const vpcDetail = "scaleway_instance_server.web is not attached to a private network"
	const refusal = "layer 3 refuses this configuration: server_ips calls strcontains()"
	var requests []generator.Request
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 3
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(_ context.Context, req generator.Request) (*generator.GeneratedCode, error) {
			requests = append(requests, req)
			if req.Iteration == 2 {
				return nil, errors.New(refusal)
			}
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static: &fakeStaticHarness{
			err: &harness.StageError{
				StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}},
				Err:         errors.New(vpcDetail),
			},
		},
		MockDeploy: &fakeMockDeployHarness{},
		Destroy:    &fakeDestroyHarness{},
	}

	cmd := newRunCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	require.Error(t, cmd.Execute())
	require.Len(t, requests, 3)
	assert.Contains(t, string(requests[1].FeedbackJSON), vpcDetail)
	third := string(requests[2].FeedbackJSON)
	assert.Contains(t, third, "strcontains()", "iteration 2's failure must reach iteration 3")
	assert.Contains(t, third, vpcDetail, "iteration 1's failure must still reach iteration 3")
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
	sandboxCredsForTest(t)
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
				"main.tf":   []byte("terraform {}\n"),
				"volume.tf": []byte("resource \"scaleway_block_volume\" \"sandbox\" { size_in_gb = 1 }\n"),
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
		RunProject:     &fakeRunProject{created: harness.RunProject{ID: "run-proj-1", Name: "if-run-t"}},
		SandboxDeploy:  sandboxDeploy,
		SandboxDestroy: sandboxDestroy,
		OrphanSweep:    &fakeOrphanSweep{},
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
func TestResolveRunControlsUsesConfigDefaultsWhenFlagsUnset(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "run"}
	registerRunFlags(cmd, false)

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
	registerRunFlags(cmd, false)
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
			registerRunFlags(cmd, false)
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
	registerRunFlags(cmd, false)
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
func TestRunCommandAutoDestroysRealResourcesOnFailure(t *testing.T) {
	h := newCommandTestHarness(t)
	outputRoot := filepath.Join(h.WorkspaceDir, "output")
	sandboxCredsForTest(t)

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
				"volume.tf":               []byte("resource \"scaleway_block_volume\" \"sandbox\" { size_in_gb = 1 }\n"),
				harness.LiveStateFilename: []byte(liveStateWithProject),
				// A prior sandbox deploy leaves BOTH: ensureRunProject
				// writes the marker into the output dir before the apply,
				// so state without one is not a shape a run can produce.
				harness.RunProjectMarkerFilename: []byte(`{"project_id":"11111111-1111-1111-1111-111111111111","name":"if-run-t"}`),
			}}, nil
		}),
		Static:         &fakeStaticHarness{err: &harness.StageError{StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}}, Err: errors.New("validate failed")}},
		MockDeploy:     &fakeMockDeployHarness{},
		Destroy:        &fakeDestroyHarness{},
		RunProject:     &fakeRunProject{created: harness.RunProject{ID: "run-proj-1", Name: "if-run-t"}},
		SandboxDeploy:  &fakeSandboxDeployHarness{},
		SandboxDestroy: sandboxDestroy,
		OrphanSweep:    &fakeOrphanSweep{},
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

// Prior state with no marker cannot be destroyed faithfully: the apply
// ran with the run's own project as the provider default and nothing
// here can say which one that was. Refusing beats destroying against the
// shared fallback -- but silence would be worse than either, so the
// operator gets the reason and the recovery command.
func TestRunCommandRefusesAutoDestroyWhenTheRunProjectIsUnknown(t *testing.T) {
	h := newCommandTestHarness(t)
	outputRoot := filepath.Join(h.WorkspaceDir, "output")
	sandboxCredsForTest(t)

	sandboxDestroy := &fakeSandboxDestroyHarness{
		result: &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}},
	}
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Paths.Output = outputRoot
		cfg.Validation.Layers.SandboxDeploy.Enabled = true
		cfg.Agent.RepairIterationsMax = 1
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(_ context.Context, _ generator.Request) (*generator.GeneratedCode, error) {
			// State, deliberately with NO marker beside it.
			return &generator.GeneratedCode{Files: map[string][]byte{
				"main.tf":                 []byte("terraform {}\n"),
				harness.LiveStateFilename: []byte(liveStateWithProject),
			}}, nil
		}),
		Static:         &fakeStaticHarness{err: &harness.StageError{StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}}, Err: errors.New("validate failed")}},
		MockDeploy:     &fakeMockDeployHarness{},
		Destroy:        &fakeDestroyHarness{},
		RunProject:     &fakeRunProject{created: harness.RunProject{ID: "run-proj-1", Name: "if-run-t"}},
		SandboxDeploy:  &fakeSandboxDeployHarness{},
		SandboxDestroy: sandboxDestroy,
		OrphanSweep:    &fakeOrphanSweep{},
		RealProbe:      &fakeRealProbeHarness{result: &harness.RealProbeResult{}},
	}

	cmd := newRunCommandForTest(opts)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--output", string(OutputModeJSON)})

	require.Error(t, cmd.Execute())
	assert.Zero(t, sandboxDestroy.calls, "destroying against the shared fallback is not the inverse of the apply")
	assert.Contains(t, out.String(), "auto_destroy_preflight")
	assert.Contains(t, out.String(), "real resources are live and were NOT destroyed")
}

func TestRunCommandNoDestroyPreservesRealResourcesOnFailure(t *testing.T) {
	h := newCommandTestHarness(t)
	outputRoot := filepath.Join(h.WorkspaceDir, "output")
	sandboxCredsForTest(t)

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
				"volume.tf":               []byte("resource \"scaleway_block_volume\" \"sandbox\" { size_in_gb = 1 }\n"),
				harness.LiveStateFilename: []byte(liveStateWithProject),
				// A prior sandbox deploy leaves BOTH: ensureRunProject
				// writes the marker into the output dir before the apply,
				// so state without one is not a shape a run can produce.
				harness.RunProjectMarkerFilename: []byte(`{"project_id":"11111111-1111-1111-1111-111111111111","name":"if-run-t"}`),
			}}, nil
		}),
		Static:         &fakeStaticHarness{err: &harness.StageError{StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}}, Err: errors.New("validate failed")}},
		MockDeploy:     &fakeMockDeployHarness{},
		Destroy:        &fakeDestroyHarness{},
		RunProject:     &fakeRunProject{created: harness.RunProject{ID: "run-proj-1", Name: "if-run-t"}},
		SandboxDeploy:  &fakeSandboxDeployHarness{},
		SandboxDestroy: sandboxDestroy,
		OrphanSweep:    &fakeOrphanSweep{},
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
	registerRunFlags(cmd, false)
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

// seqMockDeployHarness fails the calls errs names, in order, then passes.
type seqMockDeployHarness struct {
	*fakeMockDeployHarness
	errs []error
}

func (f *seqMockDeployHarness) Run(ctx context.Context, workDir string, env map[string]string, mode harness.MockDeployMode) (*harness.MockDeployResult, error) {
	f.err = nil
	if f.calls < len(f.errs) {
		f.err = f.errs[f.calls]
	}
	return f.fakeMockDeployHarness.Run(ctx, workDir, env, mode)
}

type seqSandboxDeployHarness struct {
	*fakeSandboxDeployHarness
	errs []error
}

func (f *seqSandboxDeployHarness) Run(ctx context.Context, workDir string, env map[string]string, progress io.Writer) (*harness.SandboxDeployResult, error) {
	f.err = nil
	if f.calls < len(f.errs) {
		f.err = f.errs[f.calls]
	}
	return f.fakeSandboxDeployHarness.Run(ctx, workDir, env, progress)
}

// An avoid rule records the layer whose failure taught it. The run
// rewrites every failure's Layer to "run", so without the recorded
// origin a rule learned against the mock and one learned against the
// real cloud would be indistinguishable in the corpus.
func TestRunCommandRecordsTheLearnedLayerOnAnAvoidPitfall(t *testing.T) {
	const detail = `Error: Unsupported argument on scaleway_block_volume.data: An argument named "bogus_attr" is not expected here.`
	failing := "resource \"scaleway_block_volume\" \"data\" {\n  size_in_gb = 1\n  bogus_attr = true\n}\n"
	passing := "resource \"scaleway_block_volume\" \"data\" {\n  size_in_gb = 1\n}\n"

	for _, layer := range []string{"mock_deploy", "sandbox_deploy"} {
		t.Run(layer, func(t *testing.T) {
			h := newCommandTestHarness(t)
			scenarioPath := writeUnsupportedCriteriaScenario(t, h.WorkspaceDir)
			sandbox := layer == "sandbox_deploy"
			if sandbox {
				sandboxCredsForTest(t)
			}
			opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
				cfg.Validation.Layers.SandboxDeploy.Enabled = sandbox
				return cfg
			})
			generations := 0
			mockDeploy := &seqMockDeployHarness{fakeMockDeployHarness: &fakeMockDeployHarness{result: &harness.MockDeployResult{
				Apply:         harness.StageResult{Stage: "apply"},
				StateSnapshot: []byte(`{}`),
			}}}
			sandboxDeploy := &seqSandboxDeployHarness{fakeSandboxDeployHarness: &fakeSandboxDeployHarness{result: &harness.SandboxDeployResult{
				Init:  harness.StageResult{Stage: "init"},
				Apply: harness.StageResult{Stage: "apply"},
			}}}
			if sandbox {
				// Staged, as the harness reports a failed apply: the run
				// files a Scaleway one under run/iteration_1_test, so only
				// its OriginStage says it was an apply.
				sandboxDeploy.errs = []error{&harness.SandboxDeployError{Stage: "apply", Err: errors.New(detail)}}
			} else {
				mockDeploy.errs = []error{errors.New(detail)}
			}
			opts.deps = RuntimeDependencies{
				Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
					generations++
					volume := passing
					if generations == 1 {
						volume = failing
					}
					return &generator.GeneratedCode{Files: map[string][]byte{
						"main.tf":   []byte("terraform {}\n"),
						"volume.tf": []byte(volume),
					}}, nil
				}),
				Static: &fakeStaticHarness{result: &harness.StaticResult{
					Stages:   []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}},
					PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`),
				}},
				MockDeploy: mockDeploy,
				Destroy: &fakeDestroyHarness{result: &harness.DestroyResult{
					Destroy:       harness.StageResult{Stage: "destroy"},
					StateSnapshot: []byte(`{"instance":{"servers":[]}}`),
				}},
				RunProject:     &fakeRunProject{created: harness.RunProject{ID: "run-proj-1", Name: "if-run-t"}},
				SandboxDeploy:  sandboxDeploy,
				SandboxDestroy: &fakeSandboxDestroyHarness{result: &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}}},
				OrphanSweep:    &fakeOrphanSweep{},
				RealProbe:      &fakeRealProbeHarness{result: &harness.RealProbeResult{}},
			}

			cmd := newRunCommandForTest(opts)
			stdout := &bytes.Buffer{}
			cmd.SetOut(stdout)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath})
			require.NoError(t, cmd.Execute(), stdout.String())
			require.Equal(t, 2, generations, "the run repairs once and passes")

			entries, err := generator.LoadPitfallEntries(h.PitfallsDir(), "scaleway")
			require.NoError(t, err)
			var avoid []generator.PitfallEntry
			for _, e := range entries {
				if e.Source == generator.AvoidSource {
					avoid = append(avoid, e)
				}
			}
			require.Len(t, avoid, 1, "entries: %+v", entries)
			assert.Equal(t, "scaleway_block_volume", avoid[0].Resource)
			assert.Contains(t, avoid[0].Rule, "bogus_attr")
			assert.Equal(t, layer, avoid[0].LearnedLayer)
		})
	}
}

// A failure the run cleared by deleting a resource the mock refused is
// the mock's gap: target_reached's N10/N13 extractors must not learn an
// avoid rule from it, and it lands in docs/mock-gaps.md even when it was
// cleared before the last failing iteration. The detail is verbatim
// `tofu apply` (aws provider 5.100.0) against fakeaws main at 9bc2238.
func TestRunCommandLearnsNoPitfallFromAClearedMockRefusal(t *testing.T) {
	const detail = `Error: creating Security Group (web): operation error EC2: CreateSecurityGroup, https response error StatusCode: 501, RequestID: fakeaws-synthetic, api error NotImplemented: CreateSecurityGroup: not modelled by fakeaws: no GroupName, GroupDescription or VpcId

  with aws_security_group.web,
  on main.tf line 15, in resource "aws_security_group" "web":
  15: resource "aws_security_group" "web" {`
	for _, tc := range []struct {
		name string
		errs []error
	}{
		{"cleared by the passing iteration", []error{errors.New(detail)}},
		{"cleared before an unrelated failure", []error{errors.New(detail), errors.New("Error: the apply was interrupted")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newCommandTestHarness(t)
			scenarioPath := filepath.Join(h.WorkspaceDir, "scenarios", "training", "aws.yaml")
			mustWriteFile(t, scenarioPath, `scenario: example-scenario
version: "1.0"
cloud: aws
description: example
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: connectivity
    from: public_internet
    to: compute
    expect: blocked
`)
			docsDir := t.TempDir()
			opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
				cfg.Paths.Docs = docsDir
				return cfg
			})
			generations := 0
			opts.deps = RuntimeDependencies{
				Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
					generations++
					files := map[string][]byte{"main.tf": []byte("terraform {}\n")}
					if generations == 1 {
						files["sg.tf"] = []byte("resource \"aws_security_group\" \"web\" {\n  name        = \"web\"\n  description = \"web\"\n}\n")
					}
					return &generator.GeneratedCode{Files: files}, nil
				}),
				Static: &fakeStaticHarness{result: &harness.StaticResult{
					Stages:   []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}},
					PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`),
				}},
				MockDeploy: &seqMockDeployHarness{
					fakeMockDeployHarness: &fakeMockDeployHarness{result: &harness.MockDeployResult{
						Apply:         harness.StageResult{Stage: "apply"},
						StateSnapshot: []byte(`{}`),
					}},
					errs: tc.errs,
				},
				Destroy: &fakeDestroyHarness{result: &harness.DestroyResult{
					Destroy:       harness.StageResult{Stage: "destroy"},
					StateSnapshot: []byte(`{}`),
				}},
			}

			cmd := newRunCommandForTest(opts)
			stdout := &bytes.Buffer{}
			cmd.SetOut(stdout)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath})
			require.NoError(t, cmd.Execute(), stdout.String())
			require.Equal(t, len(tc.errs)+1, generations, "the run repairs each failure and passes")

			entries, err := generator.LoadPitfallEntries(h.PitfallsDir(), "aws")
			require.NoError(t, err)
			assert.Empty(t, entries)
			gaps, err := os.ReadFile(filepath.Join(docsDir, "mock-gaps.md"))
			require.NoError(t, err)
			assert.Contains(t, string(gaps), "| aws_security_group | `not modelled by fakeaws` |")
		})
	}
}

// The Layer 3 learning tests' failure: a ResourceNotFoundException in the
// aws provider's error shape, which the real cloud returns as well as the
// mock, on a metric filter whose log group the passing HCL adds.
const (
	logFilterResource = "aws_cloudwatch_log_metric_filter"
	logFilterNotFound = `Error: creating CloudWatch Logs Metric Filter (errors): operation error CloudWatch Logs: PutMetricFilter, https response error StatusCode: 400, RequestID: 7a62c49f-347e-4fc4-9331-6e8eEXAMPLE, ResourceNotFoundException: The specified log group does not exist.

  with aws_cloudwatch_log_metric_filter.errors,
  on main.tf line 1, in resource "aws_cloudwatch_log_metric_filter" "errors":
   1: resource "aws_cloudwatch_log_metric_filter" "errors" {`
	logFilterFailing = "resource \"aws_cloudwatch_log_metric_filter\" \"errors\" {\n  name           = \"errors\"\n  pattern        = \"ERROR\"\n  log_group_name = \"/app/web\"\n}\n"
	logFilterPassing = "resource \"aws_cloudwatch_log_group\" \"web\" {\n  name = \"/app/web\"\n}\n\n" +
		"resource \"aws_cloudwatch_log_metric_filter\" \"errors\" {\n  name           = \"errors\"\n  pattern        = \"ERROR\"\n  log_group_name = aws_cloudwatch_log_group.web.name\n}\n"
)

// natGatewayRefusal is the Layer 3 allowlist's refusal of aws_nat_gateway,
// as the test step reports it.
const natGatewayRefusal = "layer 3 refuses to apply resource type(s) aws_nat_gateway: not in validation.layers.sandbox_deploy.allow_resource_types"

// layer3LearningRun is one aws Layer 3 `run` for the learner paths, with
// docs/ in a temp dir (returned) and aws_cloudwatch_log_* and allow added
// to the allowlist. main.tf is files[n] on generation n+1, the last
// repeating; deps then edits the dependencies.
func layer3LearningRun(t *testing.T, repairs int, files []string, deps func(*awsLifecycle, *RuntimeDependencies), allow ...string) (awsRun, string) {
	t.Helper()
	lc := newAWSLifecycle(t)
	docsDir := t.TempDir()
	generations := 0
	run := runAWSRun(t, lc, awsRunOptions{
		repairs: repairs,
		customize: func(cfg *config.Config) {
			cfg.Paths.Docs = docsDir
			sandbox := &cfg.Validation.Layers.SandboxDeploy
			sandbox.AllowResourceTypes = append(sandbox.AllowResourceTypes, append([]string{"aws_cloudwatch_log_*"}, allow...)...)
		},
		deps: func(d *RuntimeDependencies) {
			d.Generator = generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				main := files[min(generations, len(files)-1)]
				generations++
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte(main)}}, nil
			})
			deps(lc, d)
		},
	})
	return run, docsDir
}

// runAppLog is the run's app.log.
func runAppLog(t *testing.T, run awsRun) string {
	t.Helper()
	logs, err := filepath.Glob(filepath.Join(run.h.RunstoreRoot(), "*", "*", "app.log"))
	require.NoError(t, err)
	require.Len(t, logs, 1)
	raw, err := os.ReadFile(logs[0])
	require.NoError(t, err)
	return string(raw)
}

// learnedSources is each aws pitfall's source, after asserting every one
// names resource and was learned at layer.
func learnedSources(t *testing.T, run awsRun, resource, layer string) []string {
	t.Helper()
	entries, err := generator.LoadPitfallEntries(run.h.PitfallsDir(), "aws")
	require.NoError(t, err)
	var sources []string
	for _, e := range entries {
		assert.Equal(t, resource, e.Resource)
		assert.Equal(t, layer, e.LearnedLayer)
		sources = append(sources, e.Source)
	}
	return sources
}

// layer3ApplyFails is a Layer 3 apply that failed with stderr, as the
// sandbox harness reports one; its failure detail is
// "exit status 1 | stderr: " + stderr.
func layer3ApplyFails(stderr string) error {
	return &harness.SandboxDeployError{
		Stage: "apply", Err: errors.New("exit status 1"), Apply: harness.StageResult{Stage: "apply", Stderr: stderr},
	}
}

// The real cloud returns some mock signals too (ResourceNotFoundException),
// so a Layer 3 failure carrying one is learned on every learner path and
// never written to docs/mock-gaps.md; the same detail from the mock still
// is. target_reached runs the self-correction path (a descriptive rule)
// and the prescriptive one (a fix rule); repair_budget_exhausted runs the
// stuck/budget path alone.
func TestAWSRunLearnsALayer3FailureThatCarriesAMockSignal(t *testing.T) {
	for _, path := range []struct {
		name     string
		repairs  int
		terminal string
		// sources are the rules a Layer 3 failure teaches; events the
		// mock-gap log lines a mock failure writes instead.
		sources []string
		events  []string
	}{
		{"self-correction and prescriptive", 2, "target_reached",
			[]string{"descriptive", generator.FixSource},
			[]string{"self_correction_mock_gap_recorded", "prescriptive_mock_gap_recorded"}},
		{"stuck or budget", 1, "repair_budget_exhausted",
			[]string{"descriptive"},
			[]string{"oscillation_mock_gap_recorded"}},
	} {
		for _, layer := range []string{generator.MockDeployLayer, generator.SandboxDeployLayer} {
			t.Run(path.name+"/"+layer, func(t *testing.T) {
				run, docsDir := layer3LearningRun(t, path.repairs, []string{logFilterFailing, logFilterPassing}, func(lc *awsLifecycle, d *RuntimeDependencies) {
					if layer == generator.MockDeployLayer {
						d.MockDeploy = &seqMockDeployHarness{fakeMockDeployHarness: &fakeMockDeployHarness{},
							errs: []error{errors.New("exit status 1 | stderr: " + logFilterNotFound)}}
					} else {
						d.SandboxDeploy = &seqSandboxDeployHarness{fakeSandboxDeployHarness: lc.deploy, errs: []error{layer3ApplyFails(logFilterNotFound)}}
					}
				})
				require.Equal(t, path.terminal, run.terminalReason(), run.output)

				gaps := filepath.Join(docsDir, "mock-gaps.md")
				if layer == generator.MockDeployLayer {
					assert.Empty(t, learnedSources(t, run, logFilterResource, layer))
					raw, err := os.ReadFile(gaps)
					require.NoError(t, err)
					assert.Contains(t, string(raw), "| "+logFilterResource+" | `resourcenotfoundexception` |")
					appLog := runAppLog(t, run)
					for _, event := range path.events {
						assert.Contains(t, appLog, `"event":"`+event+`"`)
					}
					return
				}
				assert.NoFileExists(t, gaps)
				assert.ElementsMatch(t, path.sources, learnedSources(t, run, logFilterResource, layer))
			})
		}
	}
}

// A failure the run builds itself has no Origin; a generate error stands
// in for one here, carrying a resource and ResourceNotFoundException. With
// Layer 3 off the run only ever met the mock, so it is a mock gap, as it
// always was. With Layer 3 on it may have met the real cloud, so it is
// never a mock gap: it is learned, stamped with the layer it was routed
// by, which the pitfalls ratchet admits.
func TestAWSRunRoutesAFailureWithNoOriginByLayer3(t *testing.T) {
	for _, path := range []struct {
		name     string
		repairs  int
		terminal string
	}{
		// Generation fails once and then passes: the self-correction path.
		{"self-correction", 2, "target_reached"},
		{"stuck or budget", 1, "repair_budget_exhausted"},
	} {
		for _, layer3 := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/layer3=%t", path.name, layer3), func(t *testing.T) {
				lc := newAWSLifecycle(t)
				docsDir := t.TempDir()
				generations := 0
				run := runAWSRun(t, lc, awsRunOptions{
					repairs: path.repairs,
					customize: func(cfg *config.Config) {
						cfg.Paths.Docs = docsDir
						cfg.Validation.Layers.SandboxDeploy.Enabled = layer3
					},
					deps: func(d *RuntimeDependencies) {
						d.Generator = generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
							if generations++; generations == 1 {
								return nil, errors.New(logFilterNotFound)
							}
							return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
						})
					},
				})
				require.Equal(t, path.terminal, run.terminalReason(), run.output)

				gaps := filepath.Join(docsDir, "mock-gaps.md")
				if !layer3 {
					raw, err := os.ReadFile(gaps)
					require.NoError(t, err)
					assert.Contains(t, string(raw), "| "+logFilterResource+" | `resourcenotfoundexception` |")
					assert.Empty(t, learnedSources(t, run, logFilterResource, "run"))
					return
				}
				assert.NoFileExists(t, gaps)
				assert.Equal(t, []string{"descriptive"}, learnedSources(t, run, logFilterResource, "run"))
				entries, err := generator.LoadPitfallEntries(run.h.PitfallsDir(), "aws")
				require.NoError(t, err)
				for _, e := range entries {
					assert.Empty(t, generator.ForbiddenPitfallSignal(e.LearnedLayer, e.Rule), "the ratchet refuses what the run wrote")
				}
			})
		}
	}
}

// The same detail from the mock in iteration 1 and from the real cloud in
// iteration 2 is two failures: the mock's is a gap, the real cloud's a
// lesson, so neither dedupe drops the second as a repeat of the first.
// Their checks differ (the mock's error names no stage), so the run does
// not stop as stuck at iteration 2.
func TestAWSRunLearnsALayer3FailureAfterTheSameMockFailure(t *testing.T) {
	const detail = "exit status 1 | stderr: " + logFilterNotFound
	for _, tc := range []struct {
		name     string
		repairs  int
		terminal string
		sources  []string
		event    string
	}{
		// Iterations 1 and 2 fail; the stuck/budget path sees both.
		{"stuck or budget", 2, "repair_budget_exhausted", []string{"descriptive"}, "oscillation_mock_gap_recorded"},
		// Iteration 3 passes: the pair 1->2 cleared the mock's failure,
		// and 2->3 the real cloud's.
		{"prescriptive", 3, "target_reached", []string{"descriptive", generator.FixSource}, "prescriptive_mock_gap_recorded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run, docsDir := layer3LearningRun(t, tc.repairs, []string{logFilterFailing, logFilterFailing, logFilterPassing}, func(lc *awsLifecycle, d *RuntimeDependencies) {
				d.MockDeploy = &seqMockDeployHarness{fakeMockDeployHarness: &fakeMockDeployHarness{}, errs: []error{errors.New(detail)}}
				d.SandboxDeploy = &seqSandboxDeployHarness{fakeSandboxDeployHarness: lc.deploy, errs: []error{layer3ApplyFails(logFilterNotFound)}}
			})
			require.Equal(t, tc.terminal, run.terminalReason(), run.output)

			assert.ElementsMatch(t, tc.sources, learnedSources(t, run, logFilterResource, generator.SandboxDeployLayer))
			raw, err := os.ReadFile(filepath.Join(docsDir, "mock-gaps.md"))
			require.NoError(t, err)
			assert.Contains(t, string(raw), "| "+logFilterResource+" | `resourcenotfoundexception` |")
			assert.Contains(t, runAppLog(t, run), `"event":"`+tc.event+`"`)
		})
	}
}

// A Layer 3 apply that the real cloud refused for its own reasons
// (credentials, throttling, capacity) says nothing about the HCL: no
// learner path writes a pitfall or a mock gap for it, and each logs why.
func TestAWSRunLearnsNothingFromALayer3AccountOrTransientError(t *testing.T) {
	const vpc = "\n\n  with aws_vpc.main,\n  on main.tf line 1, in resource \"aws_vpc\" \"main\":\n   1: resource \"aws_vpc\" \"main\" {"
	failing := "resource \"aws_vpc\" \"main\" {\n  cidr_block = \"10.0.0.0/16\"\n}\n"
	passing := "resource \"aws_vpc\" \"main\" {\n  cidr_block           = \"10.0.0.0/16\"\n  enable_dns_hostnames = true\n}\n"
	for _, tc := range []struct{ name, signal, stderr string }{
		{"AuthFailure", "authfailure", "Error: creating EC2 VPC: operation error EC2: CreateVpc, https response error StatusCode: 401, RequestID: 7a62c49f-347e-4fc4-9331-6e8eEXAMPLE, api error AuthFailure: AWS was not able to validate the provided access credentials" + vpc},
		{"RequestLimitExceeded", "requestlimitexceeded", "Error: creating EC2 VPC: operation error EC2: CreateVpc, https response error StatusCode: 503, RequestID: 7a62c49f-347e-4fc4-9331-6e8eEXAMPLE, api error RequestLimitExceeded: Request limit exceeded." + vpc},
		{"InsufficientInstanceCapacity", "insufficientinstancecapacity", "Error: creating EC2 VPC: operation error EC2: CreateVpc, https response error StatusCode: 500, RequestID: 7a62c49f-347e-4fc4-9331-6e8eEXAMPLE, api error InsufficientInstanceCapacity: We currently do not have sufficient capacity in the Availability Zone you requested." + vpc},
	} {
		for _, path := range []struct {
			name     string
			repairs  int
			terminal string
			events   []string
		}{
			{"self-correction and prescriptive", 2, "target_reached", []string{"self_correction_learning_skipped", "prescriptive_learning_skipped"}},
			{"stuck or budget", 1, "repair_budget_exhausted", []string{"oscillation_learning_skipped"}},
		} {
			t.Run(tc.name+"/"+path.name, func(t *testing.T) {
				run, docsDir := layer3LearningRun(t, path.repairs, []string{failing, passing}, func(lc *awsLifecycle, d *RuntimeDependencies) {
					d.SandboxDeploy = &seqSandboxDeployHarness{fakeSandboxDeployHarness: lc.deploy, errs: []error{layer3ApplyFails(tc.stderr)}}
				})
				require.Equal(t, path.terminal, run.terminalReason(), run.output)

				assert.Empty(t, learnedSources(t, run, "aws_vpc", generator.SandboxDeployLayer))
				assert.NoFileExists(t, filepath.Join(docsDir, "mock-gaps.md"))
				appLog := runAppLog(t, run)
				for _, event := range path.events {
					assert.Contains(t, appLog, `"event":"`+event+`"`)
				}
				assert.Contains(t, appLog, fmt.Sprintf(`signal \"%s\" teaches nothing outside the mock`, tc.signal))
			})
		}
	}
}

// A Layer 3 allowlist refusal at the test step is the HCL's mistake,
// refused before tofu runs, so its stage is learnable: each learner path
// learns the refusal's own gate rule from it, as before this routing. Generation checks the allowlist and runs the gate
// first, so here the allowlist is widened to let aws_nat_gateway through
// and only the gate's test-step calls (the second of each iteration, as
// in runAWSOrder) check the default allowlist, with the real check.
func TestAWSRunLearnsTheGateRuleFromALayer3AllowlistRefusal(t *testing.T) {
	refused := "resource \"aws_nat_gateway\" \"nat\" {\n  subnet_id = \"subnet-0abc\"\n}\n"
	defaults := config.Default().Validation.Layers.SandboxDeploy.AllowResourceTypes
	for _, path := range []struct {
		name     string
		repairs  int
		terminal string
		// events: the prescriptive path's avoid rule for the dropped
		// gateway, which AppendPitfall then files as a duplicate of the
		// gate rule.
		events []string
	}{
		// Iteration 2 drops the gateway: self-correction and prescriptive.
		{"self-correction and prescriptive", 2, "target_reached", []string{"prescriptive_avoid_learned"}},
		{"stuck or budget", 1, "repair_budget_exhausted", nil},
	} {
		t.Run(path.name, func(t *testing.T) {
			run, _ := layer3LearningRun(t, path.repairs, []string{refused, "terraform {}\n"}, func(_ *awsLifecycle, d *RuntimeDependencies) {
				calls := 0
				d.Layer3HCLGate = func(_ layer3Cloud, dir string, _ awsGateInputs) error {
					if calls++; calls%2 == 1 {
						return nil
					}
					return validateLayer3ResourceAllowlist(dir, defaults)
				}
			}, "aws_nat_gateway")
			require.Equal(t, path.terminal, run.terminalReason(), run.output)

			entries, err := generator.LoadPitfallEntries(run.h.PitfallsDir(), "aws")
			require.NoError(t, err)
			i := slices.IndexFunc(entries, func(e generator.PitfallEntry) bool { return strings.HasPrefix(e.Rule, natGatewayRefusal) })
			require.NotEqual(t, -1, i, "no gate rule learned: %+v\n%s", entries, runAppLog(t, run))
			assert.Equal(t, "aws_nat_gateway", entries[i].Resource)
			assert.Equal(t, generator.FixSource, entries[i].Source)
			assert.Equal(t, generator.SandboxDeployLayer, entries[i].LearnedLayer)
			assert.Equal(t, []string{generator.FixSource}, learnedSources(t, run, "aws_nat_gateway", generator.SandboxDeployLayer))
			appLog := runAppLog(t, run)
			for _, event := range path.events {
				assert.Contains(t, appLog, `"event":"`+event+`"`)
			}
		})
	}
}

// The orphan_check extractor reads the mock's state, so it runs only for
// a failure routeFailure lets be learned. The mock's own orphan check is
// one, stamped with its layer; a failure with the same text that the
// router skips (here an unstaged Layer 3 error) says nothing about the
// mock state, so nothing is classified from it.
func TestAWSRunRoutesTheOrphanCheckLikeAnyFailure(t *testing.T) {
	const orphans = "detected 1 orphaned resources"
	for _, tc := range []struct {
		name    string
		fail    func(*awsLifecycle, *RuntimeDependencies)
		learned []string
		event   string
	}{
		{"the mock's orphan check", func(_ *awsLifecycle, d *RuntimeDependencies) {
			d.Destroy = &fakeDestroyHarness{err: &harness.DestroyError{
				Stage: "orphan_check", Destroy: harness.StageResult{Stage: "destroy"}, Err: errors.New(orphans),
			}}
		}, []string{"destruction"}, "orphan_pitfall_learned"},
		{"an orphan failure the router skips", func(lc *awsLifecycle, d *RuntimeDependencies) {
			d.SandboxDeploy = &seqSandboxDeployHarness{fakeSandboxDeployHarness: lc.deploy, errs: []error{errors.New(orphans)}}
		}, nil, "orphan_learning_skipped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run, _ := layer3LearningRun(t, 1, []string{"terraform {}\n"}, func(lc *awsLifecycle, d *RuntimeDependencies) {
				// A soft-deleted secret left in the mock, which the
				// extractor classifies as an LLM-side soft delete.
				d.MockState = &fakeRunMockStateClient{statePayload: []byte(`{"secretsmanager":{"secrets":[{"name":"app"}]}}`)}
				tc.fail(lc, d)
			})
			require.Equal(t, "repair_budget_exhausted", run.terminalReason(), run.output)

			entries, err := generator.LoadPitfallEntries(run.h.PitfallsDir(), "aws")
			require.NoError(t, err)
			var layers []string
			for _, e := range entries {
				if e.Resource == "aws_secretsmanager_secret" {
					layers = append(layers, e.LearnedLayer)
				}
			}
			assert.Equal(t, tc.learned, layers, "%+v", entries)
			assert.Contains(t, runAppLog(t, run), `"event":"`+tc.event+`"`)
		})
	}
}

// routeFailureRows are failures and where routeFailure sends them, with
// Layer 3 on unless a row says off.
var routeFailureRows = map[string]struct {
	f          feedback.Failure
	layer3Off  bool
	want       failureRoute
	reasonPart string
}{
	"a mock signal from the mock": {
		f: feedback.Failure{Layer: "run", Origin: "mock_deploy", Detail: logFilterNotFound}, want: routeMockGap},
	"a mock signal from the static plan": {
		f: feedback.Failure{Origin: "static", Detail: "DescribeVpcs: not modelled by fakeaws: foo"}, want: routeMockGap},
	"a mock layer's wait timeout is its own lesson": {
		f: feedback.Failure{Origin: "mock_deploy", Detail: "timeout while waiting for state to become 'running'"}, want: routeLearn},
	"the GCP escape carve-out at the mock": {
		f: feedback.Failure{Origin: "mock_deploy", Detail: `google_project_service.compute: Error 401: "reason": "ACCESS_TOKEN_TYPE_UNSUPPORTED"`}, want: routeLearn},
	"a mock signal with no Origin, Layer 3 off": {
		f: feedback.Failure{Layer: "run", Detail: logFilterNotFound}, layer3Off: true, want: routeMockGap},
	"a mock signal with no Origin, Layer 3 on": {
		f: feedback.Failure{Layer: "run", Detail: logFilterNotFound}, want: routeLearn},
	"a mock-only signal with no Origin, Layer 3 on": {
		f: feedback.Failure{Layer: "run", Detail: "Error: Plugin did not respond"}, want: routeSkip, reasonPart: `signal "plugin did not respond"`},
	"an HCL error with no Origin, Layer 3 on": {
		f: feedback.Failure{Layer: "run", Detail: `Error: Unsupported argument on aws_vpc.main: An argument named "bogus" is not expected here.`}, want: routeLearn},
	"a real-cloud signal from a Layer 3 apply": {
		f: feedback.Failure{Layer: "run", Origin: "sandbox_deploy", OriginStage: "apply", Detail: logFilterNotFound}, want: routeLearn},
	"a wait that never saw the resource at a Layer 3 apply": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "apply", Detail: "waiting for RDS DB Instance (db) create: couldn't find resource (21 retries)"}, want: routeLearn},
	"a wait-for-state timeout at a Layer 3 apply": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "apply", Detail: "waiting for EC2 NAT Gateway create: timeout while waiting for state to become 'available'"}, want: routeLearn},
	"a real probe failure": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "real_probe", Detail: "http_probe compute:80: connection refused"}, want: routeLearn},
	"a mock-only signal at a Layer 3 apply": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "apply", Detail: "DescribeVpcs: not modelled by fakeaws: foo"}, want: routeSkip, reasonPart: `signal "not modelled by fakeaws"`},
	"the AMI resolve": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "aws_ami_resolve", Check: "ami", Detail: "aws_instance AMI parameter unreadable"}, want: routeSkip, reasonPart: `stage "aws_ami_resolve" is setup`},
	"a cleanup the allowlist blocked": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "cleanup_blocked", Check: "no_orphans", Detail: "the configuration was refused, so no tofu was run"}, want: routeSkip, reasonPart: `stage "cleanup_blocked" is setup`},
	"an allowlist refusal teaches its gate rule": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "allowlist", Check: "allow_resource_types", Detail: natGatewayRefusal}, want: routeLearn},
	"a gate refusal that names no resource type": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "allowlist", Detail: "layer 3 refuses this configuration: main.tf: a provider block is not allowed"}, want: routeLearn},
	"a credentials preflight": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "preflight", Check: "credentials", Detail: "AccessDenied"}, want: routeSkip, reasonPart: `stage "preflight" is setup`},
	"an aws scope stage": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "aws_scope_sweep", Detail: "aws_instance i-0abc left"}, want: routeSkip, reasonPart: `stage "aws_scope_sweep" is setup`},
	"a Layer 3 error that names no stage": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "run", Detail: logFilterNotFound}, want: routeSkip, reasonPart: `stage "run" is setup`},
	"an authentication failure": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "apply", Detail: "api error AuthFailure: AWS was not able to validate the provided access credentials"}, want: routeSkip, reasonPart: `signal "authfailure"`},
	"throttling": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "apply", Detail: "api error RequestLimitExceeded: Request limit exceeded."}, want: routeSkip, reasonPart: `signal "requestlimitexceeded"`},
	"no capacity": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "apply", Detail: "api error InsufficientInstanceCapacity: We currently do not have sufficient capacity"}, want: routeSkip, reasonPart: `signal "insufficientinstancecapacity"`},
	"a vCPU limit": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "apply", Detail: "api error VcpuLimitExceeded: You have requested more vCPU capacity than your current vCPU limit"}, want: routeSkip, reasonPart: `signal "vcpulimitexceeded"`},
	"an unauthorized operation": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "apply", Detail: "api error UnauthorizedOperation: You are not authorized to perform this operation."}, want: routeSkip, reasonPart: `signal "unauthorizedoperation"`},
	"a Scaleway permission refusal": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "apply", Detail: "scaleway-sdk-go: insufficient permissions: write instance"}, want: routeSkip, reasonPart: `signal "insufficient permissions"`},
	"a Scaleway quota": {
		f: feedback.Failure{Origin: "sandbox_deploy", OriginStage: "apply", Detail: "scaleway-sdk-go: quota exceeded(s): instances_pro2_s_count"}, want: routeSkip, reasonPart: `signal "quota exceeded"`},
	"a plugin crash from the holdout probe": {
		f: feedback.Failure{Origin: "holdout", Detail: "The plugin encountered an error, and failed to respond"}, want: routeSkip, reasonPart: `signal "the plugin encountered an error"`},
}

func TestRouteFailure(t *testing.T) {
	for name, tc := range routeFailureRows {
		t.Run(name, func(t *testing.T) {
			got, reason := routeFailure(tc.f, !tc.layer3Off)
			assert.Equal(t, tc.want, got)
			assert.Contains(t, reason, tc.reasonPart)
		})
	}
}

// What the router lets the extractors learn, under either Layer 3
// setting, the pitfalls ratchet admits at the layer it is stamped with.
func TestRoutedLessonsPassThePitfallsRatchet(t *testing.T) {
	for name, tc := range routeFailureRows {
		for _, layer3 := range []bool{false, true} {
			if route, _ := routeFailure(tc.f, layer3); route != routeLearn {
				continue
			}
			assert.Empty(t, generator.ForbiddenPitfallSignal(failureLayer(tc.f), tc.f.Detail), "%s, layer3=%t", name, layer3)
		}
	}
}

// The repair prompt shows a detail once, whichever layer repeated it.
func TestAppendDistinctFailuresIgnoresTheLayer(t *testing.T) {
	mock := FailureSummary{Layer: "run", Origin: "mock_deploy", Detail: logFilterNotFound}
	layer3 := FailureSummary{Layer: "run", Origin: "sandbox_deploy", Detail: logFilterNotFound}

	assert.Equal(t, []FailureSummary{mock}, appendDistinctFailures(nil, []FailureSummary{mock, layer3}))
}

// A mock whose reset fails says the environment is wrong, not the HCL:
// the run stops on iteration 1, names the mock, and learns nothing.
func TestRunCommandStopsWhenAMockResetFails(t *testing.T) {
	fakeaws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	defer fakeaws.Close()
	// Another service on the s3 backend's port answers HTML.
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html><body>not s3</body></html>")
	}))
	defer s3.Close()

	h := newCommandTestHarness(t)
	scenarioPath := filepath.Join(h.WorkspaceDir, "scenarios", "training", "aws.yaml")
	mustWriteFile(t, scenarioPath, `scenario: example-scenario
version: "1.0"
cloud: aws
description: example
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: connectivity
    from: public_internet
    to: compute
    expect: blocked
`)
	docsDir := t.TempDir()
	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 4
		cfg.Paths.Docs = docsDir
		return cfg
	})
	generations := 0
	opts.deps = RuntimeDependencies{
		// The real router and mock deploy, so the reset is the one a run makes.
		MockState: &cloudMockStateRouter{
			runtime:     &CommandRuntime{loadedScenario: &scenario.Scenario{Cloud: "aws"}},
			scaleway:    newMockStateClient(closedMockURL),
			aws:         newMockStateClient(fakeaws.URL),
			s3:          newMockStateClient(s3.URL),
			s3AutoReset: true,
		},
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			generations++
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static: &fakeStaticHarness{result: &harness.StaticResult{
			Stages:   []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}},
			PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`),
		}},
		Destroy: &fakeDestroyHarness{},
	}

	cmd := newRunCommandForTest(opts)
	cmd.RunE = withRuntimeWithOptions("run", opts, sealedHandler(io.Discard, runRunCommand))
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `terminal reason "mock_unavailable" after 1 iteration(s)`)
	assert.Equal(t, 1, generations, "a reset failure must not spend a repair generation")
	assert.Contains(t, stdout.String(), "s3 mock at "+s3.URL)

	entries, err := generator.LoadPitfallEntries(h.PitfallsDir(), "aws")
	require.NoError(t, err)
	assert.Empty(t, entries)
	assert.NoFileExists(t, filepath.Join(docsDir, "mock-gaps.md"))
}

// A failed real Layer 3 stage on aws stays a sandbox_deploy failure in
// iteration.json, beside its numbered stage line, so evidence readers can
// tell it from a mock one; every carried fail line names a failure, and
// every carried failure a line. The run's stage line stays
// run/iteration_1_test, and an error that names no stage of its own
// stays there too.
func TestRunRecordsAFailedLayer3StageAtItsLayerInIterationJSON(t *testing.T) {
	const detail = "InvalidSubnet.Range"
	deployFails := func(stage string) func(*awsLifecycle) {
		return func(lc *awsLifecycle) {
			lc.deploy.err = &harness.SandboxDeployError{Stage: stage, Err: errors.New(detail)}
		}
	}
	rows := map[string]struct {
		lc    func(*awsLifecycle)
		setup orderSetup
		// stage and detail name the failure; stage "" is the run's own
		// iteration_1_test.
		stage, detail string
	}{
		"preflight": {setup: orderSetup{stsFailsOn: 2}, stage: "iteration_1_preflight", detail: "AccessDenied"},
		"allowlist": {setup: orderSetup{gateFailsOn: 2}, stage: "iteration_1_allowlist", detail: "refused by the gate's fake"},
		"init":      {lc: deployFails("init"), stage: "iteration_1_init", detail: detail},
		"plan":      {lc: deployFails("plan"), stage: "iteration_1_plan", detail: detail},
		"apply":     {lc: deployFails("apply"), stage: "iteration_1_apply", detail: detail},
		"destroy": {lc: func(lc *awsLifecycle) {
			lc.destroy.err = &harness.SandboxDestroyError{Stage: "destroy", Err: errors.New(detail)}
		}, stage: "iteration_1_destroy", detail: detail},
		"destroy (no stage)": {lc: func(lc *awsLifecycle) { lc.destroy.err = errors.New(detail) }, detail: detail},
		"deploy (no stage)":  {lc: func(lc *awsLifecycle) { lc.deploy.err = errors.New(detail) }, detail: detail},
	}
	for name, row := range rows {
		t.Run(name, func(t *testing.T) {
			lc := newAWSLifecycle(t)
			if row.lc != nil {
				row.lc(lc)
			}

			run := runAWSOrder(t, lc, row.setup)

			require.Error(t, run.err)
			paths, err := filepath.Glob(filepath.Join(run.h.RunstoreRoot(), "*", "*", "iterations", "1", "iteration.json"))
			require.NoError(t, err)
			require.Len(t, paths, 1)
			raw, err := os.ReadFile(paths[0])
			require.NoError(t, err)
			var iteration struct {
				Stages   []StageSummary   `json:"stages"`
				Failures []FailureSummary `json:"failures"`
			}
			require.NoError(t, json.Unmarshal(raw, &iteration))
			assert.Contains(t, iteration.Stages, StageSummary{Layer: "run", Stage: "iteration_1_test", Status: StageStatusFail})
			i := slices.IndexFunc(iteration.Failures, func(f FailureSummary) bool { return strings.Contains(f.Detail, row.detail) })
			require.NotEqual(t, -1, i, "%+v", iteration.Failures)
			got := iteration.Failures[i]
			if row.stage == "" {
				assert.Equal(t, "run", got.Layer)
				assert.Equal(t, "iteration_1_test", got.Stage)
			} else {
				assert.Equal(t, "sandbox_deploy", got.Layer)
				assert.Equal(t, row.stage, got.Stage)
				assert.Contains(t, iteration.Stages, StageSummary{Layer: "sandbox_deploy", Stage: row.stage, Status: StageStatusFail})
			}
			for _, s := range iteration.Stages {
				if s.Layer == "sandbox_deploy" && s.Status == StageStatusFail {
					assert.True(t, slices.ContainsFunc(iteration.Failures, func(f FailureSummary) bool { return f.Layer == s.Layer && f.Stage == s.Stage }),
						"fail line %s/%s names no failure: %+v", s.Layer, s.Stage, iteration.Failures)
				}
			}
			for _, f := range iteration.Failures {
				if f.Layer == "sandbox_deploy" {
					assert.True(t, slices.ContainsFunc(iteration.Stages, func(s StageSummary) bool { return s.Layer == f.Layer && s.Stage == f.Stage }),
						"failure %s/%s names no stage line: %+v", f.Layer, f.Stage, iteration.Stages)
				}
			}
		})
	}
}

// Scaleway is outside the rule: its run carries no sandbox stage up and
// keeps its Layer 3 failures at run/iteration_N_test. On aws every
// sandbox_deploy stage but the "run" wrap is carried, numbered.
func TestRunStageNameCarriesNoScalewaySandboxStage(t *testing.T) {
	for _, stage := range []string{"preflight", "allowlist", "init", "plan", "apply", "real_probe", "destroy", "cleanup_blocked"} {
		_, ok := runStageName("sandbox_deploy", stage, 1, layer3Scaleway)
		assert.False(t, ok, stage)
		name, ok := runStageName("sandbox_deploy", stage, 1, layer3AWS)
		assert.True(t, ok, stage)
		assert.Equal(t, "iteration_1_"+stage, name)
	}
	_, ok := runStageName("sandbox_deploy", "run", 1, layer3AWS)
	assert.False(t, ok, "the run wrap names no stage")
	for _, stage := range []string{"aws_scope_sweep", "aws_scope_release"} {
		name, ok := runStageName("sandbox_deploy", stage, 1, layer3AWS)
		assert.True(t, ok, stage)
		assert.Equal(t, stage, name, "a scope stage keeps its name")
	}
}

// A failure is filed beside its stage's carried fail line only: a
// same-named pass line, or none, leaves it at run/iteration_N_test.
func TestRunFailureStageNeedsTheStagesFailLine(t *testing.T) {
	f := FailureSummary{Layer: "sandbox_deploy", Stage: "apply", Check: "apply"}
	for name, tc := range map[string]struct {
		stages []StageSummary
		want   bool
	}{
		"fail line": {stages: []StageSummary{{Layer: "sandbox_deploy", Stage: "apply", Status: StageStatusFail}}, want: true},
		"pass line": {stages: []StageSummary{{Layer: "sandbox_deploy", Stage: "apply", Status: StageStatusPass}}},
		"no line":   {},
	} {
		t.Run(name, func(t *testing.T) {
			_, failLines := awsRunStages(tc.stages, 1, layer3AWS)
			got, ok := runFailureStage(f, failLines)
			assert.Equal(t, tc.want, ok)
			if tc.want {
				assert.Equal(t, "iteration_1_apply", got)
			}
		})
	}
}
