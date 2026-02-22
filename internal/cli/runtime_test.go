package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/scenario"
	"github.com/spf13/cobra"
)

type stubMockStateClient struct{}

func (stubMockStateClient) Reset(context.Context) error { return nil }
func (stubMockStateClient) State(context.Context) ([]byte, error) {
	return []byte("{}"), nil
}

func TestWithRuntimeLoadsConfigAndInjectsDependencies(t *testing.T) {
	t.Parallel()

	var configLoadCalls int
	deps := RuntimeDependencies{MockState: stubMockStateClient{}}
	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			configLoadCalls++
			if path != "test-config.yaml" {
				t.Fatalf("unexpected config path: %s", path)
			}
			cfg := config.Default()
			cfg.Paths.Output = "./generated-output"
			return cfg, nil
		},
		scenarioLoader: func(path string) (scenario.Scenario, error) {
			return scenario.Scenario{Name: "example"}, nil
		},
		deps: deps,
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	if err := cmd.Flags().Set("config", "test-config.yaml"); err != nil {
		t.Fatalf("set config flag: %v", err)
	}
	cmd.SetOut(&bytes.Buffer{})

	handler := withRuntimeWithOptions("generate", opts, func(_ *cobra.Command, _ []string, runtime *CommandRuntime) error {
		if runtime.ConfigPath != "test-config.yaml" {
			t.Fatalf("unexpected runtime config path: %s", runtime.ConfigPath)
		}
		if runtime.Config.Paths.Output != "./generated-output" {
			t.Fatalf("unexpected runtime output root: %s", runtime.Config.Paths.Output)
		}
		if runtime.Deps.Generator == nil {
			t.Fatal("expected default generator dependency to be injected")
		}
		if runtime.Deps.MockState == nil {
			t.Fatal("expected mock state dependency to be injected")
		}
		return nil
	})

	if err := handler(cmd, nil); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if configLoadCalls != 1 {
		t.Fatalf("expected one config load, got %d", configLoadCalls)
	}
}

func TestBuildRuntimeRespectsInjectedGeneratorDependency(t *testing.T) {
	t.Parallel()

	customGenerator := generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
		return &generator.GeneratedCode{
			Files: map[string][]byte{"main.tf": []byte("terraform {}\n")},
		}, nil
	})

	opts := runtimeOptions{
		configLoader: func(string) (config.Config, error) {
			return config.Default(), nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: customGenerator,
		},
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	if err := cmd.Flags().Set("config", "test-config.yaml"); err != nil {
		t.Fatalf("set config flag: %v", err)
	}

	runtime, err := buildRuntime(cmd, opts)
	if err != nil {
		t.Fatalf("build runtime: %v", err)
	}

	out, err := runtime.Deps.Generator.Generate(context.Background(), generator.Request{Iteration: 1})
	if err != nil {
		t.Fatalf("expected injected generator to be used, got: %v", err)
	}
	if out == nil || len(out.Files) != 1 {
		t.Fatalf("expected generated file output from injected generator, got: %#v", out)
	}
}

func TestCommandRuntimeLoadScenarioCachesResultAndOutputDir(t *testing.T) {
	t.Parallel()

	var scenarioLoadCalls int
	opts := runtimeOptions{
		configLoader: func(string) (config.Config, error) {
			cfg := config.Default()
			cfg.Paths.Output = "/tmp/out"
			return cfg, nil
		},
		scenarioLoader: func(path string) (scenario.Scenario, error) {
			scenarioLoadCalls++
			if path != "scenarios/training/a.yaml" {
				t.Fatalf("unexpected scenario path: %s", path)
			}
			return scenario.Scenario{Name: "web-app"}, nil
		},
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	if err := cmd.Flags().Set("config", "test-config.yaml"); err != nil {
		t.Fatalf("set config flag: %v", err)
	}

	runtime, err := buildRuntime(cmd, opts)
	if err != nil {
		t.Fatalf("build runtime: %v", err)
	}

	first, err := runtime.LoadScenario("scenarios/training/a.yaml")
	if err != nil {
		t.Fatalf("first load scenario: %v", err)
	}
	second, err := runtime.LoadScenario("scenarios/training/a.yaml")
	if err != nil {
		t.Fatalf("second load scenario: %v", err)
	}
	if first.Name != second.Name {
		t.Fatalf("expected cached scenario name %q, got %q", first.Name, second.Name)
	}
	if scenarioLoadCalls != 1 {
		t.Fatalf("expected one scenario load, got %d", scenarioLoadCalls)
	}
	if runtime.OutputDir() != filepath.Join("/tmp/out", "web-app") {
		t.Fatalf("unexpected output dir: %s", runtime.OutputDir())
	}

	_, err = runtime.LoadScenario("scenarios/training/other.yaml")
	if err == nil {
		t.Fatal("expected error when loading a different scenario path after cache")
	}
	if !strings.Contains(err.Error(), "scenario already loaded") {
		t.Fatalf("expected already-loaded scenario error, got: %v", err)
	}
}

func TestFormatCommandErrorClassifiesKnownErrorKinds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		err          error
		expectedCode string
		expectedIs   error
	}{
		{
			name:         "config invalid",
			err:          fmt.Errorf("load config: %w", &config.ValidationError{Fields: []config.FieldError{{Field: "version", Err: "is required"}}}),
			expectedCode: "config_invalid",
			expectedIs:   config.ErrInvalidConfig,
		},
		{
			name:         "scenario malformed",
			err:          fmt.Errorf("load scenario: %w", scenario.ErrMalformedScenario),
			expectedCode: "scenario_malformed",
			expectedIs:   scenario.ErrMalformedScenario,
		},
		{
			name:         "scenario invalid",
			err:          fmt.Errorf("load scenario: %w", scenario.ErrInvalidScenario),
			expectedCode: "scenario_invalid",
			expectedIs:   scenario.ErrInvalidScenario,
		},
		{
			name:         "fallback",
			err:          errors.New("boom"),
			expectedCode: "command_failed",
			expectedIs:   nil,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			formatted := formatCommandError("validate", tc.err)
			cliErr := &CLIError{}
			if !errors.As(formatted, &cliErr) {
				t.Fatalf("expected *CLIError, got %T (%v)", formatted, formatted)
			}
			if cliErr.Op != "validate" {
				t.Fatalf("expected op validate, got %s", cliErr.Op)
			}
			if cliErr.Code != tc.expectedCode {
				t.Fatalf("expected code %q, got %q", tc.expectedCode, cliErr.Code)
			}
			if tc.expectedIs != nil && !errors.Is(formatted, tc.expectedIs) {
				t.Fatalf("expected formatted error to wrap %v, got %v", tc.expectedIs, formatted)
			}
		})
	}
}

func TestFormatCommandErrorPassThroughPaths(t *testing.T) {
	t.Parallel()

	notImpl := fmt.Errorf("generate: %w", ErrNotImplemented)
	formattedNotImpl := formatCommandError("generate", notImpl)
	if formattedNotImpl != notImpl {
		t.Fatalf("expected not implemented error to pass through unchanged")
	}

	existing := &CLIError{Op: "run", Code: "command_failed", Err: errors.New("existing")}
	formattedExisting := formatCommandError("run", existing)
	if formattedExisting != existing {
		t.Fatalf("expected existing CLI error to pass through unchanged")
	}
}
