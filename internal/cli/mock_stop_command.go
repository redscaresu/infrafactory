package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

func runMockStopCommand(cmd *cobra.Command, _ []string, runtime *CommandRuntime) error {
	if runtime.Deps.MockStop == nil {
		return fmt.Errorf("mock stopper dependency unavailable: %w", ErrDependencyUnavailable)
	}

	err := runtime.Deps.MockStop.Stop(cmd.Context(), runtime.Config.Mockway)
	status := CommandStatusSuccess
	stages := []StageSummary{
		{Layer: "mock", Stage: "preflight", Status: StageStatusPass},
		{Layer: "mock", Stage: "stop", Status: StageStatusPass},
	}
	failures := []FailureSummary{}
	if err != nil {
		status = CommandStatusFailed
		stages = []StageSummary{
			{Layer: "mock", Stage: "preflight", Status: StageStatusFail},
		}
		failures = append(failures, FailureSummary{
			Layer:   "mock",
			Stage:   "preflight",
			Check:   "stop",
			Command: "mock stop",
			Detail:  err.Error(),
		})
	}

	result := OutputResult{
		Command:  "mock stop",
		Scenario: "n/a",
		Status:   status,
		Stages:   stages,
		Failures: failures,
	}
	if outErr := writeCommandOutput(cmd, result); outErr != nil {
		return outErr
	}

	if status == CommandStatusFailed {
		return &CLIError{Op: "mock stop", Code: errorCodeCommandFailed, Err: errors.New("mock stop failed")}
	}

	return nil
}
