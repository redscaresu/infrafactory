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
	if !strings.Contains(stdout.String(), "default seed generator for agent type") {
		t.Fatalf("expected default generator failure detail, got:\n%s", stdout.String())
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
