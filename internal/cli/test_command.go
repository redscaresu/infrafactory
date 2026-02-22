package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
	"github.com/spf13/cobra"
)

func runTestCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	result, err := executeTest(runtime, args[0])
	if err != nil {
		if writeErr := writeCommandOutput(cmd, result); writeErr != nil {
			return writeErr
		}
		return err
	}
	return writeCommandOutput(cmd, result)
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

func executeTest(runtime *CommandRuntime, scenarioPath string) (OutputResult, error) {
	sc, err := runtime.LoadScenario(scenarioPath)
	if err != nil {
		return OutputResult{}, fmt.Errorf("load scenario %q: %w", scenarioPath, err)
	}
	return executeTestWithScenario(runtime, sc, runtime.OutputDir())
}

func executeTestWithScenario(runtime *CommandRuntime, sc scenario.Scenario, outputDir string) (OutputResult, error) {
	unsupportedStages, unsupportedFailures, err := unsupportedCriteriaResult(sc)
	if err != nil {
		return OutputResult{}, err
	}
	if len(unsupportedFailures) > 0 {
		return OutputResult{
				Command:  "test",
				Scenario: sc.Name,
				Status:   CommandStatusFailed,
				Stages:   unsupportedStages,
				Failures: unsupportedFailures,
			}, &CLIError{
				Op:   "test",
				Code: errorCodeCommandFailed,
				Err:  errors.New("unsupported acceptance criteria present"),
			}
	}

	if runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		return OutputResult{
				Command:  "test",
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
		}, &CLIError{
				Op:   "test",
				Code: errorCodeCommandFailed,
				Err:  errors.New("sandbox deploy layer is blocked"),
			}
	}

	if runtime.Deps.MockDeploy == nil || runtime.Deps.Destroy == nil {
		return OutputResult{}, fmt.Errorf("mock deploy/destroy dependencies unavailable: %w", ErrDependencyUnavailable)
	}

	stages := make([]StageSummary, 0)
	failures := make([]FailureSummary, 0)
	env := testCommandEnv(runtime)

	if !runtime.Config.Validation.Layers.MockDeploy.Enabled {
		stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "disabled", Status: StageStatusSkip})
		if runtime.Config.Validation.Layers.Destruction.Enabled {
			stages = append(stages, StageSummary{Layer: "destruction", Stage: "blocked", Status: StageStatusSkip, Detail: "requires mock_deploy.enabled"})
		} else {
			stages = append(stages, StageSummary{Layer: "destruction", Stage: "disabled", Status: StageStatusSkip})
		}

		return OutputResult{
			Command:  "test",
			Scenario: sc.Name,
			Status:   CommandStatusSuccess,
			Stages:   stages,
		}, nil
	}

	deployResult, deployErr := runtime.Deps.MockDeploy.Run(context.Background(), outputDir, env)
	stages, failures = appendMockDeployResult(stages, failures, deployResult, deployErr)
	if deployErr == nil && runtime.Config.Validation.Layers.Destruction.Enabled {
		criteriaStages, criteriaFailures := evaluateSupportedCriteria(sc, runtime, deployResult)
		stages = append(stages, criteriaStages...)
		failures = append(failures, criteriaFailures...)

		destroyResult, destroyErr := runtime.Deps.Destroy.Run(context.Background(), outputDir, env)
		stages, failures = appendDestroyResult(stages, failures, destroyResult, destroyErr)
	} else if deployErr == nil {
		criteriaStages, criteriaFailures := evaluateSupportedCriteria(sc, runtime, deployResult)
		stages = append(stages, criteriaStages...)
		failures = append(failures, criteriaFailures...)
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "disabled", Status: StageStatusSkip})
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
	if status == CommandStatusFailed {
		return result, &CLIError{
			Op:   "test",
			Code: errorCodeCommandFailed,
			Err:  errors.New("test checks failed"),
		}
	}

	return result, nil
}

