package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOutputContractGoldenSnapshots(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		result OutputResult
	}{
		{
			name: "generate_success",
			result: OutputResult{
				Command:  "generate",
				Scenario: "web-app",
				Status:   CommandStatusSuccess,
				Stages: []StageSummary{
					{Layer: "generate", Stage: "seed", Status: StageStatusPass},
					{Layer: "generate", Stage: "write_files", Status: StageStatusPass, Detail: "3 files"},
				},
			},
		},
		{
			name: "generate_failure",
			result: OutputResult{
				Command:  "generate",
				Scenario: "web-app",
				Status:   CommandStatusFailed,
				Stages: []StageSummary{
					{Layer: "generate", Stage: "seed", Status: StageStatusFail},
				},
				Failures: []FailureSummary{
					{Layer: "generate", Stage: "seed", Check: "seed", Command: "generator", Detail: "transport unavailable"},
				},
			},
		},
		{
			name: "validate_success",
			result: OutputResult{
				Command:  "validate",
				Scenario: "web-app",
				Status:   CommandStatusSuccess,
				Stages: []StageSummary{
					{Layer: "static", Stage: "init", Status: StageStatusPass},
					{Layer: "static", Stage: "validate", Status: StageStatusPass},
					{Layer: "static", Stage: "plan", Status: StageStatusPass},
					{Layer: "static", Stage: "show", Status: StageStatusPass},
					{Layer: "static", Stage: "opa", Status: StageStatusPass},
				},
			},
		},
		{
			name: "validate_failure_policy",
			result: OutputResult{
				Command:  "validate",
				Scenario: "web-app",
				Status:   CommandStatusFailed,
				Stages: []StageSummary{
					{Layer: "static", Stage: "init", Status: StageStatusPass},
					{Layer: "static", Stage: "validate", Status: StageStatusPass},
					{Layer: "static", Stage: "plan", Status: StageStatusPass},
					{Layer: "static", Stage: "show", Status: StageStatusPass},
					{Layer: "static", Stage: "opa", Status: StageStatusFail, Detail: "1 policy failures"},
				},
				Failures: []FailureSummary{
					{Layer: "static", Stage: "opa", Check: "policy", Policy: "no_public_database", Detail: "database endpoint is public"},
				},
			},
		},
		{
			name: "test_success",
			result: OutputResult{
				Command:  "test",
				Scenario: "web-app",
				Status:   CommandStatusSuccess,
				Stages: []StageSummary{
					{Layer: "mock_deploy", Stage: "apply", Status: StageStatusPass},
					{Layer: "mock_deploy", Stage: "topology", Status: StageStatusPass},
					{Layer: "mock_deploy", Stage: "state_policy", Status: StageStatusPass},
					{Layer: "destruction", Stage: "destroy", Status: StageStatusPass},
					{Layer: "destruction", Stage: "state", Status: StageStatusPass},
					{Layer: "destruction", Stage: "orphan_check", Status: StageStatusPass},
				},
			},
		},
		{
			name: "test_failure_connectivity",
			result: OutputResult{
				Command:  "test",
				Scenario: "web-app",
				Status:   CommandStatusFailed,
				Stages: []StageSummary{
					{Layer: "mock_deploy", Stage: "apply", Status: StageStatusPass},
					{Layer: "mock_deploy", Stage: "topology", Status: StageStatusFail, Detail: "1 topology failures"},
				},
				Failures: []FailureSummary{
					{Layer: "mock_deploy", Stage: "topology", Check: "connectivity", Resource: "database", Detail: "connectivity expectation not met"},
				},
			},
		},
		{
			name: "run_success",
			result: OutputResult{
				Command:  "run",
				Scenario: "web-app",
				Status:   CommandStatusSuccess,
				Stages: []StageSummary{
					{Layer: "run", Stage: "iteration_1_generate", Status: StageStatusPass},
					{Layer: "run", Stage: "iteration_1_validate", Status: StageStatusPass},
					{Layer: "run", Stage: "iteration_1_test", Status: StageStatusPass},
					{Layer: "run", Stage: "metadata", Status: StageStatusPass, Detail: "run_id=20260222T120000Z+0000"},
				},
			},
		},
		{
			name: "run_failure",
			result: OutputResult{
				Command:  "run",
				Scenario: "web-app",
				Status:   CommandStatusFailed,
				Stages: []StageSummary{
					{Layer: "run", Stage: "iteration_1_generate", Status: StageStatusPass},
					{Layer: "run", Stage: "iteration_1_validate", Status: StageStatusFail},
				},
				Failures: []FailureSummary{
					{Layer: "run", Stage: "iteration_1_validate", Check: "validate", Command: "validate", Detail: "validation failed"},
					{Layer: "run", Stage: "iteration_1", Check: "stuck", Command: "run loop", Detail: "stopped due to stuck detection"},
				},
			},
		},
		{
			name: "mock_start_success",
			result: OutputResult{
				Command:  "mock start",
				Scenario: "n/a",
				Status:   CommandStatusSuccess,
				Stages: []StageSummary{
					{Layer: "mock", Stage: "preflight", Status: StageStatusPass},
					{Layer: "mock", Stage: "start", Status: StageStatusPass},
				},
			},
		},
		{
			name: "mock_start_failure",
			result: OutputResult{
				Command:  "mock start",
				Scenario: "n/a",
				Status:   CommandStatusFailed,
				Stages: []StageSummary{
					{Layer: "mock", Stage: "preflight", Status: StageStatusFail},
				},
				Failures: []FailureSummary{
					{Layer: "mock", Stage: "preflight", Check: "start", Command: "mock start", Detail: "docker unavailable"},
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			human := RenderHumanSummary(tc.result)
			assertGolden(t, tc.name+".human.txt", []byte(human))

			machine, err := RenderMachineJSON(tc.result)
			if err != nil {
				t.Fatalf("render machine json: %v", err)
			}
			assertGolden(t, tc.name+".json", append(machine, '\n'))
		})
	}
}

func assertGolden(t *testing.T, name string, actual []byte) {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file path")
	}
	path := filepath.Join(filepath.Dir(thisFile), "testdata", "golden", "output_contract", name)
	update := os.Getenv("UPDATE_GOLDEN") == "1"
	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}
		if err := os.WriteFile(path, actual, 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
	}

	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file %q: %v", path, err)
	}
	if string(expected) != string(actual) {
		t.Fatalf("golden mismatch for %s (set UPDATE_GOLDEN=1 to refresh)", path)
	}
}
