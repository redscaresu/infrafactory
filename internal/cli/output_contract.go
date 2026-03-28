package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const OutputSchemaVersion = "infrafactory.output.v1"

type CommandStatus string

type StageStatus string

const (
	CommandStatusSuccess CommandStatus = "success"
	CommandStatusFailed  CommandStatus = "failed"

	StageStatusPass StageStatus = "pass"
	StageStatusFail StageStatus = "fail"
	StageStatusSkip StageStatus = "skip"
)

type StageSummary struct {
	Layer  string      `json:"layer"`
	Stage  string      `json:"stage"`
	Status StageStatus `json:"status"`
	Detail string      `json:"detail,omitempty"`
}

type FailureSummary struct {
	Layer    string `json:"layer"`
	Stage    string `json:"stage"`
	Check    string `json:"check,omitempty"`
	Policy   string `json:"policy,omitempty"`
	Command  string `json:"command,omitempty"`
	Resource string `json:"resource,omitempty"`
	Detail   string `json:"detail"`
}

type ExplainabilitySummary struct {
	Layer   string `json:"layer"`
	Stage   string `json:"stage"`
	Check   string `json:"check,omitempty"`
	Policy  string `json:"policy,omitempty"`
	Summary string `json:"summary"`
	Action  string `json:"action,omitempty"`
}

type OutputResult struct {
	Command        string                  `json:"command"`
	Scenario       string                  `json:"scenario"`
	Status         CommandStatus           `json:"status"`
	Stages         []StageSummary          `json:"stages"`
	Failures       []FailureSummary        `json:"failures"`
	Explainability []ExplainabilitySummary `json:"explainability,omitempty"`
}

type MachineOutput struct {
	Schema string       `json:"schema"`
	Result OutputResult `json:"result"`
}

func NormalizeOutput(result OutputResult) OutputResult {
	normalized := result
	normalized.Stages = append(make([]StageSummary, 0, len(result.Stages)), result.Stages...)
	normalized.Failures = append(make([]FailureSummary, 0, len(result.Failures)), result.Failures...)
	normalized.Explainability = append(make([]ExplainabilitySummary, 0, len(result.Explainability)), result.Explainability...)

	sort.Slice(normalized.Stages, func(i, j int) bool {
		left := normalized.Stages[i]
		right := normalized.Stages[j]
		if left.Layer != right.Layer {
			return left.Layer < right.Layer
		}
		if left.Stage != right.Stage {
			return left.Stage < right.Stage
		}
		if left.Status != right.Status {
			return left.Status < right.Status
		}
		return left.Detail < right.Detail
	})

	sort.Slice(normalized.Failures, func(i, j int) bool {
		left := normalized.Failures[i]
		right := normalized.Failures[j]
		if left.Layer != right.Layer {
			return left.Layer < right.Layer
		}
		if left.Stage != right.Stage {
			return left.Stage < right.Stage
		}
		if left.Check != right.Check {
			return left.Check < right.Check
		}
		if left.Resource != right.Resource {
			return left.Resource < right.Resource
		}
		if left.Policy != right.Policy {
			return left.Policy < right.Policy
		}
		if left.Command != right.Command {
			return left.Command < right.Command
		}
		return left.Detail < right.Detail
	})

	normalized.Explainability = normalizeExplainability(normalized.Explainability, normalized.Failures)

	return normalized
}

func RenderHumanSummary(result OutputResult) string {
	normalized := NormalizeOutput(result)

	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Command: %s\n", normalized.Command)
	_, _ = fmt.Fprintf(&b, "Scenario: %s\n", normalized.Scenario)
	_, _ = fmt.Fprintf(&b, "Status: %s\n", normalized.Status)

	if len(normalized.Stages) > 0 {
		_, _ = fmt.Fprintf(&b, "\nStages:\n")
		for _, stage := range normalized.Stages {
			if stage.Detail == "" {
				_, _ = fmt.Fprintf(&b, "- %s/%s: %s\n", stage.Layer, stage.Stage, stage.Status)
				continue
			}
			_, _ = fmt.Fprintf(&b, "- %s/%s: %s (%s)\n", stage.Layer, stage.Stage, stage.Status, stage.Detail)
		}
	}

	if len(normalized.Failures) > 0 {
		_, _ = fmt.Fprintf(&b, "\nFailures:\n")
		for _, failure := range normalized.Failures {
			_, _ = fmt.Fprintf(&b, "- %s/%s", failure.Layer, failure.Stage)
			if failure.Check != "" {
				_, _ = fmt.Fprintf(&b, " check=%s", failure.Check)
			}
			if failure.Policy != "" {
				_, _ = fmt.Fprintf(&b, " policy=%s", failure.Policy)
			}
			if failure.Resource != "" {
				_, _ = fmt.Fprintf(&b, " resource=%s", failure.Resource)
			}
			if failure.Command != "" {
				_, _ = fmt.Fprintf(&b, " command=%q", failure.Command)
			}
			_, _ = fmt.Fprintf(&b, " detail=%q\n", failure.Detail)
		}
	}

	if len(normalized.Explainability) > 0 {
		_, _ = fmt.Fprintf(&b, "\nExplainability:\n")
		for _, explanation := range normalized.Explainability {
			_, _ = fmt.Fprintf(&b, "- %s/%s", explanation.Layer, explanation.Stage)
			if explanation.Check != "" {
				_, _ = fmt.Fprintf(&b, " check=%s", explanation.Check)
			}
			if explanation.Policy != "" {
				_, _ = fmt.Fprintf(&b, " policy=%s", explanation.Policy)
			}
			_, _ = fmt.Fprintf(&b, " summary=%q", explanation.Summary)
			if explanation.Action != "" {
				_, _ = fmt.Fprintf(&b, " action=%q", explanation.Action)
			}
			_, _ = fmt.Fprintf(&b, "\n")
		}
	}

	return b.String()
}

