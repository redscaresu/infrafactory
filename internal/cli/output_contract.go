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

type OutputResult struct {
	Command  string           `json:"command"`
	Scenario string           `json:"scenario"`
	Status   CommandStatus    `json:"status"`
	Stages   []StageSummary   `json:"stages"`
	Failures []FailureSummary `json:"failures"`
}

type MachineOutput struct {
	Schema string       `json:"schema"`
	Result OutputResult `json:"result"`
}

func NormalizeOutput(result OutputResult) OutputResult {
	normalized := result
	normalized.Stages = append(make([]StageSummary, 0, len(result.Stages)), result.Stages...)
	normalized.Failures = append(make([]FailureSummary, 0, len(result.Failures)), result.Failures...)

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
