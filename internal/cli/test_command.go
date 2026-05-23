package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
	"github.com/spf13/cobra"
)

func runTestCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	noDestroy, err := cmd.Flags().GetBool("no-destroy")
	if err != nil {
		return &CLIError{Op: "test", Code: errorCodeUsage, Err: fmt.Errorf("read --no-destroy flag: %w", err)}
	}

	result, err := executeTest(cmd.Context(), runtime, args[0], testExecutionOptions{
		MockDeployMode: harness.MockDeployModeClean,
		SkipDestroy:    noDestroy,
	})
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
		"SCW_ACCESS_KEY":         "SCWMOCKACCESSKEY0000",
		"SCW_SECRET_KEY":         "00000000-0000-0000-0000-000000000000",
		"SCW_DEFAULT_PROJECT_ID": "00000000-0000-0000-0000-000000000000",
	}
}

func appendMockDeployResult(stages []StageSummary, failures []FailureSummary, result *harness.MockDeployResult, runErr error) ([]StageSummary, []FailureSummary) {
	if runErr == nil {
		if result != nil && result.Init.Stage != "" {
			stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "init", Status: StageStatusPass})
		}
		if result != nil && result.Apply.Stage != "" {
			stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "apply", Status: StageStatusPass})
		}
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
	case "init":
		stages = append(stages, StageSummary{Layer: "mock_deploy", Stage: "init", Status: StageStatusFail})
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
		Detail:  mockDeployFailureDetail(mockErr),
	})

	return stages, failures
}

func mockDeployFailureDetail(err *harness.MockDeployError) string {
	if err == nil {
		return ""
	}
	detail := err.Err.Error()
	var stderr string
	switch err.Stage {
	case "init":
		stderr = err.Init.Stderr
	case "apply":
		stderr = err.Apply.Stderr
	}
	if stderr != "" {
		trimmedStderr := strings.TrimSpace(stderr)
		if trimmedStderr != "" {
			if len(trimmedStderr) > 600 {
				trimmedStderr = trimmedStderr[:600] + "..."
			}
			detail = fmt.Sprintf("%s | stderr: %s", detail, trimmedStderr)
		}
	}
	return detail
}

func appendDestroyResult(stages []StageSummary, failures []FailureSummary, result *harness.DestroyResult, runErr error) ([]StageSummary, []FailureSummary) {
	if runErr == nil {
		if result != nil && result.Destroy.Stage != "" {
			stages = append(stages, StageSummary{Layer: "destruction", Stage: "destroy", Status: StageStatusPass})
		}
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "state", Status: StageStatusPass})
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "orphan_check", Status: StageStatusPass})
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
		Detail:  destroyFailureDetail(destroyErr),
	})

	return stages, failures
}

func destroyFailureDetail(err *harness.DestroyError) string {
	if err == nil {
		return ""
	}
	detail := err.Err.Error()
	trimmedStderr := strings.TrimSpace(err.Destroy.Stderr)
	if trimmedStderr == "" {
		return detail
	}
	if len(trimmedStderr) > 600 {
		trimmedStderr = trimmedStderr[:600] + "..."
	}
	return fmt.Sprintf("%s | stderr: %s", detail, trimmedStderr)
}

func appendSandboxDeployResult(stages []StageSummary, failures []FailureSummary, result *harness.SandboxDeployResult, runErr error) ([]StageSummary, []FailureSummary) {
	if runErr == nil {
		if result != nil && result.Init.Stage != "" {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "init", Status: StageStatusPass})
		}
		if result != nil && result.Plan.Stage != "" {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "plan", Status: StageStatusPass})
		}
		if result != nil && result.Apply.Stage != "" {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "apply", Status: StageStatusPass})
		}
		return stages, failures
	}

	deployErr := &harness.SandboxDeployError{}
	if !errors.As(runErr, &deployErr) {
		failures = append(failures, FailureSummary{Layer: "sandbox_deploy", Stage: "run", Detail: runErr.Error()})
		return stages, failures
	}

	// Record passed stages before the failed one for diagnostic visibility.
	if deployErr.Init.Stage != "" && deployErr.Stage != "init" {
		stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "init", Status: StageStatusPass})
	}
	if deployErr.Plan.Stage != "" && deployErr.Stage != "plan" {
		stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "plan", Status: StageStatusPass})
	}
	switch deployErr.Stage {
	case "init":
		stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "init", Status: StageStatusFail})
	case "plan":
		stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "plan", Status: StageStatusFail})
	case "apply":
		stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "apply", Status: StageStatusFail})
	}

	failures = append(failures, FailureSummary{
		Layer:   "sandbox_deploy",
		Stage:   deployErr.Stage,
		Check:   deployErr.Stage,
		Command: "sandbox deploy harness",
		Detail:  deployErr.Err.Error(),
	})
	return stages, failures
}

