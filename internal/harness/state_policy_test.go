package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluateStatePolicies(t *testing.T) {
	t.Parallel()

	policyPath := filepath.Join("testdata", "state-policy", "policy.rego")
	cases := []struct {
		name          string
		statePath     string
		expectedCount int
	}{
		{
			name:          "state policy pass",
			statePath:     filepath.Join("testdata", "state-policy", "state-pass.json"),
			expectedCount: 0,
		},
		{
			name:          "state policy fail",
			statePath:     filepath.Join("testdata", "state-policy", "state-fail.json"),
			expectedCount: 1,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stateJSON, err := os.ReadFile(tc.statePath)
			if err != nil {
				t.Fatalf("read state fixture: %v", err)
			}

			failures, err := EvaluateStatePolicies(context.Background(), stateJSON, []string{policyPath})
			if err != nil {
				t.Fatalf("evaluate state policy: %v", err)
			}
			if len(failures) != tc.expectedCount {
				t.Fatalf("expected %d failures, got %d (%+v)", tc.expectedCount, len(failures), failures)
			}
			for _, failure := range failures {
				if failure.Layer != "mock_deploy" || failure.Stage != "state_policy" || failure.Status != "fail" {
					t.Fatalf("unexpected failure shape: %+v", failure)
				}
			}
		})
	}
}
