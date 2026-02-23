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
	"github.com/spf13/cobra"
)

type statusFixture struct {
	Success map[string]string `json:"success"`
	Failure map[string]string `json:"failure"`
}

func TestCLIOrchestrationIntegrationSuccess(t *testing.T) {
	h := newCommandTestHarness(t)
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", filepath.Join(h.WorkspaceDir, ".infrafactory", "runs"))

	fixture := loadStatusFixture(t)
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Paths.Output = filepath.Join(h.WorkspaceDir, "output")
			cfg.Agent.RepairIterationsMax = 2
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
			}),
			Static:     &fakeStaticHarness{result: &harness.StaticResult{Stages: []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}}, PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`)}},
			MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{Apply: harness.StageResult{Stage: "apply"}, StateSnapshot: []byte(`{}`)}},
			Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{Destroy: harness.StageResult{Stage: "destroy"}, OrphanCount: 0}},
			MockStart:  &fakeMockStarter{},
		},
	}

	commands := map[string]*cobra.Command{
		"generate":   newGenerateCommandForTest(opts),
		"validate":   newValidateCommandForTest(opts),
		"test":       newTestCommandForTest(opts),
		"run":        newRunCommandForTest(opts),
		"mock start": newMockStartCommandForTest(opts),
	}

	for name, cmd := range commands {
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		if name == "mock start" {
			cmd.SetArgs([]string{"--config", h.ConfigPath})
		} else {
			cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})
		}

		if err := cmd.Execute(); err != nil {
			t.Fatalf("%s should succeed, got error: %v", name, err)
		}
		expectedStatus := fixture.Success[name]
		if !strings.Contains(stdout.String(), "Status: "+expectedStatus) {
			t.Fatalf("%s expected status %q, got:\n%s", name, expectedStatus, stdout.String())
		}
	}
}

func TestCLIOrchestrationIntegrationFailureRegression(t *testing.T) {
	h := newCommandTestHarness(t)
	t.Setenv("INFRAFACTORY_RUNSTORE_ROOT", filepath.Join(h.WorkspaceDir, ".infrafactory", "runs"))

	fixture := loadStatusFixture(t)
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Paths.Output = filepath.Join(h.WorkspaceDir, "output")
			cfg.Agent.RepairIterationsMax = 2
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
			}),
			Static:     &fakeStaticHarness{err: &harness.StageError{StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}}, Err: errors.New("validate failed")}},
			MockDeploy: &fakeMockDeployHarness{},
			Destroy:    &fakeDestroyHarness{},
			MockStart:  &fakeMockStarter{},
		},
	}

	cases := map[string]*cobra.Command{
		"validate": newValidateCommandForTest(opts),
		"run":      newRunCommandForTest(opts),
	}
	for name, cmd := range cases {
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

		err := cmd.Execute()
		if err == nil {
			t.Fatalf("%s should fail", name)
		}
		expectedStatus := fixture.Failure[name]
		if !strings.Contains(stdout.String(), "Status: "+expectedStatus) {
			t.Fatalf("%s expected status %q, got:\n%s", name, expectedStatus, stdout.String())
		}
	}
}

func loadStatusFixture(t *testing.T) statusFixture {
	t.Helper()

	path := filepath.Join("testdata", "integration", "expected_command_statuses.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read status fixture: %v", err)
	}

	var fixture statusFixture
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatalf("decode status fixture: %v", err)
	}

	return fixture
}