func appendSandboxDestroyResult(stages []StageSummary, failures []FailureSummary, result *harness.SandboxDestroyResult, runErr error) ([]StageSummary, []FailureSummary) {
	if runErr == nil {
		if result != nil && result.Destroy.Stage != "" {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "destroy", Status: StageStatusPass})
		}
		return stages, failures
	}

	destroyErr := &harness.SandboxDestroyError{}
	if !errors.As(runErr, &destroyErr) {
		failures = append(failures, FailureSummary{Layer: "sandbox_deploy", Stage: "destroy", Detail: runErr.Error()})
		return stages, failures
	}
	stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "destroy", Status: StageStatusFail})
	failures = append(failures, FailureSummary{
		Layer:   "sandbox_deploy",
		Stage:   destroyErr.Stage,
		Check:   destroyErr.Stage,
		Command: "sandbox destroy harness",
		Detail:  destroyErr.Err.Error(),
	})
	return stages, failures
}

type testExecutionOptions struct {
	MockDeployMode harness.MockDeployMode
	SkipDestroy    bool
}

func executeTest(ctx context.Context, runtime *CommandRuntime, scenarioPath string, opts testExecutionOptions) (OutputResult, error) {
	sc, err := runtime.LoadScenario(scenarioPath)
	if err != nil {
		return OutputResult{}, fmt.Errorf("load scenario %q: %w", scenarioPath, err)
	}
	return executeTestWithScenario(ctx, runtime, sc, runtime.OutputDir(), opts)
}