func RenderMachineJSON(result OutputResult) ([]byte, error) {
	payload := MachineOutput{
		Schema: OutputSchemaVersion,
		Result: NormalizeOutput(result),
	}

	bytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal machine output: %w", err)
	}

	return bytes, nil
}

func normalizeExplainability(input []ExplainabilitySummary, failures []FailureSummary) []ExplainabilitySummary {
	combined := append(make([]ExplainabilitySummary, 0, len(input)+len(failures)), input...)
	for _, failure := range failures {
		explanation, ok := explainabilityFromFailure(failure)
		if ok {
			combined = append(combined, explanation)
		}
	}

	if len(combined) == 0 {
		return nil
	}

	dedup := make(map[string]ExplainabilitySummary, len(combined))
	for _, explanation := range combined {
		key := explanation.Layer + "\x00" + explanation.Stage + "\x00" + explanation.Check + "\x00" + explanation.Policy + "\x00" + explanation.Summary + "\x00" + explanation.Action
		dedup[key] = explanation
	}

	normalized := make([]ExplainabilitySummary, 0, len(dedup))
	for _, explanation := range dedup {
		normalized = append(normalized, explanation)
	}

	sort.Slice(normalized, func(i, j int) bool {
		left := normalized[i]
		right := normalized[j]
		if left.Layer != right.Layer {
			return left.Layer < right.Layer
		}
		if left.Stage != right.Stage {
			return left.Stage < right.Stage
		}
		if left.Check != right.Check {
			return left.Check < right.Check
		}
		if left.Policy != right.Policy {
			return left.Policy < right.Policy
		}
		if left.Summary != right.Summary {
			return left.Summary < right.Summary
		}
		return left.Action < right.Action
	})

	return normalized
}

func explainabilityFromFailure(failure FailureSummary) (ExplainabilitySummary, bool) {
	if failure.Policy == "" && failure.Check == "" {
		return ExplainabilitySummary{}, false
	}

	explanation := ExplainabilitySummary{
		Layer:  failure.Layer,
		Stage:  failure.Stage,
		Check:  failure.Check,
		Policy: failure.Policy,
	}

	switch {
	case failure.Policy != "":
		explanation.Summary = "policy check failed for mapped constraint"
		explanation.Action = "verify constraint policy mapping and inspect plan/state inputs for the named policy"
	case failure.Check == "connectivity" || failure.Check == "http_probe":
		if failure.Layer == "sandbox_deploy" {
			explanation.Summary = "real Layer 3 network probe failed"
			explanation.Action = "inspect live endpoints, bootstrap project wiring, and service readiness for the probe target"
		} else {
			explanation.Summary = "criteria check failed for network reachability expectations"
			explanation.Action = "validate topology targets/ports and inspect generated infrastructure connectivity"
		}
	case failure.Check == "dns_resolution":
		explanation.Summary = "real Layer 3 DNS probe failed"
		explanation.Action = "inspect DNS records, propagation timing, and the generated domain/output wiring"
	case failure.Check == "credentials":
		explanation.Summary = "Layer 3 real Scaleway deploy is enabled but credentials are unavailable"
		explanation.Action = "set SCW_ACCESS_KEY and SCW_SECRET_KEY before enabling sandbox_deploy"
	case failure.Check == "real_probe":
		explanation.Summary = "real probe harness failed before probe evaluation completed"
		explanation.Action = "inspect terraform-live.tfstate and probe target resolution for the requested criteria"
	case failure.Check == "destruction" || failure.Check == "orphan_check":
		explanation.Summary = "destruction criteria failed to prove clean teardown"
		explanation.Action = "inspect destroy logs/state and ensure no orphaned resources remain"
	default:
		explanation.Summary = "criteria check failed and requires scenario/harness inspection"
		explanation.Action = "review failure detail and criteria definition for this check"
	}

	return explanation, true
}
