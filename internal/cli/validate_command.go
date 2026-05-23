package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/redscaresu/infrafactory/internal/feedback"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
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
	result, _, err := executeValidateWithArtifacts(ctx, runtime, scenarioPath)
	return result, err
}

type validateArtifacts struct {
	PlanText []byte
}

func executeValidateWithArtifacts(ctx context.Context, runtime *CommandRuntime, scenarioPath string) (OutputResult, validateArtifacts, error) {
	sc, err := runtime.LoadScenario(scenarioPath)
	if err != nil {
		return OutputResult{}, validateArtifacts{}, fmt.Errorf("load scenario %q: %w", scenarioPath, err)
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
		return result, validateArtifacts{}, nil
	}
	if runtime.Deps.Static == nil {
		return OutputResult{}, validateArtifacts{}, fmt.Errorf("static harness dependency unavailable: %w", ErrDependencyUnavailable)
	}

	staticResult, staticErr := runtime.Deps.Static.Run(ctx, runtime.OutputDir(), validateCommandEnv(runtime))
	stages := toValidateStageSummaries(staticResult, staticErr)
	failures := make([]FailureSummary, 0)
	artifacts := validateArtifacts{PlanText: extractPlanText(staticResult)}

	if staticFailure, ok := harness.StaticFailureFromError(staticErr); ok {
		failures = append(failures, toFailureSummary(*staticFailure))
	} else if staticErr != nil {
		return OutputResult{}, validateArtifacts{}, fmt.Errorf("run static harness: %w", staticErr)
	}

	if staticErr == nil {
		policyPaths := resolvePolicyPaths(runtime.Config.Paths.Policies, runtime.Config.Validation.Layers.Static.PolicyPaths)
		policyPaths = filterPolicyPathsByCloud(policyPaths, sc.Cloud)
		// S51: collect every policy criterion's Params into a single
		// input.params map. Pre-S51 this read sc.Constraints; the
		// scenario-level constraints field was deleted in S51-T1.
		// Auto-discovery of policy files is preserved (option A in
		// the slice plan); each rego file sees the merged params.
		params := mergeCriterionParams(sc.AcceptanceCriteria)
		policyFailures, err := harness.EvaluatePlanPoliciesWithParams(ctx, staticResult.PlanJSON, params, policyPaths)
		if err != nil {
			return OutputResult{}, validateArtifacts{}, fmt.Errorf("evaluate static policies: %w", err)
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
		return result, artifacts, &CLIError{
			Op:   "validate",
			Code: errorCodeCommandFailed,
			Err:  errors.New("validation failed"),
		}
	}

	return result, artifacts, nil
}

func extractPlanText(result *harness.StaticResult) []byte {
	if result == nil {
		return nil
	}
	for _, stage := range result.Stages {
		if stage.Stage == "plan" && stage.Stdout != "" {
			return []byte(stage.Stdout)
		}
	}
	return nil
}

func validateCommandEnv(runtime *CommandRuntime) map[string]string {
	return map[string]string{
		"SCW_API_URL":            runtime.Config.Mockway.URL,
		"SCW_ACCESS_KEY":         "SCWMOCKACCESSKEY0000",
		"SCW_SECRET_KEY":         "00000000-0000-0000-0000-000000000000",
		"SCW_DEFAULT_PROJECT_ID": "00000000-0000-0000-0000-000000000000",
	}
}

// filterPolicyPathsByCloud drops paths under `./policies/{other-cloud}/`
// when a scenario declares a cloud. An `aws` scenario should not have
// `policies/scaleway/*` or `policies/gcp/*` evaluated against its plan
// (the rego packages would either no-op vacuously or worse, fire on
// AWS resource shapes by accident). Paths that don't match any known
// cloud directory (common/, custom/, project-specific dirs) are kept.
// An empty/unknown cloud falls through unfiltered for backwards compat.
func filterPolicyPathsByCloud(paths []string, cloud string) []string {
	cloud = strings.ToLower(strings.TrimSpace(cloud))
	if cloud == "" {
		return paths
	}
	knownClouds := []string{"scaleway", "gcp", "aws"}
	otherClouds := make(map[string]struct{}, len(knownClouds)-1)
	for _, c := range knownClouds {
		if c != cloud {
			otherClouds[c] = struct{}{}
		}
	}
	filtered := make([]string, 0, len(paths))
	for _, p := range paths {
		// Match the cloud-segmented directory regardless of the rest
		// of the path. `./policies/scaleway` and `/abs/path/policies/scaleway`
		// both filter out for cloud != "scaleway".
		base := filepath.Base(strings.TrimRight(p, string(filepath.Separator)))
		if _, isOther := otherClouds[base]; isOther {
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered
}

// mergeCriterionParams walks every acceptance_criteria entry and
// merges each policy criterion's Params into a single map exposed to
// OPA as input.params. Per S51 this replaces the scenario-level
// constraints map. Later keys overwrite earlier ones on collision —
// scenarios shouldn't declare conflicting values across criteria
// (e.g., two `region_restriction` criteria with different `region`),
// but the audit follow-up will catch that case.
func mergeCriterionParams(criteria []scenario.AcceptanceCriterion) map[string]any {
	merged := map[string]any{}
	for _, c := range criteria {
		if c.Type != "policy" {
			continue
		}
		for k, v := range c.Params {
			merged[k] = v
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
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
	var stageErr *harness.StageError
	if errors.As(runErr, &stageErr) {
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
