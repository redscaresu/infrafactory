package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluatePlanPolicies(t *testing.T) {
	t.Parallel()

	policyPath := filepath.Join("testdata", "opa", "policy.rego")
	cases := []struct {
		name           string
		planPath       string
		expectedCount  int
		expectedPolicy string
	}{
		{
			name:          "policy pass",
			planPath:      filepath.Join("testdata", "opa", "plan-pass.json"),
			expectedCount: 0,
		},
		{
			name:           "policy fail",
			planPath:       filepath.Join("testdata", "opa", "plan-fail.json"),
			expectedCount:  1,
			expectedPolicy: "test.plan",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			planJSON, err := os.ReadFile(tc.planPath)
			if err != nil {
				t.Fatalf("read plan fixture: %v", err)
			}

			failures, err := EvaluatePlanPolicies(context.Background(), planJSON, []string{policyPath})
			if err != nil {
				t.Fatalf("evaluate policies: %v", err)
			}
			if len(failures) != tc.expectedCount {
				t.Fatalf("expected %d failures, got %d (%+v)", tc.expectedCount, len(failures), failures)
			}

			if tc.expectedCount > 0 {
				if failures[0].Policy != tc.expectedPolicy {
					t.Fatalf("expected policy %q, got %q", tc.expectedPolicy, failures[0].Policy)
				}
				if failures[0].Layer != "static" || failures[0].Check != "policy" {
					t.Fatalf("unexpected failure shape: %+v", failures[0])
				}
				if failures[0].Stage != "opa" || failures[0].Command != "opa eval" || failures[0].Status != "fail" {
					t.Fatalf("unexpected failure shape: %+v", failures[0])
				}
			}
		})
	}
}
