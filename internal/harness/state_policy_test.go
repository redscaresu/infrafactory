package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestEvaluateStatePoliciesWithInputIncludesExtraFields(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	policyPath := filepath.Join(tmp, "target.rego")
	policy := `package test.target

import rego.v1

deny_state contains msg if {
	input.target != "database"
	msg := "unexpected target"
}
`
	if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
		t.Fatalf("write policy fixture: %v", err)
	}

	stateJSON := []byte(`{"rdb":{"instances":[]}}`)
	failures, err := EvaluateStatePoliciesWithInput(
		context.Background(),
		stateJSON,
		map[string]any{"target": "database"},
		[]string{policyPath},
	)
	if err != nil {
		t.Fatalf("evaluate state policy: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("expected no failures, got %+v", failures)
	}
}

func TestNoPublicDatabaseStatePolicyReadsTopLevelRDB(t *testing.T) {
	t.Parallel()

	policyPath := filepath.Join("..", "..", "policies", "scaleway", "no_public_database.rego")
	stateJSON := []byte(`{
  "rdb": {
    "instances": [
      {
        "id": "db-1",
        "endpoints": [{"private_network": false}]
      }
    ]
  }
}`)

	failures, err := EvaluateStatePolicies(context.Background(), stateJSON, []string{policyPath})
	if err != nil {
		t.Fatalf("evaluate state policy: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("expected one failure, got %d (%+v)", len(failures), failures)
	}
}

func TestEvaluateStatePoliciesWithInputRejectsTopLevelStateCollision(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	policyPath := filepath.Join(tmp, "target.rego")
	policy := `package test.target

import rego.v1

deny_state contains msg if {
	false
	msg := "unused"
}
`
	if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
		t.Fatalf("write policy fixture: %v", err)
	}

	_, err := EvaluateStatePoliciesWithInput(
		context.Background(),
		[]byte(`{"state":{"existing":true}}`),
		map[string]any{"target": "database"},
		[]string{policyPath},
	)
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(err.Error(), `top-level "state" key`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
