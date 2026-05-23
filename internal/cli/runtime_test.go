package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
	"github.com/spf13/cobra"
)

type stubMockStateClient struct{}

func (stubMockStateClient) Reset(context.Context) error    { return nil }
func (stubMockStateClient) Snapshot(context.Context) error { return nil }
func (stubMockStateClient) Restore(context.Context) error  { return nil }
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
			cfg.Agent.Type = generator.AgentTypeClaudeCode
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
		if runtime.TransportContract.AgentType != generator.AgentTypeClaudeCode {
			t.Fatalf("expected claude transport contract, got %q", runtime.TransportContract.AgentType)
		}
		if runtime.Deps.Generator == nil {
			t.Fatal("expected default generator dependency to be injected")
		}
		if runtime.Deps.MockState == nil {
			t.Fatal("expected mock state dependency to be injected")
		}
		if runtime.Logger == nil {
			t.Fatal("expected runtime logger to be injected")
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

func TestBuildRuntimeMapsOpenRouterTransportContract(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	opts := runtimeOptions{
		configLoader: func(string) (config.Config, error) {
			cfg := config.Default()
			cfg.Agent.Type = generator.AgentTypeOpenRouter
			cfg.Agent.OpenRouter.Model = "anthropic/claude-3.5-sonnet"
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
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

	if runtime.TransportContract.AgentType != generator.AgentTypeOpenRouter {
		t.Fatalf("expected openrouter transport contract, got %q", runtime.TransportContract.AgentType)
	}
	if len(runtime.TransportContract.RequiredEnv) != 1 || runtime.TransportContract.RequiredEnv[0] != "OPENROUTER_API_KEY" {
		t.Fatalf("unexpected required env vars: %+v", runtime.TransportContract.RequiredEnv)
	}
}

func TestBuildRuntimeOpenRouterWithoutAPIKeyFails(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	opts := runtimeOptions{
		configLoader: func(string) (config.Config, error) {
			cfg := config.Default()
			cfg.Agent.Type = generator.AgentTypeOpenRouter
			cfg.Agent.OpenRouter.Model = "anthropic/claude-3.5-sonnet"
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	if err := cmd.Flags().Set("config", "test-config.yaml"); err != nil {
		t.Fatalf("set config flag: %v", err)
	}

	_, err := buildRuntime(cmd, opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable error, got %v", err)
	}
}

func TestBuildRuntimeUnknownTransportContractFails(t *testing.T) {
	t.Parallel()

	opts := runtimeOptions{
		configLoader: func(string) (config.Config, error) {
			cfg := config.Default()
			cfg.Agent.Type = "unknown-transport"
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	if err := cmd.Flags().Set("config", "test-config.yaml"); err != nil {
		t.Fatalf("set config flag: %v", err)
	}

	_, err := buildRuntime(cmd, opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, generator.ErrUnknownTransport) {
		t.Fatalf("expected unknown transport error, got %v", err)
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
			cfg := config.Default()
			cfg.Agent.Type = generator.AgentTypeClaudeCode
			return cfg, nil
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
			cfg.Agent.Type = generator.AgentTypeClaudeCode
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

func TestDefaultScenarioLoaderFailsWhenSchemaUnavailable(t *testing.T) {
	workspace := t.TempDir()
	scenarioPath := filepath.Join(workspace, "scenario.yaml")
	if err := os.WriteFile(scenarioPath, []byte(`scenario: web
version: "1.0"
cloud: scaleway
description: example
resources:
  compute:
    purpose: web
    size: dev1-s
acceptance_criteria: []
`), 0o600); err != nil {
		t.Fatalf("write scenario: %v", err)
	}

	_, err := loadScenarioWithSchemaCandidates(scenarioPath, []string{
		filepath.Join(workspace, "missing-schema-a.json"),
		filepath.Join(workspace, "missing-schema-b.json"),
	})
	if err == nil {
		t.Fatal("expected schema resolution error")
	}
	if !strings.Contains(err.Error(), "locate scenario schema") {
		t.Fatalf("expected schema availability error, got %v", err)
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
			expectedCode: errorCodeConfigInvalid,
			expectedIs:   config.ErrInvalidConfig,
		},
		{
			name:         "scenario malformed",
			err:          fmt.Errorf("load scenario: %w", scenario.ErrMalformedScenario),
			expectedCode: errorCodeScenarioMalformed,
			expectedIs:   scenario.ErrMalformedScenario,
		},
		{
			name:         "scenario invalid",
			err:          fmt.Errorf("load scenario: %w", scenario.ErrInvalidScenario),
			expectedCode: errorCodeScenarioInvalid,
			expectedIs:   scenario.ErrInvalidScenario,
		},
		{
			name:         "dependency unavailable",
			err:          fmt.Errorf("run mock: %w", ErrDependencyUnavailable),
			expectedCode: errorCodeDependencyUnavailable,
			expectedIs:   ErrDependencyUnavailable,
		},
		{
			name:         "fallback",
			err:          errors.New("boom"),
			expectedCode: errorCodeCommandFailed,
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

	existing := &CLIError{Op: "run", Code: errorCodeCommandFailed, Err: errors.New("existing")}
	formattedExisting := formatCommandError("run", existing)
	if formattedExisting != existing {
		t.Fatalf("expected existing CLI error to pass through unchanged")
	}
}

// providersTFEchoRunner is a fake harness.CommandRunner whose Run reads
// the providers.tf in cmd.Dir on the `tofu providers schema -json`
// invocation and returns the file's bytes as the "schema". That lets
// EnsureProviderSchema's per-cloud dispatcher prove it picked the right
// provider — the schema bytes for scaleway, gcp, aws are all
// distinguishable.
type providersTFEchoRunner struct {
	calls int
}

func (r *providersTFEchoRunner) Run(_ context.Context, cmd harness.Command) (harness.CommandResult, error) {
	r.calls++
	// tofu init is a no-op for the test; only schema returns a payload.
	if len(cmd.Args) >= 1 && cmd.Args[0] == "providers" {
		payload, err := os.ReadFile(filepath.Join(cmd.Dir, "providers.tf"))
		if err != nil {
			return harness.CommandResult{}, err
		}
		return harness.CommandResult{Stdout: payload}, nil
	}
	return harness.CommandResult{}, nil
}

// TestEnsureProviderSchemaLazyDispatchAcrossCloudsInOneProcess pins
// S43-T9's lazy-extraction contract: a single CommandRuntime visiting
// scaleway and aws scenarios back-to-back must extract each cloud's
// schema exactly once AND surface the correct schema for whichever
// cloud was last requested. Pre-S43-T9 the runtime cached only one
// schema slot, so a scaleway→aws transition would either re-extract
// from scratch every call (wasted work) or worse, leak the scaleway
// schema into the aws generator request.
func TestEnsureProviderSchemaLazyDispatchAcrossCloudsInOneProcess(t *testing.T) {
	t.Parallel()

	runner := &providersTFEchoRunner{}
	runtime := &CommandRuntime{
		Logger:       NewAppLogger(io.Discard),
		schemaRunner: runner,
	}

	runtime.EnsureProviderSchema(context.Background(), "scaleway")
	if !strings.Contains(string(runtime.ProviderSchemaJSON), "scaleway/scaleway") {
		t.Fatalf("expected scaleway schema after first call, got %q", runtime.ProviderSchemaJSON)
	}
	callsAfterScaleway := runner.calls // 2 calls: init + schema

	runtime.EnsureProviderSchema(context.Background(), "aws")
	if !strings.Contains(string(runtime.ProviderSchemaJSON), "hashicorp/aws") {
		t.Fatalf("expected aws schema after second call (cloud transition), got %q", runtime.ProviderSchemaJSON)
	}
	if runner.calls <= callsAfterScaleway {
		t.Fatalf("expected runner to be re-invoked for aws extraction (got %d calls, was %d before aws)", runner.calls, callsAfterScaleway)
	}
	callsAfterAWS := runner.calls

	// Second visit to scaleway must hit the cache — no new runner calls,
	// schema reverts to the scaleway payload.
	runtime.EnsureProviderSchema(context.Background(), "scaleway")
	if !strings.Contains(string(runtime.ProviderSchemaJSON), "scaleway/scaleway") {
		t.Fatalf("expected cached scaleway schema on revisit, got %q", runtime.ProviderSchemaJSON)
	}
	if runner.calls != callsAfterAWS {
		t.Fatalf("expected cache hit for scaleway revisit (no new runner calls), got %d (was %d)", runner.calls, callsAfterAWS)
	}

	// Second visit to aws must also hit the cache.
	runtime.EnsureProviderSchema(context.Background(), "aws")
	if !strings.Contains(string(runtime.ProviderSchemaJSON), "hashicorp/aws") {
		t.Fatalf("expected cached aws schema on revisit, got %q", runtime.ProviderSchemaJSON)
	}
	if runner.calls != callsAfterAWS {
		t.Fatalf("expected cache hit for aws revisit, got %d (was %d)", runner.calls, callsAfterAWS)
	}
}

// TestEnsureProviderSchemaEmptyCloudFallsBackToScaleway preserves the
// pre-multi-cloud default for any straggling call site that didn't get
// updated when EnsureProviderSchema gained the cloud parameter.
func TestEnsureProviderSchemaEmptyCloudFallsBackToScaleway(t *testing.T) {
	t.Parallel()

	runner := &providersTFEchoRunner{}
	runtime := &CommandRuntime{
		Logger:       NewAppLogger(io.Discard),
		schemaRunner: runner,
	}
	runtime.EnsureProviderSchema(context.Background(), "")
	if !strings.Contains(string(runtime.ProviderSchemaJSON), "scaleway/scaleway") {
		t.Fatalf("expected empty cloud to fall back to scaleway, got %q", runtime.ProviderSchemaJSON)
	}
}
