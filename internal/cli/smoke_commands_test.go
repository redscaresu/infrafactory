package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
)

func TestCommandOrchestrationSmokeGenerate(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		h := newCommandTestHarness(t)
		opts := runtimeOptions{
			configLoader: func(path string) (config.Config, error) {
				cfg, err := config.Load(path)
				if err != nil {
					return config.Config{}, err
				}
				cfg.Paths.Output = filepath.Join(h.WorkspaceDir, "output")
				return cfg, nil
			},
			scenarioLoader: defaultScenarioLoader,
			deps: RuntimeDependencies{Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
			})},
		}
		cmd := newGenerateCommandForTest(opts)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("expected generate success, got: %v", err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		t.Parallel()
		h := newCommandTestHarness(t)
		opts := runtimeOptions{
			configLoader:   config.Load,
			scenarioLoader: defaultScenarioLoader,
			deps: RuntimeDependencies{Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return nil, errors.New("generator failed")
			})},
		}
		cmd := newGenerateCommandForTest(opts)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})
		if err := cmd.Execute(); err == nil {
			t.Fatal("expected generate failure")
		}
	})
}

func TestCommandOrchestrationSmokeValidate(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		h := newCommandTestHarness(t)
		opts := runtimeOptions{
			configLoader:   config.Load,
			scenarioLoader: defaultScenarioLoader,
			deps:           RuntimeDependencies{Static: &fakeStaticHarness{result: &harness.StaticResult{Stages: []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}}, PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`)}}},
		}
		cmd := newValidateCommandForTest(opts)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("expected validate success, got: %v", err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		t.Parallel()
		h := newCommandTestHarness(t)
		opts := runtimeOptions{
			configLoader:   config.Load,
			scenarioLoader: defaultScenarioLoader,
			deps:           RuntimeDependencies{Static: &fakeStaticHarness{result: &harness.StaticResult{Stages: []harness.StageResult{{Stage: "init"}, {Stage: "validate"}}}, err: &harness.StageError{StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}}, Err: errors.New("validate failed")}}},
		}
		cmd := newValidateCommandForTest(opts)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})
		if err := cmd.Execute(); err == nil {
			t.Fatal("expected validate failure")
		}
	})
}

func TestCommandOrchestrationSmokeTest(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		h := newCommandTestHarness(t)
		opts := runtimeOptions{
			configLoader:   config.Load,
			scenarioLoader: defaultScenarioLoader,
			deps: RuntimeDependencies{
				MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`)}},
				Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{Destroy: harness.StageResult{Stage: "destroy"}, OrphanCount: 0}},
			},
		}
		cmd := newTestCommandForTest(opts)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("expected test success, got: %v", err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		t.Parallel()
		h := newCommandTestHarness(t)
		opts := runtimeOptions{
			configLoader:   config.Load,
			scenarioLoader: defaultScenarioLoader,
			deps: RuntimeDependencies{
				MockDeploy: &fakeMockDeployHarness{err: &harness.MockDeployError{Stage: "apply", Err: errors.New("apply failed")}},
				Destroy:    &fakeDestroyHarness{},
			},
		}
		cmd := newTestCommandForTest(opts)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})
		if err := cmd.Execute(); err == nil {
			t.Fatal("expected test failure")
		}
	})
}
