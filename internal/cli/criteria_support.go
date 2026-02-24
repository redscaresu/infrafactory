package cli

import (
	"fmt"

	"github.com/redscaresu/infrafactory/internal/scenario"
)

func unsupportedCriteriaResult(sc scenario.Scenario) ([]StageSummary, []FailureSummary, error) {
	specs, err := sc.ExecutableChecks()
	if err != nil {
		return nil, nil, fmt.Errorf("parse acceptance criteria: %w", err)
	}

	autoPassCount := 0
	stages := make([]StageSummary, 0, 1)
	failures := make([]FailureSummary, 0)
	for idx, spec := range specs {
		reason, supported, autoPass := criteriaSupportReason(spec.Type)
		if supported {
			continue
		}
		if autoPass {
			autoPassCount++
			continue
		}

		failures = append(failures, FailureSummary{
			Layer:   "criteria",
			Stage:   "support_matrix",
			Check:   spec.Type,
			Command: "criteria support matrix",
			Detail:  fmt.Sprintf("criterion %d (%s): %s", idx+1, spec.Type, reason),
		})
	}

	if autoPassCount > 0 {
		stages = append(stages, StageSummary{
			Layer:  "criteria",
			Stage:  "support_matrix",
			Status: StageStatusSkip,
			Detail: fmt.Sprintf("%d criteria auto-passed %s", autoPassCount, sandboxRealDeploySkippedMessage),
		})
	}

	if len(failures) > 0 {
		stages = append(stages, StageSummary{
			Layer:  "criteria",
			Stage:  "support_matrix",
			Status: StageStatusSkip,
			Detail: fmt.Sprintf("%d unsupported criteria", len(failures)),
		})
	}

	if len(stages) == 0 && len(failures) == 0 {
		return nil, nil, nil
	}

	return stages, failures, nil
}

func criteriaSupportReason(criterionType string) (reason string, supported bool, autoPass bool) {
	switch criterionType {
	case "policy", "destruction":
		return "", true, false
	case "connectivity", "http_probe":
		return topologyAutoPassMessage(), false, true
	case "dns_resolution":
		return dnsResolutionAutoPassMessage(), false, true
	default:
		return "is not supported by the current criteria support matrix", false, false
	}
}

func topologyAutoPassMessage() string {
	return "requires live infrastructure to evaluate " + sandboxRealDeploySkippedMessage
}

func dnsResolutionAutoPassMessage() string {
	return "currently automatically passes due to lack of real world cloud provider " + sandboxRealDeploySkippedMessage
}
