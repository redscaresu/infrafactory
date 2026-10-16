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
// stderr, so ReportError prints nothing when exitCodeFor passes the code through.
type TofuExitError struct {
	ExitCode int
}

func (e *TofuExitError) Error() string {
	return fmt.Sprintf("tofu exited with code %d", e.ExitCode)
}

// passesThrough reports whether ExitCode can reach os.Exit unchanged: 0 would
// report success for an error, and the OS keeps only the low 8 bits of the rest.
func (e *TofuExitError) passesThrough() bool {
	return e.ExitCode >= 1 && e.ExitCode <= 255
}

func ExitCodeForError(err error) int {
	code, _ := exitCodeFor(err)
	return code
}

// ReportError writes err to w and returns the exit code for it. It writes
// nothing only when that code is tofu's own, since tofu already explained it.
func ReportError(err error, w io.Writer) int {
	code, tofuReported := exitCodeFor(err)
	if err != nil && !tofuReported {
		fmt.Fprintln(w, err)
	}
	return code
}

// exitCodeFor returns the exit code for err and whether it is tofu's own. A
// usage error anywhere in err gives 2. Tofu's code passes through only when
// the carrier is reached through soleCause, so a carrier beside another
// failure, such as an interrupt, gives 1 and that failure is printed.
func exitCodeFor(err error) (code int, tofuReported bool) {
	if err == nil {
		return ExitCodeSuccess, false
	}

	var cliErr *CLIError
	if errors.As(err, &cliErr) && cliErr.Code == errorCodeUsage {
		return ExitCodeUsage, false
	}

	for e := err; e != nil; e = soleCause(e) {
		if tofuErr, ok := e.(*TofuExitError); ok && tofuErr.passesThrough() {
			return tofuErr.ExitCode, true
		}
	}

	return ExitCodeRuntime, false
}

// soleCause returns the one error err wraps, or nil when it wraps none or
// several. errors.Join(err, nil) wraps one, the same as a single %w.
func soleCause(err error) error {
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		if errs := multi.Unwrap(); len(errs) == 1 {
			return errs[0]
		}
		return nil
	}
	return errors.Unwrap(err)
}
