package cli

import (
	"strings"
	"testing"
)

func TestNormalizeOutputOrdersStagesAndFailuresDeterministically(t *testing.T) {
	t.Parallel()

	result := NormalizeOutput(OutputResult{
		Command:  "validate",
		Scenario: "web-app",
		Status:   CommandStatusFailed,
		Stages: []StageSummary{
			{Layer: "static", Stage: "validate", Status: StageStatusPass},
			{Layer: "deploy", Stage: "apply", Status: StageStatusFail},
			{Layer: "static", Stage: "init", Status: StageStatusPass},
		},
		Failures: []FailureSummary{
			{Layer: "deploy", Stage: "apply", Check: "connectivity", Resource: "database", Detail: "db reachable"},
			{Layer: "static", Stage: "policy", Check: "policy", Policy: "no_public_database", Detail: "policy failed"},
			{Layer: "deploy", Stage: "apply", Check: "connectivity", Resource: "compute", Detail: "compute blocked"},
		},
	})

	if got := result.Stages[0]; got.Layer != "deploy" || got.Stage != "apply" {
		t.Fatalf("expected first stage deploy/apply, got %+v", got)
	}
	if got := result.Stages[1]; got.Layer != "static" || got.Stage != "init" {
		t.Fatalf("expected second stage static/init, got %+v", got)
	}
	if got := result.Stages[2]; got.Layer != "static" || got.Stage != "validate" {
		t.Fatalf("expected third stage static/validate, got %+v", got)
	}

	if got := result.Failures[0]; got.Layer != "deploy" || got.Resource != "compute" {
		t.Fatalf("expected first failure deploy compute, got %+v", got)
	}
	if got := result.Failures[1]; got.Layer != "deploy" || got.Resource != "database" {
		t.Fatalf("expected second failure deploy database, got %+v", got)
	}
	if got := result.Failures[2]; got.Layer != "static" || got.Policy != "no_public_database" {
		t.Fatalf("expected third failure static policy, got %+v", got)
	}
}

func TestRenderHumanSummaryIsDeterministic(t *testing.T) {
	t.Parallel()

	summary := RenderHumanSummary(OutputResult{
		Command:  "validate",
		Scenario: "web-app",
		Status:   CommandStatusFailed,
		Stages: []StageSummary{
			{Layer: "static", Stage: "plan", Status: StageStatusPass},
			{Layer: "static", Stage: "policy", Status: StageStatusFail, Detail: "deny rules"},
		},
		Failures: []FailureSummary{
			{Layer: "static", Stage: "policy", Policy: "no_public_database", Detail: "database is public"},
		},
	})

	expectedParts := []string{
		"Command: validate",
		"Scenario: web-app",
		"Status: failed",
		"Stages:",
		"- static/plan: pass",
		"- static/policy: fail (deny rules)",
		"Failures:",
		"- static/policy policy=no_public_database detail=\"database is public\"",
	}
	for _, part := range expectedParts {
		if !strings.Contains(summary, part) {
			t.Fatalf("expected summary to contain %q, got:\n%s", part, summary)
		}
	}
}

func TestRenderMachineJSONIncludesSchemaAndNormalizedCollections(t *testing.T) {
	t.Parallel()

	jsonBytes, err := RenderMachineJSON(OutputResult{
		Command:  "run",
		Scenario: "web-app",
		Status:   CommandStatusSuccess,
		Stages: []StageSummary{
			{Layer: "mock", Stage: "apply", Status: StageStatusPass},
			{Layer: "static", Stage: "init", Status: StageStatusPass},
		},
		Failures: []FailureSummary{},
	})
	if err != nil {
		t.Fatalf("render machine json: %v", err)
	}

	output := string(jsonBytes)
	checks := []string{
		"\"schema\": \"" + OutputSchemaVersion + "\"",
		"\"command\": \"run\"",
		"\"scenario\": \"web-app\"",
		"\"status\": \"success\"",
		"\"stages\"",
		"\"failures\": []",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("expected machine json to contain %q, got:\n%s", check, output)
		}
	}

	firstIdx := strings.Index(output, "\"layer\": \"mock\"")
	secondIdx := strings.Index(output, "\"layer\": \"static\"")
	if firstIdx == -1 || secondIdx == -1 {
		t.Fatalf("expected both stage layers in json, got:\n%s", output)
	}
	if firstIdx >= secondIdx {
		t.Fatalf("expected normalized stage ordering (mock before static), got:\n%s", output)
	}
}
