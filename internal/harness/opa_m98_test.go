package harness

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestOPAPoliciesM98KnownAfterApplyBranches pins the M98 fix in
// the rego policies that read reference-typed attributes from
// `planned_values`. At plan time, fields like
// `network_interface[0].subnetwork = google_compute_subnetwork.X.id`
// resolve to `null` in `planned_values` because the subnetwork is
// `known after apply` — the literal value is unknown until apply.
// Without an `after_unknown` branch, the policy false-fires on
// correct HCL.
//
// The four affected policies (GCP + AWS encryption + vpc_required)
// have M98 fixes in place since 2026-05-23 / S60. This ratchet
// fails CI if a future edit drops the fix from any of them. AWS
// encryption reads the reference from `configuration` rather than
// `after_unknown`, because it must know which bucket is named.
//
// Policies that inspect literal fields (region restrictions, plain
// booleans like `encryption_at_rest`, block presence) don't need
// `after_unknown` — they're not in this guard.
func TestOPAPoliciesM98KnownAfterApplyBranches(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path       string
		wantTokens []string
	}{
		{
			path: "policies/gcp/vpc_required.rego",
			wantTokens: []string{
				"after_unknown",       // M98 fix present
				"network_interface",   // applied to compute instance subnet
				"has_cluster_network", // GKE cluster network/subnetwork branch
			},
		},
		{
			path: "policies/gcp/encryption.rego",
			wantTokens: []string{
				"after_unknown",
				"bucket_has_cmek",
				"sql_has_cmek",
				"disk_has_encryption",
			},
		},
		{
			path: "policies/aws/vpc_required.rego",
			wantTokens: []string{
				"after_unknown",
				"subnet_id",
				"db_subnet_group_name",
				"vpc_config", // EKS subnet_ids check
			},
		},
		{
			path: "policies/aws/encryption.rego",
			wantTokens: []string{
				"references", // M98 fix: SSE config's bucket reference
				"bucket",     // S3 / RDS / etc.
			},
		},
	}

	repoRoot := opaTestRepoRoot(t)
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			full := filepath.Join(repoRoot, tc.path)
			data, err := os.ReadFile(full)
			if err != nil {
				t.Fatalf("read %s: %v", full, err)
			}
			body := string(data)
			for _, want := range tc.wantTokens {
				if !strings.Contains(body, want) {
					t.Errorf("%s missing required token %q — M98 fix regression?\n"+
						"This policy reads reference-typed attributes, so it MUST also accept known-after-apply references "+
						"(an `after_unknown.X == true` branch, or the reference read from `configuration`). See ADR-0012 "+
						"§ M98 amendment for the pattern.", tc.path, want)
				}
			}
		})
	}
}

func opaTestRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}
