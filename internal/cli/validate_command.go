package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/redscaresu/infrafactory/internal/feedback"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/spf13/cobra"
)

func runValidateCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	result, err := executeValidate(cmd.Context(), runtime, args[0])
	if writeErr := writeCommandOutput(cmd, result); writeErr != nil {
		return writeErr
	}
	return err
}

func executeValidate(ctx context.Context, runtime *CommandRuntime, scenarioPath string) (OutputResult, error) {
	sc, err := runtime.LoadScenario(scenarioPath)
	if err != nil {
		return OutputResult{}, fmt.Errorf("load scenario %q: %w", scenarioPath, err)
	}
	if runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		result := OutputResult{
			Command:  "validate",
			Scenario: sc.Name,
			Status:   CommandStatusFailed,
			Stages: []StageSummary{
				{Layer: "sandbox_deploy", Stage: "blocked", Status: StageStatusSkip, Detail: sandboxBlockedStageDetail()},
			},
			Failures: []FailureSummary{
				{
					Layer:   "sandbox_deploy",
					Stage:   "blocked",
					Check:   "sandbox_deploy",
					Command: "layer gate",
					Detail:  sandboxDeferredDetail(),
				},
			},
		}
		return result, &CLIError{
			Op:   "validate",
			Code: errorCodeCommandFailed,
			Err:  errors.New("sandbox deploy layer is blocked"),
		}
	}
	if !runtime.Config.Validation.Layers.Static.Enabled {
		result := OutputResult{
			Command:  "validate",
			Scenario: sc.Name,
			Status:   CommandStatusSuccess,
			Stages: []StageSummary{
				{Layer: "static", Stage: "disabled", Status: StageStatusSkip},
			},
		}
		return result, nil
	}
	if runtime.Deps.Static == nil {
		return OutputResult{}, fmt.Errorf("static harness dependency unavailable: %w", ErrDependencyUnavailable)
	}

	staticResult, staticErr := runtime.Deps.Static.Run(ctx, runtime.OutputDir(), validateCommandEnv(runtime))
	stages := toValidateStageSummaries(staticResult, staticErr)
	failures := make([]FailureSummary, 0)

	if staticFailure, ok := harness.StaticFailureFromError(staticErr); ok {
		failures = append(failures, toFailureSummary(*staticFailure))
	} else if staticErr != nil {
		return OutputResult{}, fmt.Errorf("run static harness: %w", staticErr)
	}

	if staticErr == nil {
		policyPaths := resolvePolicyPaths(runtime.Config.Paths.Policies, runtime.Config.Validation.Layers.Static.PolicyPaths)
		policyFailures, err := harness.EvaluatePlanPoliciesWithConstraints(ctx, staticResult.PlanJSON, sc.Constraints, policyPaths)
		if err != nil {
			return OutputResult{}, fmt.Errorf("evaluate static policies: %w", err)
		}
		if len(policyFailures) > 0 {
			stages = append(stages, StageSummary{
				Layer:  "static",
				Stage:  "opa",
				Status: StageStatusFail,
				Detail: fmt.Sprintf("%d policy failures", len(policyFailures)),
			})
		} else {
			stages = append(stages, StageSummary{
				Layer:  "static",
				Stage:  "opa",
				Status: StageStatusPass,
			})
		}

		for _, failure := range policyFailures {
			failures = append(failures, toFailureSummary(failure))
		}
	}

	status := CommandStatusSuccess
	if len(failures) > 0 {
		status = CommandStatusFailed
	}

	result := OutputResult{
		Command:  "validate",
		Scenario: sc.Name,
		Status:   status,
		Stages:   stages,
		Failures: failures,
	}
	if status == CommandStatusFailed {
		return result, &CLIError{
			Op:   "validate",
			Code: errorCodeCommandFailed,
			Err:  errors.New("validation failed"),
		}
	}

	return result, nil
}

func validateCommandEnv(runtime *CommandRuntime) map[string]string {
	return map[string]string{
		"SCW_API_URL":            runtime.Config.Mockway.URL,
		"SCW_ACCESS_KEY":         "mock-access-key",
		"SCW_SECRET_KEY":         "mock-secret-key",
		"SCW_DEFAULT_PROJECT_ID": "mock-project-id",
	}
}

func resolvePolicyPaths(baseDir string, policyPaths []string) []string {
	resolved := make([]string, 0, len(policyPaths))
	for _, policyPath := range policyPaths {
		if policyPath == "" {
			continue
		}
		if filepath.IsAbs(policyPath) {
			resolved = append(resolved, policyPath)
			continue
		}
		if _, err := os.Stat(policyPath); err == nil {
			resolved = append(resolved, policyPath)
			continue
		}
		if baseDir != "" {
			resolved = append(resolved, filepath.Join(baseDir, policyPath))
			continue
		}
		resolved = append(resolved, policyPath)
	}
	return resolved
}

func toValidateStageSummaries(result *harness.StaticResult, runErr error) []StageSummary {
	if result == nil {
		return nil
	}

	failedStage := ""
	if stageErr, ok := runErr.(*harness.StageError); ok {
		failedStage = stageErr.Stage
	}

	stages := make([]StageSummary, 0, len(result.Stages))
	for _, stage := range result.Stages {
		status := StageStatusPass
		if stage.Stage == failedStage {
			status = StageStatusFail
		}
		stages = append(stages, StageSummary{
			Layer:  "static",
			Stage:  stage.Stage,
			Status: status,
		})
	}

	return stages
}

func toFailureSummary(failure feedback.Failure) FailureSummary {
	return FailureSummary{
		Layer:    failure.Layer,
		Stage:    failure.Stage,
		Check:    failure.Check,
		Policy:   failure.Policy,
		Command:  failure.Command,
		Resource: failure.Resource,
		Detail:   failure.Detail,
	}
}
