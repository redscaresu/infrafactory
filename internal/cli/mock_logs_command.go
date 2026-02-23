package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func runMockLogsCommand(cmd *cobra.Command, _ []string, runtime *CommandRuntime) error {
	if runtime.Deps.MockLogs == nil {
		return fmt.Errorf("mock logs dependency unavailable: %w", ErrDependencyUnavailable)
	}

	logDetail, err := runtime.Deps.MockLogs.Logs(cmd.Context(), runtime.Config.Mockway)
	status := CommandStatusSuccess
	stages := []StageSummary{
		{Layer: "mock", Stage: "preflight", Status: StageStatusPass},
		{Layer: "mock", Stage: "logs", Status: StageStatusPass, Detail: logDetail},
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
			Check:   "logs",
			Command: "mock logs",
			Detail:  err.Error(),
		})
	}

	result := OutputResult{
		Command:  "mock logs",
		Scenario: "n/a",
		Status:   status,
		Stages:   stages,
		Failures: failures,
	}
	if outErr := writeCommandOutput(cmd, result); outErr != nil {
		return outErr
	}

	if status == CommandStatusFailed {
		return &CLIError{Op: "mock logs", Code: errorCodeCommandFailed, Err: fmt.Errorf("mock logs failed")}
	}

	return nil
}
