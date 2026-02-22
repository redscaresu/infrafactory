package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

type OutputMode string

const (
	OutputModeHuman OutputMode = "human"
	OutputModeJSON  OutputMode = "json"
)

const (
	errorCodeUsage                 = "usage"
	errorCodeCommandFailed         = "command_failed"
	errorCodeConfigInvalid         = "config_invalid"
	errorCodeScenarioMalformed     = "scenario_malformed"
	errorCodeScenarioInvalid       = "scenario_invalid"
	errorCodeDependencyUnavailable = "dependency_unavailable"
)

const (
	ExitCodeSuccess = 0
	ExitCodeRuntime = 1
	ExitCodeUsage   = 2
)

var ErrInvalidOutputMode = errors.New("invalid output mode")

func (m OutputMode) IsValid() bool {
	return m == OutputModeHuman || m == OutputModeJSON
}

func requireScenarioArg(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return &CLIError{
			Op:   cmd.CommandPath(),
			Code: errorCodeUsage,
			Err:  fmt.Errorf("expected exactly 1 scenario path argument"),
		}
	}

	return nil
}

func outputModeFromCommand(cmd *cobra.Command) (OutputMode, error) {
	value, err := cmd.Flags().GetString("output")
	if err != nil {
		return "", &CLIError{
			Op:   cmd.CommandPath(),
			Code: errorCodeUsage,
			Err:  fmt.Errorf("read --output flag: %w", err),
		}
	}

	mode := OutputMode(value)
	if !mode.IsValid() {
		return "", &CLIError{
			Op:   cmd.CommandPath(),
			Code: errorCodeUsage,
			Err:  fmt.Errorf("%w: %q (allowed: %s, %s)", ErrInvalidOutputMode, value, OutputModeHuman, OutputModeJSON),
		}
	}

	return mode, nil
}

func ExitCodeForError(err error) int {
	if err == nil {
		return ExitCodeSuccess
	}

	var cliErr *CLIError
	if errors.As(err, &cliErr) && cliErr.Code == errorCodeUsage {
		return ExitCodeUsage
	}

	return ExitCodeRuntime
}
