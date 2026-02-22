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

	failures := make([]FailureSummary, 0)
	for idx, spec := range specs {
		reason, supported := criteriaSupportReason(spec.Type)
		if supported {
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

	if len(failures) == 0 {
		return nil, nil, nil
	}

	stages := []StageSummary{
		{
			Layer:  "criteria",
			Stage:  "support_matrix",
			Status: StageStatusSkip,
			Detail: fmt.Sprintf("%d unsupported criteria", len(failures)),
		},
	}
	return stages, failures, nil
}

func criteriaSupportReason(criterionType string) (string, bool) {
	switch criterionType {
	case "connectivity", "http_probe", "policy", "destruction":
		return "", true
	case "dns_resolution":
		return "requires sandbox/live deploy layer, which is intentionally deferred " + sandboxRealDeploySkippedMessage, false
	default:
		return "is not supported by the current criteria support matrix", false
	}
}
