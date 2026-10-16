package cli

import (
	"errors"
	"fmt"
	"io"

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

// TofuExitError carries the exit code of a tofu run that finished, so
// infrafactory exits with tofu's own code. Tofu has already written its own
// stderr, so ReportError prints nothing for it.
type TofuExitError struct {
	ExitCode int
}

func (e *TofuExitError) Error() string {
	return fmt.Sprintf("tofu exited with code %d", e.ExitCode)
}

// passesThrough reports whether ExitCode can reach os.Exit unchanged: 0 would
// report success for an error, and a code outside 1-255 is truncated by the OS.
func (e *TofuExitError) passesThrough() bool {
	return e.ExitCode >= 1 && e.ExitCode <= 255
}

func ExitCodeForError(err error) int {
	if err == nil {
		return ExitCodeSuccess
	}

	var tofuErr *TofuExitError
	if errors.As(err, &tofuErr) {
		if tofuErr.passesThrough() {
			return tofuErr.ExitCode
		}
		return ExitCodeRuntime
	}

	var cliErr *CLIError
	if errors.As(err, &cliErr) && cliErr.Code == errorCodeUsage {
		return ExitCodeUsage
	}

	return ExitCodeRuntime
}

// ReportError writes err to w and returns the exit code for it. A tofu exit
// code that passes through is not written: tofu already explained it. Any
// other carried code is written, so an exit 1 is never silent.
func ReportError(err error, w io.Writer) int {
	var tofuErr *TofuExitError
	quiet := errors.As(err, &tofuErr) && tofuErr.passesThrough()
	if err != nil && !quiet {
		fmt.Fprintln(w, err)
	}
	return ExitCodeForError(err)
}
