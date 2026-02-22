package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluateTopology(t *testing.T) {
	t.Parallel()

	stateJSON, err := os.ReadFile(filepath.Join("testdata", "topology", "state.json"))
	if err != nil {
		t.Fatalf("read state fixture: %v", err)
	}

	cases := []struct {
		name          string
		checks        []TopologyCheck
		expectedCount int
	}{
		{
			name: "all checks pass",
			checks: []TopologyCheck{
				{Type: "connectivity", From: "public_internet", To: "database", Port: 5432, Expect: "blocked"},
				{Type: "connectivity", From: "compute", To: "database", Port: 5432, Expect: "success"},
				{Type: "http_probe", Target: "load_balancer", Port: 80, Expect: "reachable"},
			},
			expectedCount: 0,
		},
		{
			name: "connectivity failure",
			checks: []TopologyCheck{
				{Type: "connectivity", From: "public_internet", To: "database", Port: 5432, Expect: "success"},
			},
			expectedCount: 1,
		},
		{
			name: "http probe failure",
			checks: []TopologyCheck{
				{Type: "http_probe", Target: "load_balancer", Port: 443, Expect: "reachable"},
			},
			expectedCount: 1,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			failures, err := EvaluateTopology(stateJSON, tc.checks)
			if err != nil {
				t.Fatalf("evaluate topology: %v", err)
			}
			if len(failures) != tc.expectedCount {
				t.Fatalf("expected %d failures, got %d (%+v)", tc.expectedCount, len(failures), failures)
			}
			for _, failure := range failures {
				if failure.Layer != "mock_deploy" || failure.Stage != "topology" || failure.Status != "fail" {
					t.Fatalf("unexpected failure shape: %+v", failure)
				}
			}
		})
	}
}