func executeTestWithScenario(ctx context.Context, runtime *CommandRuntime, sc scenario.Scenario, outputDir string, opts testExecutionOptions) (OutputResult, error) {
	unsupportedStages, unsupportedFailures, err := unsupportedCriteriaResult(sc, runtime.Config.Validation.Layers.SandboxDeploy.Enabled)
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

	if runtime.Deps.MockDeploy == nil || runtime.Deps.Destroy == nil {
		return OutputResult{}, fmt.Errorf("mock deploy/destroy dependencies unavailable: %w", ErrDependencyUnavailable)
	}

	stages := append(make([]StageSummary, 0, len(unsupportedStages)+8), unsupportedStages...)
	failures := make([]FailureSummary, 0)
	var planLiveText []byte
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

	deployMode := opts.MockDeployMode
	if deployMode == "" {
		deployMode = harness.MockDeployModeClean
	}

	deployResult, deployErr := runtime.Deps.MockDeploy.Run(ctx, outputDir, env, deployMode)
	stages, failures = appendMockDeployResult(stages, failures, deployResult, deployErr)
	sandboxEnabled := runtime.Config.Validation.Layers.SandboxDeploy.Enabled
	sandboxSucceeded := false
	if deployErr == nil && sandboxEnabled {
		sandboxEnv, sandboxEnvErr := sandboxCommandEnv(runtime)
		if sandboxEnvErr != nil {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "preflight", Status: StageStatusFail})
			failures = append(failures, FailureSummary{
				Layer:   "sandbox_deploy",
				Stage:   "preflight",
				Check:   "credentials",
				Command: "sandbox deploy preflight",
				Detail:  sandboxEnvErr.Error(),
			})
		} else {
			sandboxResult, sandboxErr := runtime.Deps.SandboxDeploy.Run(ctx, outputDir, sandboxEnv)
			stages, failures = appendSandboxDeployResult(stages, failures, sandboxResult, sandboxErr)
			sandboxSucceeded = sandboxErr == nil
			if sandboxResult != nil && len(sandboxResult.Plan.Stdout) > 0 {
				planLiveText = []byte(sandboxResult.Plan.Stdout)
			} else if sandboxErr != nil {
				var deployErr *harness.SandboxDeployError
				if errors.As(sandboxErr, &deployErr) && len(deployErr.Plan.Stdout) > 0 {
					planLiveText = []byte(deployErr.Plan.Stdout)
				}
			}
		}
	}
	if deployErr == nil && runtime.Config.Validation.Layers.Destruction.Enabled && !opts.SkipDestroy {
		criteriaStages, criteriaFailures := evaluateSupportedCriteria(ctx, sc, runtime, deployResult)
		stages = append(stages, criteriaStages...)
		failures = append(failures, criteriaFailures...)

		destroyResult, destroyErr := runtime.Deps.Destroy.Run(ctx, outputDir, env)
		stages, failures = appendDestroyResult(stages, failures, destroyResult, destroyErr)
		if sandboxEnabled && sandboxSucceeded {
			sandboxEnv, sandboxEnvErr := sandboxCommandEnv(runtime)
			if sandboxEnvErr != nil {
				stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "destroy_preflight", Status: StageStatusFail})
				failures = append(failures, FailureSummary{
					Layer:   "sandbox_deploy",
					Stage:   "destroy_preflight",
					Check:   "credentials",
					Command: "sandbox destroy preflight",
					Detail:  sandboxEnvErr.Error(),
				})
			} else {
				sandboxDestroyResult, sandboxDestroyErr := runtime.Deps.SandboxDestroy.Run(ctx, outputDir, sandboxEnv)
				stages, failures = appendSandboxDestroyResult(stages, failures, sandboxDestroyResult, sandboxDestroyErr)
			}
		}
	} else if deployErr == nil {
		criteriaStages, criteriaFailures := evaluateSupportedCriteria(ctx, sc, runtime, deployResult)
		stages = append(stages, criteriaStages...)
		failures = append(failures, criteriaFailures...)
		detail := ""
		if opts.SkipDestroy {
			detail = "skipped by --no-destroy"
		}
		stages = append(stages, StageSummary{Layer: "destruction", Stage: "disabled", Status: StageStatusSkip, Detail: detail})
	}

	status := CommandStatusSuccess
	if len(failures) > 0 {
		status = CommandStatusFailed
	}

	result := OutputResult{
		Command:      "test",
		Scenario:     sc.Name,
		Status:       status,
		Stages:       stages,
		Failures:     failures,
		PlanLiveText: planLiveText,
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

func sandboxCommandEnv(runtime *CommandRuntime) (map[string]string, error) {
	accessKey := strings.TrimSpace(os.Getenv("SCW_ACCESS_KEY"))
	if accessKey == "" {
		return nil, fmt.Errorf("sandbox deploy requires SCW_ACCESS_KEY in the environment")
	}
	secretKey := strings.TrimSpace(os.Getenv("SCW_SECRET_KEY"))
	if secretKey == "" {
		return nil, fmt.Errorf("sandbox deploy requires SCW_SECRET_KEY in the environment")
	}

	env := map[string]string{
		"SCW_ACCESS_KEY": accessKey,
		"SCW_SECRET_KEY": secretKey,
	}
	return env, nil
}

func evaluateSupportedCriteria(ctx context.Context, sc scenario.Scenario, runtime *CommandRuntime, deployResult *harness.MockDeployResult) ([]StageSummary, []FailureSummary) {
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

	policySpecs := make([]scenario.ExecutableCheckSpec, 0)
	topologyChecks := make([]harness.TopologyCheck, 0)
	realProbeChecks := make([]harness.ProbeCheck, 0)
	sandboxEnabled := runtime.Config.Validation.Layers.SandboxDeploy.Enabled
	for _, spec := range specs {
		_, supported, _ := criteriaSupportReason(spec.Type, sandboxEnabled)
		if !supported {
			continue
		}
		switch spec.Type {
		case "policy":
			policySpecs = append(policySpecs, spec)
		case "connectivity":
			if spec.Connectivity == nil {
				continue
			}
			topologyChecks = append(topologyChecks, harness.TopologyCheck{
				Type:   spec.Type,
				From:   spec.Connectivity.From,
				To:     spec.Connectivity.To,
				Port:   spec.Connectivity.Port,
				Expect: spec.Expect,
			})
			realProbeChecks = append(realProbeChecks, harness.ProbeCheck{
				Type:   spec.Type,
				Expect: spec.Expect,
				From:   spec.Connectivity.From,
				To:     spec.Connectivity.To,
				Port:   spec.Connectivity.Port,
			})
		case "http_probe":
			if spec.HTTPProbe == nil {
				continue
			}
			topologyChecks = append(topologyChecks, harness.TopologyCheck{
				Type:   spec.Type,
				Target: spec.HTTPProbe.Target,
				Port:   spec.HTTPProbe.Port,
				Expect: spec.Expect,
			})
			realProbeChecks = append(realProbeChecks, harness.ProbeCheck{
				Type:   spec.Type,
				Expect: spec.Expect,
				Target: spec.HTTPProbe.Target,
				Port:   spec.HTTPProbe.Port,
			})
		case "dns_resolution":
			if spec.DNSResolution == nil {
				continue
			}
			realProbeChecks = append(realProbeChecks, harness.ProbeCheck{
				Type:   spec.Type,
				Expect: spec.Expect,
				Domain: spec.DNSResolution.Domain,
			})
		}
	}

	stages := make([]StageSummary, 0, 2)
	failures := make([]FailureSummary, 0)

	if len(policySpecs) > 0 {
		policyFailures := evaluateStatePolicyCriteria(ctx, runtime, sc.Cloud, deployResult.StateSnapshot, policySpecs)
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

	if sandboxEnabled && len(realProbeChecks) > 0 {
		probeResult, err := runtime.Deps.RealProbe.Run(ctx, runtime.OutputDir(), sc.Name, realProbeChecks)
		if err != nil {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "real_probe", Status: StageStatusFail})
			failures = append(failures, FailureSummary{
				Layer:   "sandbox_deploy",
				Stage:   "real_probe",
				Check:   "real_probe",
				Command: "real probe harness",
				Detail:  err.Error(),
			})
		} else if len(probeResult.Failures) > 0 {
			stages = append(stages, StageSummary{
				Layer:  "sandbox_deploy",
				Stage:  "real_probe",
				Status: StageStatusFail,
				Detail: fmt.Sprintf("%d probe failures", len(probeResult.Failures)),
			})
			for _, failure := range probeResult.Failures {
				failures = append(failures, toFailureSummary(failure))
			}
		} else {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: "real_probe", Status: StageStatusPass})
		}
	} else if len(topologyChecks) > 0 {
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

	return stages, failures
}

