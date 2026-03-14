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
}

func WithUIAssets(assets fs.FS) RootOption {
	return func(cfg *rootConfig) {
		cfg.uiAssets = assets
	}
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
		Short:         "Scenario-driven infrastructure generation and validation for Scaleway",
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
		newGenerateCmd(),
		newValidateCmd(),
		newTestCmd(),
		newRunCmd(),
		newMockCmd(),
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

func newGenerateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "generate <scenario>",
		Short: "Generate OpenTofu files from a scenario",
		Args:  requireScenarioArg,
		RunE:  withRuntime("generate", runGenerateCommand),
	}
}

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <scenario>",
		Short: "Validate generated infrastructure definitions",
		Args:  requireScenarioArg,
		RunE:  withRuntime("validate", runValidateCommand),
	}
}

func newTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <scenario>",
		Short: "Run harness checks for a scenario",
		Args:  requireScenarioArg,
		RunE:  withRuntime("test", runTestCommand),
	}
}

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <scenario>",
		Short: "Run generation and validation loop",
		Args:  requireScenarioArg,
		RunE:  withRuntime("run", runRunCommand),
	}

	cmd.Flags().Int("repair-iterations-max", 0, "Override failure-triggered retry budget for run loop (0 uses config)")

	return cmd
}

func newMockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mock",
		Short: "Manage mock service dependencies",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "start",
			Short: "Start Mockway dependency",
			RunE:  withRuntime("mock start", runMockStartCommand),
		},
		&cobra.Command{
			Use:   "stop",
			Short: "Stop Mockway dependency",
			RunE:  withRuntime("mock stop", runMockStopCommand),
		},
		&cobra.Command{
			Use:   "status",
			Short: "Show Mockway dependency status",
			RunE:  withRuntime("mock status", runMockStatusCommand),
		},
		&cobra.Command{
			Use:   "logs",
			Short: "Show Mockway dependency logs",
			RunE:  withRuntime("mock logs", runMockLogsCommand),
		},
	)

	return cmd
}
