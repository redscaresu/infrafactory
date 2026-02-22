package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/spf13/cobra"
)

func runTestCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	scenarioPath := args[0]
	sc, err := runtime.LoadScenario(scenarioPath)
	if err != nil {
		return fmt.Errorf("load scenario %q: %w", scenarioPath, err)
	}
	if runtime.Deps.MockDeploy == nil || runtime.Deps.Destroy == nil {
		return fmt.Errorf("mock deploy/destroy dependencies unavailable: %w", ErrNotImplemented)
	}

	stages := make([]StageSummary, 0)
	failures := make([]FailureSummary, 0)
	env := testCommandEnv(runtime)

	deployResult, deployErr := runtime.Deps.MockDeploy.Run(context.Background(), runtime.OutputDir(), env)
	stages, failures = appendMockDeployResult(stages, failures, deployResult, deployErr)
	if deployErr == nil {
		destroyResult, destroyErr := runtime.Deps.Destroy.Run(context.Background(), runtime.OutputDir(), env)
		stages, failures = appendDestroyResult(stages, failures, destroyResult, destroyErr)
	}

	status := CommandStatusSuccess
	if len(failures) > 0 {
		status = CommandStatusFailed
	}

	result := OutputResult{
		Command:  "test",
		Scenario: sc.Name,
		Status:   status,
		Stages:   stages,
		Failures: failures,
	}
	if err := writeCommandOutput(cmd, result); err != nil {
		return err
	}

	if status == CommandStatusFailed {
		return &CLIError{
			Op:   "test",
			Code: "command_failed",
			Err:  errors.New("test checks failed"),
		}
	}

	return nil
}

func testCommandEnv(runtime *CommandRuntime) map[string]string {
	return map[string]string{
		"SCW_API_URL":            runtime.Config.Mockway.URL,
		"SCW_ACCESS_KEY":         "mock-access-key",
		"SCW_SECRET_KEY":         "mock-secret-key",
		"SCW_DEFAULT_PROJECT_ID": "mock-project-id",
	}
}

func appendMockDeployResult(stages []StageSummary, failures []FailureSummary, result *harness.MockDeployResult, runErr error) ([]StageSummary, []FailureSummary) {
	if result != nil && result.Apply.Stage != "" {
		stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "apply", Status: StageStatusPass})
	}

	if runErr == nil {
		stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "state", Status: StageStatusPass})
		return stages, failures
	}

	mockErr := &harness.MockDeployError{}
	if !errors.As(runErr, &mockErr) {
		failures = append(failures, FailureSummary{Layer: "mock_deploy", Stage: "run", Detail: runErr.Error()})
		return stages, failures
	}

	switch mockErr.Stage {
	case "reset":
		stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "reset", Status: StageStatusFail})
	case "apply":
		if len(stages) == 0 || stages[len(stages)-1].Stage != "apply" {
			stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "apply", Status: StageStatusFail})
		} else {
			stages[len(stages)-1].Status = StageStatusFail
		}
	case "state":
		stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "state", Status: StageStatusFail})
	}

	failures = append(failures, FailureSummary{
		Layer:   "mock_deploy",
		Stage:   mockErr.Stage,
		Check:   mockErr.Stage,
		Command: "mock deploy harness",
		Detail:  mockErr.Err.Error(),
	})

	return stages, failures
}

func appendDestroyResult(stages []StageSummary, failures []FailureSummary, result *harness.DestroyResult, runErr error) ([]StageSummary, []FailureSummary) {
	if result != nil && result.Destroy.Stage != "" {
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "destroy", Status: StageStatusPass})
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "state", Status: StageStatusPass})
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "orphan_check", Status: StageStatusPass})
	}

	if runErr == nil {
		return stages, failures
	}

	destroyErr := &harness.DestroyError{}
	if !errors.As(runErr, &destroyErr) {
		failures = append(failures, FailureSummary{Layer: "destruction", Stage: "run", Detail: runErr.Error()})
		return stages, failures
	}

	switch destroyErr.Stage {
	case "destroy":
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "destroy", Status: StageStatusFail})
	case "state":
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "state", Status: StageStatusFail})
	case "orphan_check":
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "orphan_check", Status: StageStatusFail})
	}

	failures = append(failures, FailureSummary{
		Layer:   "destruction",
		Stage:   destroyErr.Stage,
		Check:   destroyErr.Stage,
		Command: "destroy harness",
		Detail:  destroyErr.Err.Error(),
	})

	return stages, failures
}
