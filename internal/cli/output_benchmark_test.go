package cli

import "testing"

func BenchmarkOutputContractRenderMachineJSON(b *testing.B) {
	result := OutputResult{
		Command:  "run",
		Scenario: "web-app",
		Status:   CommandStatusFailed,
		Stages: []StageSummary{
			{Layer: "run", Stage: "iteration_1_generate", Status: StageStatusPass},
			{Layer: "run", Stage: "iteration_1_validate", Status: StageStatusFail},
			{Layer: "run", Stage: "iteration_1_test", Status: StageStatusSkip},
		},
		Failures: []FailureSummary{
			{Layer: "run", Stage: "iteration_1_validate", Check: "validate", Command: "validate", Detail: "validation failed"},
			{Layer: "run", Stage: "iteration_1", Check: "stuck", Command: "run loop", Detail: "stopped due to stuck detection"},
			{Layer: "mock_deploy", Stage: "state_policy", Check: "policy", Policy: "encryption_at_rest", Detail: "deny_state triggered"},
		},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := RenderMachineJSON(result); err != nil {
			b.Fatalf("render machine json: %v", err)
		}
	}
}

func BenchmarkOutputContractRenderHumanSummary(b *testing.B) {
	result := OutputResult{
		Command:  "test",
		Scenario: "web-app",
		Status:   CommandStatusFailed,
		Stages: []StageSummary{
			{Layer: "mock_deploy", Stage: "apply", Status: StageStatusPass},
			{Layer: "mock_deploy", Stage: "topology", Status: StageStatusFail, Detail: "2 topology failures"},
			{Layer: "destruction", Stage: "disabled", Status: StageStatusSkip},
		},
		Failures: []FailureSummary{
			{Layer: "mock_deploy", Stage: "topology", Check: "connectivity", Resource: "database", Detail: "connectivity expectation not met"},
			{Layer: "mock_deploy", Stage: "topology", Check: "http_probe", Resource: "load_balancer", Detail: "http probe expectation not met"},
		},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = RenderHumanSummary(result)
	}
}
