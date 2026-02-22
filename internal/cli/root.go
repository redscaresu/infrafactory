package cli

import (
	"errors"
	"fmt"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/spf13/cobra"
)

var ErrNotImplemented = errors.New("not implemented")

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "infrafactory",
		Short:         "Scenario-driven infrastructure generation and validation for Scaleway",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().String("config", config.DefaultPath, "Path to infrafactory config file")

	cmd.AddCommand(
		newInitCmd(),
		newGenerateCmd(),
		newValidateCmd(),
		newTestCmd(),
		newRunCmd(),
		newMockCmd(),
	)

	return cmd
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize an infrafactory scenario scaffold",
		RunE:  notImplemented("init"),
	}
}

func newGenerateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "generate",
		Short: "Generate OpenTofu files from a scenario",
		RunE:  withConfig(notImplemented("generate")),
	}
}

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate generated infrastructure definitions",
		RunE:  withConfig(notImplemented("validate")),
	}
}

func newTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "Run harness checks for a scenario",
		RunE:  withConfig(notImplemented("test")),
	}
}

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run generation and validation loop",
		RunE:  withConfig(notImplemented("run")),
	}
}

func newMockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mock",
		Short: "Manage mock service dependencies",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start Mockway dependency",
		RunE:  withConfig(notImplemented("mock start")),
	})

	return cmd
}

func notImplemented(command string) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, _ []string) error {
		return fmt.Errorf("%s: %w", command, ErrNotImplemented)
	}
}

func withConfig(next func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		configPath, err := cmd.Flags().GetString("config")
		if err != nil {
			return fmt.Errorf("read --config flag: %w", err)
		}
		if _, err := config.Load(configPath); err != nil {
			return err
		}

		return next(cmd, args)
	}
}