func evaluateSupportedCriteria(sc scenario.Scenario, runtime *CommandRuntime, deployResult *harness.MockDeployResult) ([]StageSummary, []FailureSummary) {
	if deployResult == nil {
		return nil, nil
	}

	specs, err := sc.ExecutableChecks()
	if err != nil {
		return []StageSummary{
			{Layer: "criteria", Stage: "parse", Status: StageStatusFail},
		}, []FailureSummary{
			{
				Layer:   "criteria",
				Stage:   "parse",
				Check:   "criteria_parse",
				Command: "criteria mapper",
				Detail:  err.Error(),
			},
		}
	}

	topologyChecks := make([]harness.TopologyCheck, 0)
	policySpecs := make([]scenario.ExecutableCheckSpec, 0)
	for _, spec := range specs {
		switch spec.Type {
		case "connectivity":
			if spec.Connectivity != nil {
				topologyChecks = append(topologyChecks, harness.TopologyCheck{
					Type:   spec.Type,
					From:   spec.Connectivity.From,
					To:     spec.Connectivity.To,
					Port:   spec.Connectivity.Port,
					Expect: spec.Expect,
				})
			}
		case "http_probe":
			if spec.HTTPProbe != nil {
				topologyChecks = append(topologyChecks, harness.TopologyCheck{
					Type:   spec.Type,
					Target: spec.HTTPProbe.Target,
					Port:   spec.HTTPProbe.Port,
					Expect: spec.Expect,
				})
			}
		case "policy":
			policySpecs = append(policySpecs, spec)
		}
	}

	stages := make([]StageSummary, 0, 2)
	failures := make([]FailureSummary, 0)

	if len(topologyChecks) > 0 {
		topologyFailures, err := harness.EvaluateTopology(deployResult.StateSnapshot, topologyChecks)
		if err != nil {
			stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "topology", Status: StageStatusFail})
			failures = append(failures, FailureSummary{
				Layer:   "mock_deploy",
				Stage:   "topology",
				Check:   "topology",
				Command: "topology evaluator",
				Detail:  err.Error(),
			})
		} else if len(topologyFailures) > 0 {
			stages = append(stages, StageSummary{
				Layer:  "mock_deploy",
				Stage:  "topology",
				Status: StageStatusFail,
				Detail: fmt.Sprintf("%d topology failures", len(topologyFailures)),
			})
			for _, failure := range topologyFailures {
				failures = append(failures, toFailureSummary(failure))
			}
		} else {
			stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "topology", Status: StageStatusPass})
		}
	}

	if len(policySpecs) > 0 {
		policyFailures := evaluateStatePolicyCriteria(runtime, deployResult.StateSnapshot, policySpecs)
		if len(policyFailures) > 0 {
			stages = append(stages, StageSummary{
				Layer:  "mock_deploy",
				Stage:  "state_policy",
				Status: StageStatusFail,
				Detail: fmt.Sprintf("%d policy failures", len(policyFailures)),
			})
			failures = append(failures, policyFailures...)
		} else {
			stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "state_policy", Status: StageStatusPass})
		}
	}

	return stages, failures
}

func evaluateStatePolicyCriteria(runtime *CommandRuntime, stateSnapshot []byte, specs []scenario.ExecutableCheckSpec) []FailureSummary {
	failures := make([]FailureSummary, 0)

	for _, spec := range specs {
		if spec.Policy == nil {
			continue
		}

		policyPath, ok := runtime.Config.ConstraintPolicies[spec.Policy.Check]
		if !ok || policyPath == "" {
			failures = append(failures, FailureSummary{
				Layer:   "mock_deploy",
				Stage:   "state_policy",
				Check:   "policy",
				Policy:  spec.Policy.Check,
				Command: "state policy evaluator",
				Detail:  "no constraint_policies mapping found for criteria check",
			})
			continue
		}

		evaluatedFailures, err := harness.EvaluateStatePolicies(context.Background(), stateSnapshot, []string{policyPath})
		if err != nil {
			failures = append(failures, FailureSummary{
				Layer:   "mock_deploy",
				Stage:   "state_policy",
				Check:   "policy",
				Policy:  spec.Policy.Check,
				Command: "state policy evaluator",
				Detail:  err.Error(),
			})
			continue
		}

		switch spec.Expect {
		case "pass":
			for _, evaluated := range evaluatedFailures {
				summary := toFailureSummary(evaluated)
				summary.Policy = spec.Policy.Check
				failures = append(failures, summary)
			}
		case "fail":
			if len(evaluatedFailures) == 0 {
				failures = append(failures, FailureSummary{
					Layer:   "mock_deploy",
					Stage:   "state_policy",
					Check:   "policy",
					Policy:  spec.Policy.Check,
					Command: "state policy evaluator",
					Detail:  "expected policy failure but evaluator returned pass",
				})
			}
		default:
			failures = append(failures, FailureSummary{
				Layer:   "mock_deploy",
				Stage:   "state_policy",
				Check:   "policy",
				Policy:  spec.Policy.Check,
				Command: "state policy evaluator",
				Detail:  fmt.Sprintf("unsupported policy expectation %q", spec.Expect),
			})
		}
	}

	return failures
}