// cloudConstraintPolicies maps a scenario's cloud to (criteria check
// name → policy path) so a `cloud: gcp` scenario with
// `check: encryption_at_rest` is routed to policies/gcp/encryption.rego
// instead of the Scaleway-only encryption_at_rest.rego that would
// otherwise vacuously pass on a google_*-only plan. Closes the
// cross-cloud bypass M37 was tracking.
var cloudConstraintPolicies = map[string]map[string]string{
	"gcp": {
		"encryption_at_rest":  "gcp/encryption.rego",
		"no_public_endpoints": "gcp/no_public_sql.rego",
		"no_public_database":  "gcp/no_public_sql.rego",
		// region_restriction is the post-S51 criterion check name
		// (matches the .rego filename); `region`/`zone` kept as
		// legacy aliases for pre-S51 scenarios.
		"region_restriction": "gcp/region_restriction.rego",
		"region":             "gcp/region_restriction.rego",
		"zone":               "gcp/region_restriction.rego",
	},
	"aws": {
		"encryption_at_rest":  "aws/encryption.rego",
		"no_public_endpoints": "aws/no_public_db.rego",
		"no_public_database":  "aws/no_public_db.rego",
		"vpc_required":        "aws/vpc_required.rego",
		"region_restriction":  "aws/region_restriction.rego",
		"region":              "aws/region_restriction.rego",
	},
}

func evaluateStatePolicyCriteria(ctx context.Context, runtime *CommandRuntime, cloud string, stateSnapshot []byte, specs []scenario.ExecutableCheckSpec) []FailureSummary {
	failures := make([]FailureSummary, 0)

	for _, spec := range specs {
		if spec.Policy == nil {
			continue
		}

		// Per-cloud lookup first; fall back to the flat
		// `constraint_policies` map (which is Scaleway-shaped today).
		policyPath := ""
		if cloudMap, ok := cloudConstraintPolicies[cloud]; ok {
			if p, ok := cloudMap[spec.Policy.Check]; ok {
				policyPath = p
			}
		}
		if policyPath == "" {
			if p, ok := runtime.Config.ConstraintPolicies[spec.Policy.Check]; ok {
				policyPath = p
			}
		}
		if policyPath == "" {
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
		policyPath = resolveConstraintPolicyPath(runtime.Config.Paths.Policies, policyPath)

		extraInput := map[string]any{}
		if spec.Policy.Target != "" {
			extraInput["target"] = spec.Policy.Target
		}

		evaluatedFailures, err := harness.EvaluateStatePoliciesWithInput(ctx, stateSnapshot, extraInput, []string{policyPath})
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

func resolveConstraintPolicyPath(baseDir, policyPath string) string {
	if policyPath == "" || filepath.IsAbs(policyPath) {
		return policyPath
	}
	if _, err := os.Stat(policyPath); err == nil {
		return policyPath
	}
	if baseDir == "" {
		return policyPath
	}
	return filepath.Join(baseDir, policyPath)
}
