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
	"github.com/spf13/cobra"
)

func TestGenerateCommandWritesFilesDeterministically(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	outputRoot := filepath.Join(h.WorkspaceDir, "output")

	gen := generator.SeedGeneratorFunc(func(_ context.Context, req generator.Request) (*generator.GeneratedCode, error) {
		if req.ScenarioPath != h.ScenarioPath {
			t.Fatalf("unexpected scenario path: %s", req.ScenarioPath)
		}
		if req.Iteration != 1 {
			t.Fatalf("expected iteration 1, got %d", req.Iteration)
		}
		return &generator.GeneratedCode{
			Files: map[string][]byte{
				"main.tf":         []byte("terraform {}\n"),
				"modules/vpc.tf":  []byte("resource \"x\" \"y\" {}\n"),
				"outputs/main.tf": []byte("output \"id\" {}\n"),
			},
		}, nil
	})

	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Paths.Output = outputRoot
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: gen,
		},
	}

	cmd := newGenerateCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute generate: %v", err)
	}

	expectedFiles := map[string]string{
		filepath.Join(outputRoot, "example-scenario", "main.tf"):            "terraform {}\n",
		filepath.Join(outputRoot, "example-scenario", "modules", "vpc.tf"):  "resource \"x\" \"y\" {}\n",
		filepath.Join(outputRoot, "example-scenario", "outputs", "main.tf"): "output \"id\" {}\n",
	}
	for path, expected := range expectedFiles {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected generated file %q: %v", path, err)
		}
		if string(payload) != expected {
			t.Fatalf("unexpected file %q content\nexpected:\n%s\nactual:\n%s", path, expected, string(payload))
		}
	}

	output := stdout.String()
	checks := []string{
		"Command: generate",
		"Scenario: example-scenario",
		"Status: success",
		"- generate/seed: pass",
		"- generate/write_files: pass (3 files)",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("expected output to contain %q, got:\n%s", check, output)
		}
	}
}

func TestGenerateCommandReturnsGeneratorFailure(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	genErr := errors.New("transport unavailable")

	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return nil, genErr
			}),
		},
	}

	cmd := newGenerateCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, genErr) {
		t.Fatalf("expected wrapped generator error %v, got: %v", genErr, err)
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *CLIError, got %T (%v)", err, err)
	}
	if cliErr.Op != "generate" || cliErr.Code != errorCodeCommandFailed {
		t.Fatalf("expected generate/%s CLI error, got op=%q code=%q", errorCodeCommandFailed, cliErr.Op, cliErr.Code)
	}
}

func TestGenerateCommandSupportsJSONOutputMode(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	outputRoot := filepath.Join(h.WorkspaceDir, "output")

	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Paths.Output = outputRoot
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{Files: map[string][]byte{
					"main.tf": []byte("terraform {}\n"),
				}}, nil
			}),
		},
	}

	cmd := newGenerateCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath, "--output", string(OutputModeJSON)})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute generate json mode: %v", err)
	}
	if !strings.Contains(stdout.String(), "\"schema\": \""+OutputSchemaVersion+"\"") {
		t.Fatalf("expected machine json schema in output, got:\n%s", stdout.String())
	}
}

func TestGenerateCommandAutoAddsScalewayProviderWiringWhenMissing(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)
	outputRoot := filepath.Join(h.WorkspaceDir, "output")

	opts := runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Paths.Output = outputRoot
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{
					Files: map[string][]byte{
						"compute.tf": []byte(`resource "scaleway_instance_server" "web_1" { name = "web-1" }`),
					},
				}, nil
			}),
		},
	}

	cmd := newGenerateCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute generate: %v", err)
	}

	providersPath := filepath.Join(outputRoot, "example-scenario", "providers.tf")
	providers, err := os.ReadFile(providersPath)
	if err != nil {
		t.Fatalf("expected generated provider file %q: %v", providersPath, err)
	}
	providersContent := string(providers)
	if !strings.Contains(providersContent, "required_providers") || !strings.Contains(providersContent, "scaleway/scaleway") {
		t.Fatalf("expected required_providers.scaleway wiring, got:\n%s", providersContent)
	}
	if !strings.Contains(providersContent, `provider "scaleway"`) {
		t.Fatalf("expected provider block wiring, got:\n%s", providersContent)
	}
	if !strings.Contains(stdout.String(), "Status: success") {
		t.Fatalf("expected success output, got:\n%s", stdout.String())
	}
}

func TestGenerateCommandDefaultRuntimeUsesConcreteGeneratorDependency(t *testing.T) {
	t.Parallel()

	h := newCommandTestHarness(t)

	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
	}

	cmd := newGenerateCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected concrete generator failure, got not implemented: %v", err)
	}
	if strings.Contains(err.Error(), "default seed generator for agent type") {
		t.Fatalf("expected concrete adapter path (not stub), got: %v", err)
	}
	if !strings.Contains(err.Error(), "prompt render failed") {
		t.Fatalf("expected concrete adapter failure detail, got: %v", err)
	}
}

func TestGenerateCommandOpenRouterMissingAPIKeyReturnsDependencyUnavailable(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	h := newCommandTestHarness(t)

	opts := runtimeOptions{
		configLoader: func(_ string) (config.Config, error) {
			cfg := config.Default()
			cfg.Agent.Type = generator.AgentTypeOpenRouter
			cfg.Agent.OpenRouter.Model = "anthropic/claude-3.5-sonnet"
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
	}

	cmd := newGenerateCommandForTest(opts)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T (%v)", err, err)
	}
	if cliErr.Code != errorCodeDependencyUnavailable {
		t.Fatalf("expected dependency_unavailable code, got %q (%v)", cliErr.Code, err)
	}
}

func newGenerateCommandForTest(opts runtimeOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "generate <scenario>",
		Args: requireScenarioArg,
		RunE: withRuntimeWithOptions("generate", opts, runGenerateCommand),
	}
	cmd.Flags().String("config", config.DefaultPath, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	return cmd
}
