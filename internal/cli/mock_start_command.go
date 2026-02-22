package cli

import (
	"context"
	"errors"

	"github.com/spf13/cobra"
)

func runMockStartCommand(cmd *cobra.Command, _ []string, runtime *CommandRuntime) error {
	if runtime.Deps.MockStart == nil {
		return errors.New("mock starter dependency unavailable")
	}

	err := runtime.Deps.MockStart.Start(context.Background(), runtime.Config.Mockway)
	status := CommandStatusSuccess
	stages := []StageSummary{
		{Layer: "mock", Stage: "preflight", Status: StageStatusPass},
		{Layer: "mock", Stage: "start", Status: StageStatusPass},
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
			Check:   "start",
			Command: "mock start",
			Detail:  err.Error(),
		})
	}

	result := OutputResult{
		Command:  "mock start",
		Scenario: "n/a",
		Status:   status,
		Stages:   stages,
		Failures: failures,
	}
	if outErr := writeCommandOutput(cmd, result); outErr != nil {
		return outErr
	}

	if status == CommandStatusFailed {
		return &CLIError{Op: "mock start", Code: "command_failed", Err: errors.New("mock start failed")}
	}

	return nil
}
