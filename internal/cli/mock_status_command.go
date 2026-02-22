package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

func runMockStatusCommand(cmd *cobra.Command, _ []string, runtime *CommandRuntime) error {
	if runtime.Deps.MockStatus == nil {
		return fmt.Errorf("mock status dependency unavailable: %w", ErrDependencyUnavailable)
	}

	statusDetail, err := runtime.Deps.MockStatus.Status(context.Background(), runtime.Config.Mockway)
	status := CommandStatusSuccess
	stages := []StageSummary{
		{Layer: "mock", Stage: "preflight", Status: StageStatusPass},
		{Layer: "mock", Stage: "status", Status: StageStatusPass, Detail: statusDetail},
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
			Check:   "status",
			Command: "mock status",
			Detail:  err.Error(),
		})
	}

	result := OutputResult{
		Command:  "mock status",
		Scenario: "n/a",
		Status:   status,
		Stages:   stages,
		Failures: failures,
	}
	if outErr := writeCommandOutput(cmd, result); outErr != nil {
		return outErr
	}

	if status == CommandStatusFailed {
		return &CLIError{Op: "mock status", Code: errorCodeCommandFailed, Err: fmt.Errorf("mock status failed")}
	}

	return nil
}
