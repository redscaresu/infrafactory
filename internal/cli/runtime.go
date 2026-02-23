package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
	"github.com/spf13/cobra"
)

type StaticHarnessRunner interface {
	Run(context.Context, string, map[string]string) (*harness.StaticResult, error)
}

type MockDeployHarnessRunner interface {
	Run(context.Context, string, map[string]string) (*harness.MockDeployResult, error)
}

type DestroyHarnessRunner interface {
	Run(context.Context, string, map[string]string) (*harness.DestroyResult, error)
}

type MockStarter interface {
	Start(context.Context, config.MockwayConfig) error
}

type MockStopper interface {
	Stop(context.Context, config.MockwayConfig) error
}

type MockStatuser interface {
	Status(context.Context, config.MockwayConfig) (string, error)
}

type MockLogger interface {
	Logs(context.Context, config.MockwayConfig) (string, error)
}

type RuntimeDependencies struct {
	Generator  generator.SeedGenerator
	Static     StaticHarnessRunner
	MockDeploy MockDeployHarnessRunner
	Destroy    DestroyHarnessRunner
	MockState  harness.MockStateClient
	MockStart  MockStarter
	MockStop   MockStopper
	MockStatus MockStatuser
	MockLogs   MockLogger
}

type CommandRuntime struct {
	ConfigPath        string
	Config            config.Config
	TransportContract generator.TransportContract
	Deps              RuntimeDependencies
	Logger            *AppLogger

	scenarioLoader func(string) (scenario.Scenario, error)
	loadedScenario *scenario.Scenario
	scenarioPath   string
	outputDir      string
}

func (r *CommandRuntime) LoadScenario(path string) (scenario.Scenario, error) {
	if r.loadedScenario != nil {
		if path != r.scenarioPath {
			return scenario.Scenario{}, fmt.Errorf("scenario already loaded from %q", r.scenarioPath)
		}
		return *r.loadedScenario, nil
	}

	sc, err := r.scenarioLoader(path)
	if err != nil {
		return scenario.Scenario{}, err
	}

	r.loadedScenario = &sc
	r.scenarioPath = path
	r.outputDir = filepath.Join(r.Config.Paths.Output, sc.Name)

	return sc, nil
}

func (r *CommandRuntime) OutputDir() string {
	return r.outputDir
}

type CLIError struct {
	Op   string
	Code string
	Err  error
}

var ErrDependencyUnavailable = errors.New("dependency unavailable")

func (e *CLIError) Error() string {
	if e == nil {
		return "command failed"
	}
	if e.Op == "" {
		return fmt.Sprintf("[%s] %v", e.Code, e.Err)
	}
	return fmt.Sprintf("%s: [%s] %v", e.Op, e.Code, e.Err)
}

func (e *CLIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type runtimeHandler func(*cobra.Command, []string, *CommandRuntime) error

type runtimeOptions struct {
	configLoader   func(string) (config.Config, error)
	scenarioLoader func(string) (scenario.Scenario, error)
	deps           RuntimeDependencies
}

func defaultRuntimeOptions() runtimeOptions {
	return runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
	}
}

func defaultScenarioLoader(path string) (scenario.Scenario, error) {
	return loadScenarioWithSchemaCandidates(path, []string{
		scenario.DefaultSchemaPath,
		filepath.Join("..", "..", scenario.DefaultSchemaPath),
	})
}

func loadScenarioWithSchemaCandidates(path string, candidates []string) (scenario.Scenario, error) {
	for _, schemaPath := range candidates {
		if _, err := os.Stat(schemaPath); err == nil {
			return scenario.LoadWithSchema(path, schemaPath)
		}
	}

	return scenario.Scenario{}, fmt.Errorf(
		"locate scenario schema: none of the default schema paths exist (%s)",
		strings.Join(candidates, ", "),
	)
}

func withRuntime(op string, next runtimeHandler) func(*cobra.Command, []string) error {
	return withRuntimeWithOptions(op, defaultRuntimeOptions(), next)
}

func withRuntimeWithOptions(op string, opts runtimeOptions, next runtimeHandler) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		runtime, err := buildRuntime(cmd, opts)
		if err != nil {
			return formatCommandError(op, err)
		}
		runtime.Logger.Log(LogEntry{
			Level:   logLevelInfo,
			Command: op,
			Event:   "command_start",
			Status:  "start",
		})

		err = next(cmd, args, runtime)
		if err != nil {
			runtime.Logger.Log(LogEntry{
				Level:   logLevelError,
				Command: op,
				Event:   "command_end",
				Status:  "failed",
				Detail:  err.Error(),
			})
			return formatCommandError(op, err)
		}
		runtime.Logger.Log(LogEntry{
			Level:   logLevelInfo,
			Command: op,
			Event:   "command_end",
			Status:  "success",
		})

		return nil
	}
}

