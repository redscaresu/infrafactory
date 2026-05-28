package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/spf13/cobra"
)

var ErrNotImplemented = errors.New("not implemented")

const defaultInitScenarioPath = "scenarios/training/example.yaml"

type RootOption func(*rootConfig)

type rootConfig struct {
	uiAssets fs.FS
	deps     RuntimeDependencies
	depsSet  bool
}

func WithUIAssets(assets fs.FS) RootOption {
	return func(cfg *rootConfig) {
		cfg.uiAssets = assets
	}
}

// WithRuntimeDependencies overrides the default runtime dependencies used by
// command handlers. External test packages use it to inject stub generators,
// mock state clients, or harness runners without going through process
// boundaries.
func WithRuntimeDependencies(deps RuntimeDependencies) RootOption {
	return func(cfg *rootConfig) {
		cfg.deps = deps
		cfg.depsSet = true
	}
}

func (cfg *rootConfig) withRuntime(op string, next runtimeHandler) func(*cobra.Command, []string) error {
	if cfg == nil || !cfg.depsSet {
		return withRuntime(op, next)
	}
	opts := defaultRuntimeOptions()
	opts.deps = cfg.deps
	return withRuntimeWithOptions(op, opts, next)
}

func NewRootCmd(opts ...RootOption) *cobra.Command {
	cfg := &rootConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	cmd := &cobra.Command{
		Use:           "infrafactory",
		Short:         "Scenario-driven infrastructure generation and validation for AWS, GCP, and Scaleway",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			_, err := outputModeFromCommand(cmd)
			return err
		},
	}
	cmd.PersistentFlags().String("config", config.DefaultPath, "Path to infrafactory config file")
	cmd.PersistentFlags().String("output", string(OutputModeHuman), "Output mode: human|json")

	cmd.AddCommand(
		newInitCmd(),
		newGenerateCmd(cfg),
		newValidateCmd(cfg),
		newTestCmd(cfg),
		newRunCmd(cfg),
		newMockCmd(cfg),
		newUICmd(cfg.uiAssets),
	)

	return cmd
}

func newInitCmd() *cobra.Command {
	var outputPath string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize an infrafactory scenario scaffold",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := writeInitScaffold(outputPath); err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Created scenario scaffold: %s\n", outputPath)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Next steps:\n")
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "1. Edit %s and replace placeholder values.\n", outputPath)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "2. infrafactory generate %s --config %s\n", outputPath, config.DefaultPath)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "3. infrafactory run %s --config %s\n", outputPath, config.DefaultPath)

			return nil
		},
	}

	cmd.Flags().StringVar(&outputPath, "path", defaultInitScenarioPath, "Path to write the scenario scaffold")

	return cmd
}

func writeInitScaffold(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create scaffold directory for %q: %w", path, err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create scenario scaffold %q: %w", path, err)
	}
	defer f.Close()

	if _, err := f.WriteString(defaultScenarioScaffold()); err != nil {
		return fmt.Errorf("write scenario scaffold %q: %w", path, err)
	}

	return nil
}

func defaultScenarioScaffold() string {
	return `# Minimal training scenario scaffold.
# Replace placeholder values before running generate/run.
scenario: example-scenario
version: "1.0"
cloud: scaleway
description: >
  Replace this with a concise description of the infrastructure intent.

resources:
  compute:
    # Example: web-server, api-server, worker.
    purpose: web-server
    # Size maps via mappings.yaml.
    size: small

constraints:
  # Example region: fr-par, nl-ams, pl-waw.
  region: fr-par

acceptance_criteria:
  # Keep at least one criterion. Add more as needed.
  - type: destruction
    expect: no_orphans
`
}

func newGenerateCmd(cfg *rootConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "generate <scenario>",
		Short: "Generate OpenTofu files from a scenario",
		Args:  requireScenarioArg,
		RunE:  cfg.withRuntime("generate", runGenerateCommand),
	}
}

func newValidateCmd(cfg *rootConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "validate <scenario>",
		Short: "Validate generated infrastructure definitions",
		Args:  requireScenarioArg,
		RunE:  cfg.withRuntime("validate", runValidateCommand),
	}
}

func newTestCmd(cfg *rootConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test <scenario>",
		Short: "Run harness checks for a scenario",
		Args:  requireScenarioArg,
		RunE:  cfg.withRuntime("test", runTestCommand),
	}

	cmd.Flags().Bool("no-destroy", false, "Skip destruction after a successful test run to preserve state for incremental follow-up runs")

	return cmd
}

func newRunCmd(cfg *rootConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <scenario>",
		Short: "Run generation and validation loop",
		Args:  requireScenarioArg,
		RunE:  cfg.withRuntime("run", runRunCommand),
	}

	cmd.Flags().Int("repair-iterations-max", 0, "Override failure-triggered retry budget for run loop (0 uses config)")
	cmd.Flags().Bool("clean", false, "Force a clean run by resetting mock state and discarding prior Terraform state")
	cmd.Flags().Bool("no-destroy", false, "Skip destruction after a successful run to preserve state for incremental follow-up runs")

	return cmd
}

func newMockCmd(cfg *rootConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mock",
		Short: "Manage the Mockway (Scaleway) mock — use `make mocks-up` to manage all three (mockway/fakegcp/fakeaws)",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "start",
			Short: "Start Mockway (Scaleway mock). For all three mocks use `make mocks-up`.",
			RunE:  cfg.withRuntime("mock start", runMockStartCommand),
		},
		&cobra.Command{
			Use:   "stop",
			Short: "Stop Mockway (Scaleway mock). For all three mocks use `make mocks-down`.",
			RunE:  cfg.withRuntime("mock stop", runMockStopCommand),
		},
		&cobra.Command{
			Use:   "status",
			Short: "Show Mockway status. For all three mocks use `make mocks-status`.",
			RunE:  cfg.withRuntime("mock status", runMockStatusCommand),
		},
		&cobra.Command{
			Use:   "logs",
			Short: "Show Mockway logs. For all three mocks use `make mocks-logs`.",
			RunE:  cfg.withRuntime("mock logs", runMockLogsCommand),
		},
	)

	return cmd
}
