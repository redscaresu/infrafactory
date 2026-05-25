package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestMinistackState_LivePolyfill is gated behind MINISTACK_LIVE_URL.
// Set MINISTACK_LIVE_URL=http://127.0.0.1:4566 and have ministack
// running with the M64-spike composition applied (14 resources across
// VPC + IAM + EKS + S3 + RDS + Secrets Manager) to verify the polyfill
// produces the expected state shape end-to-end.
//
// Asserted invariants (just enough to catch the most likely regressions —
// the polyfill returns a parseable JSON blob with the expected top-level
// keys and resource counts > 0 for each populated service):
//   - top-level keys: ec2, iam, rds, eks, s3, secretsmanager (+ schema_version)
//   - ec2.vpcs len ≥ 1
//   - ec2.subnets len ≥ 2
//   - iam.roles len ≥ 2
//   - rds.db_instances len ≥ 1
//   - eks.clusters len ≥ 1
//   - s3.buckets len ≥ 1
//   - secretsmanager.secrets len ≥ 1
func TestMinistackState_LivePolyfill(t *testing.T) {
	baseURL := os.Getenv("MINISTACK_LIVE_URL")
	if baseURL == "" {
		t.Skip("set MINISTACK_LIVE_URL=http://127.0.0.1:4566 to run live polyfill test")
	}

	// Sanity: ministack must be reachable.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/_ministack/health", nil)
	if err != nil {
		t.Fatalf("build health request: %v", err)
	}
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 200 {
		t.Skipf("ministack not reachable at %s: %v", baseURL, err)
	} else {
		resp.Body.Close()
	}

	builder := newMinistackStateBuilder()
	stateJSON, err := builder.BuildState(context.Background(), baseURL)
	if err != nil {
		t.Fatalf("BuildState: %v", err)
	}

	var state map[string]any
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		t.Fatalf("unmarshal state: %v\nraw: %s", err, stateJSON)
	}

	// schema_version must be set (consumers look for it as the AWS
	// detection signal in topology_derive.detectCloud).
	if _, ok := state["schema_version"]; !ok {
		t.Errorf("missing schema_version in state output")
	}

	mustCount := func(root, collection string, min int) {
		t.Helper()
		rootMap, ok := state[root].(map[string]any)
		if !ok {
			t.Errorf("state[%q] is not a map (got %T)", root, state[root])
			return
		}
		items, ok := rootMap[collection].([]any)
		if !ok {
			t.Errorf("state[%q][%q] is not an array (got %T)", root, collection, rootMap[collection])
			return
		}
		if len(items) < min {
			t.Errorf("state[%q][%q] has %d items, want ≥ %d", root, collection, len(items), min)
		}
	}

	mustCount("ec2", "vpcs", 1)
	mustCount("ec2", "subnets", 2)
	mustCount("iam", "roles", 2)
	mustCount("rds", "db_instances", 1)
	mustCount("rds", "db_subnet_groups", 1)
	mustCount("eks", "clusters", 1)
	mustCount("s3", "buckets", 1)
	mustCount("secretsmanager", "secrets", 1)
}