func buildRuntime(cmd *cobra.Command, opts runtimeOptions) (*CommandRuntime, error) {
	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return nil, fmt.Errorf("read --config flag: %w", err)
	}

	cfg, err := opts.configLoader(configPath)
	if err != nil {
		return nil, err
	}
	transportContract, err := generator.ContractForAgentType(cfg.Agent.Type)
	if err != nil {
		return nil, fmt.Errorf("resolve generator transport contract: %w", err)
	}

	// Runtime defaults keep command dependencies concrete in production paths,
	// while tests can still override any dependency explicitly through opts.deps.
	deps := opts.deps
	if deps.Generator == nil {
		defaultGenerator, err := buildDefaultSeedGenerator(cfg)
		if err != nil {
			return nil, err
		}
		deps.Generator = defaultGenerator
	}
	if deps.MockState == nil {
		deps.MockState = newMockwayStateClient(cfg.Mockway.URL)
	}
	if deps.Static == nil {
		deps.Static = harness.NewStaticHarness(execCommandRunner{})
	}
	if deps.MockDeploy == nil {
		deps.MockDeploy = harness.NewMockDeployHarness(execCommandRunner{}, deps.MockState)
	}
	if deps.Destroy == nil {
		deps.Destroy = harness.NewDestroyHarness(execCommandRunner{}, deps.MockState)
	}
	if deps.MockStart == nil {
		deps.MockStart = &dockerMockStarter{}
	}
	if deps.MockStop == nil {
		deps.MockStop = &dockerMockStarter{}
	}
	if deps.MockStatus == nil {
		deps.MockStatus = &dockerMockStarter{}
	}
	if deps.MockLogs == nil {
		deps.MockLogs = &dockerMockStarter{}
	}

	return &CommandRuntime{
		ConfigPath:        configPath,
		Config:            cfg,
		TransportContract: transportContract,
		Deps:              deps,
		Logger:            NewAppLogger(os.Stderr),
		scenarioLoader:    opts.scenarioLoader,
	}, nil
}

func buildDefaultSeedGenerator(cfg config.Config) (generator.SeedGenerator, error) {
	phaseDelay := time.Duration(cfg.Agent.PhaseDelaySeconds) * time.Second

	switch cfg.Agent.Type {
	case generator.AgentTypeClaudeCode:
		seed, err := generator.NewClaudeSeedGenerator(generator.ClaudeTransportConfig{
			Command:          cfg.Agent.Claude.Command,
			PromptsDir:       cfg.Paths.Prompts,
			Phases:           cfg.Agent.Phases,
			PhaseDelay:       phaseDelay,
			PhaseTimeout:     time.Duration(cfg.Agent.Claude.PhaseTimeoutSeconds) * time.Second,
			ProgressWriter:   os.Stderr,
			Constraints:      "",
			ResolvedMappings: "",
			Overrides:        "",
			Acceptance:       "",
		}, nil)
		if err != nil {
			return nil, fmt.Errorf("configure claude transport: %w", err)
		}
		return seed, nil
	case generator.AgentTypeOpenRouter:
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("openrouter requires OPENROUTER_API_KEY: %w", ErrDependencyUnavailable)
		}
		seed, err := generator.NewOpenRouterSeedGenerator(generator.OpenRouterTransportConfig{
			APIKey:           apiKey,
			Model:            cfg.Agent.OpenRouter.Model,
			BaseURL:          cfg.Agent.OpenRouter.BaseURL,
			Timeout:          time.Duration(cfg.Agent.OpenRouter.TimeoutSeconds) * time.Second,
			MaxRetries:       cfg.Agent.OpenRouter.MaxRetries,
			RetryDelay:       time.Second,
			PhaseDelay:       phaseDelay,
			PromptsDir:       cfg.Paths.Prompts,
			Phases:           cfg.Agent.Phases,
			Constraints:      "",
			ResolvedMappings: "",
			Overrides:        "",
			Acceptance:       "",
		}, nil)
		if err != nil {
			return nil, fmt.Errorf("configure openrouter transport: %w", err)
		}
		return seed, nil
	default:
		return nil, fmt.Errorf("unsupported generator agent type %q: %w", cfg.Agent.Type, ErrDependencyUnavailable)
	}
}

func formatCommandError(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotImplemented) {
		return err
	}

	var existing *CLIError
	if errors.As(err, &existing) {
		return err
	}

	code := errorCodeCommandFailed
	switch {
	case errors.Is(err, config.ErrInvalidConfig):
		code = errorCodeConfigInvalid
	case errors.Is(err, scenario.ErrMalformedScenario):
		code = errorCodeScenarioMalformed
	case errors.Is(err, scenario.ErrInvalidScenario):
		code = errorCodeScenarioInvalid
	case errors.Is(err, ErrDependencyUnavailable):
		code = errorCodeDependencyUnavailable
	}

	return &CLIError{
		Op:   op,
		Code: code,
		Err:  err,
	}
}
