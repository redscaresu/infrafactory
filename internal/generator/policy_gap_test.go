package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDetectPolicyConflict_ScalewayVPCRequiredCountBased pins the
// motivating case for N8. The 2026-06-01 deterministic sweep saw
// web-app-paris fail twice with the same policy=scaleway.vpc_required
// violation despite the LLM's HCL matching the prescriptive
// scaleway_instance_server pitfall verbatim. Pre-N8, the system
// terminated `stuck` with no actionable signal; post-N8, the
// detector emits a PolicyGap pointing at the rego file.
func TestDetectPolicyConflict_ScalewayVPCRequiredCountBased(t *testing.T) {
	failureDetail := `check=policy policy=scaleway.vpc_required command="validate" detail="scaleway_instance_server.web[0] is not attached to a private network via scaleway_instance_private_nic"`
	// LLM's HCL — matches the prescriptive pitfall verbatim.
	hcl := `
resource "scaleway_instance_server" "web" {
  count = 2
  name  = "web-${count.index}"
  type  = "DEV1-S"
}
resource "scaleway_instance_private_nic" "web" {
  count              = 2
  server_id          = scaleway_instance_server.web[count.index].id
  private_network_id = scaleway_vpc_private_network.main.id
}
`
	pitfalls := []PitfallEntry{
		{
			Resource: "scaleway_instance_server",
			Rule:     "Always declare a `scaleway_vpc_private_network` AND a `scaleway_instance_private_nic` for EACH `scaleway_instance_server`. Set `server_id = scaleway_instance_server.SERVER.id` and `private_network_id = scaleway_vpc_private_network.PN.id`.",
		},
	}
	gap := DetectPolicyConflict("", failureDetail, hcl, pitfalls, "scaleway", "web-app-paris", "20260601T130000Z")
	if gap == nil {
		t.Fatal("expected a PolicyGap for matching HCL + policy fire (legacy `policy=` prefix in detail), got nil")
	}
	if gap.Policy != "scaleway.vpc_required" {
		t.Errorf("policy = %q, want scaleway.vpc_required", gap.Policy)
	}
	if gap.Resource != "scaleway_instance_server" {
		t.Errorf("resource = %q, want scaleway_instance_server", gap.Resource)
	}
	if gap.Cloud != "scaleway" {
		t.Errorf("cloud = %q", gap.Cloud)
	}
}

// TestDetectPolicyConflict_LLMMissingKeywords confirms the detector
// returns nil when the LLM ISN'T following the pitfall. The system
// should still let the existing pitfall path handle it (the LLM is
// the one to fix, not the policy).
func TestDetectPolicyConflict_LLMMissingKeywords(t *testing.T) {
	failureDetail := `policy=scaleway.vpc_required detail="scaleway_instance_server.web[0] is not attached..."`
	// LLM's HCL is missing scaleway_instance_private_nic entirely —
	// real LLM mistake, not a policy bug.
	hcl := `resource "scaleway_instance_server" "web" { count = 2 }`
	pitfalls := []PitfallEntry{
		{
			Resource: "scaleway_instance_server",
			Rule:     "Always declare a `scaleway_vpc_private_network` AND a `scaleway_instance_private_nic`.",
		},
	}
	gap := DetectPolicyConflict("", failureDetail, hcl, pitfalls, "scaleway", "scenario", "ts")
	if gap != nil {
		t.Errorf("expected nil (LLM missing keywords), got %+v", gap)
	}
}

// TestDetectPolicyConflict_NoMatchingPitfall returns nil when no
// prescriptive pitfall exists for the resource. Without a pitfall to
// compare against, the detector has no signal to declare a conflict.
// The existing learning path runs unchanged.
func TestDetectPolicyConflict_NoMatchingPitfall(t *testing.T) {
	failureDetail := `policy=gcp.encryption detail="google_storage_bucket.app_assets has no encryption.default_kms_key_name"`
	hcl := `resource "google_storage_bucket" "app_assets" {}`
	pitfalls := []PitfallEntry{
		{Resource: "google_compute_instance", Rule: "set network"},
	}
	gap := DetectPolicyConflict("", failureDetail, hcl, pitfalls, "gcp", "scenario", "ts")
	if gap != nil {
		t.Errorf("expected nil (no matching pitfall), got %+v", gap)
	}
}

// TestDetectPolicyConflict_NotAPolicyFailure returns nil when the
// failure detail isn't a policy violation. Other failure shapes go
// to their existing channels (pitfalls/mock-gaps).
func TestDetectPolicyConflict_NotAPolicyFailure(t *testing.T) {
	failureDetail := `Error: Unsupported argument named "deletion_protection"`
	hcl := `whatever`
	pitfalls := []PitfallEntry{{Resource: "scaleway_instance_server", Rule: "Use `scaleway_instance_private_nic`"}}
	gap := DetectPolicyConflict("", failureDetail, hcl, pitfalls, "scaleway", "scenario", "ts")
	if gap != nil {
		t.Errorf("expected nil (not a policy failure), got %+v", gap)
	}
}

// TestDetectPolicyConflict_StructuredPolicyField pins the real
// shape observed in the 2026-06-01 sweep: FailureSummary.Policy is
// populated ("scaleway.vpc_required") but Detail contains only the
// rego deny message ("scaleway_instance_server.api[0] is not
// attached..."). Pre-fix, the regex required `policy=X.Y` in Detail
// and missed every real failure.
func TestDetectPolicyConflict_StructuredPolicyField(t *testing.T) {
	policy := "scaleway.vpc_required"
	failureDetail := `scaleway_instance_server.api[0] is not attached to a private network via scaleway_instance_private_nic`
	hcl := `
resource "scaleway_instance_server" "api" {
  count = 2
}
resource "scaleway_instance_private_nic" "api_nic" {
  count              = 2
  server_id          = scaleway_instance_server.api[count.index].id
  private_network_id = scaleway_vpc_private_network.main.id
}
`
	pitfalls := []PitfallEntry{
		{
			Resource: "scaleway_instance_server",
			Rule:     "Always declare a `scaleway_vpc_private_network` AND a `scaleway_instance_private_nic` for EACH `scaleway_instance_server`.",
		},
	}
	gap := DetectPolicyConflict(policy, failureDetail, hcl, pitfalls, "scaleway", "compute-lb-multi-paris", "20260601T140659Z")
	if gap == nil {
		t.Fatal("expected a PolicyGap when policy passed as separate arg, got nil")
	}
	if gap.Policy != "scaleway.vpc_required" {
		t.Errorf("policy = %q, want scaleway.vpc_required", gap.Policy)
	}
	if gap.Resource != "scaleway_instance_server" {
		t.Errorf("resource = %q, want scaleway_instance_server", gap.Resource)
	}
}

// TestExtractKeywords filters out narrative words and short tokens.
func TestExtractKeywords(t *testing.T) {
	rule := "Set `server_id` and `private_network_id` on `scaleway_instance_private_nic`. Do `not` use `for` empty."
	got := extractKeywords(rule)
	want := map[string]bool{
		"server_id":                     true,
		"private_network_id":            true,
		"scaleway_instance_private_nic": true,
	}
	if len(got) != len(want) {
		t.Errorf("expected %d keywords, got %d: %v", len(want), len(got), got)
	}
	for _, k := range got {
		if !want[k] {
			t.Errorf("unexpected keyword %q (should have been skipped)", k)
		}
	}
}

// TestAppendPolicyGap_CreatesFileAndDedups exercises the writer's
// initial header, per-cloud section, dedup, and multi-cloud sections.
func TestAppendPolicyGap_CreatesFileAndDedups(t *testing.T) {
	dir := t.TempDir()
	gap := PolicyGap{
		Cloud:     "scaleway",
		Policy:    "scaleway.vpc_required",
		Resource:  "scaleway_instance_server",
		Scenario:  "web-app-paris",
		Detail:    "scaleway_instance_server.web[0] is not attached...",
		Timestamp: "20260601T130000Z",
	}
	if err := AppendPolicyGap(dir, gap); err != nil {
		t.Fatalf("first append: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "policy-gaps.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "# Policy gaps") {
		t.Errorf("missing header")
	}
	if !strings.Contains(bodyStr, "## scaleway") {
		t.Errorf("missing scaleway section")
	}
	if !strings.Contains(bodyStr, "scaleway.vpc_required") {
		t.Errorf("missing policy name")
	}

	// Dedup on (policy, resource).
	if err := AppendPolicyGap(dir, gap); err != nil {
		t.Fatalf("second append: %v", err)
	}
	body2, _ := os.ReadFile(filepath.Join(dir, "policy-gaps.md"))
	if strings.Count(string(body2), "scaleway.vpc_required") != 1 {
		t.Errorf("duplicate row on re-append, got %d occurrences", strings.Count(string(body2), "scaleway.vpc_required"))
	}

	// Different resource under same cloud appends to same section.
	gap2 := gap
	gap2.Resource = "scaleway_lb_backend"
	if err := AppendPolicyGap(dir, gap2); err != nil {
		t.Fatalf("third append: %v", err)
	}
	body3, _ := os.ReadFile(filepath.Join(dir, "policy-gaps.md"))
	if !strings.Contains(string(body3), "scaleway_lb_backend") {
		t.Errorf("second resource not appended")
	}
	if strings.Count(string(body3), "## scaleway") != 1 {
		t.Errorf("scaleway heading duplicated")
	}

	// Different cloud adds its own section.
	gap3 := PolicyGap{
		Cloud:     "gcp",
		Policy:    "gcp.encryption",
		Resource:  "google_storage_bucket",
		Scenario:  "gcp-storage",
		Detail:    "...",
		Timestamp: "ts",
	}
	if err := AppendPolicyGap(dir, gap3); err != nil {
		t.Fatalf("gcp append: %v", err)
	}
	body4, _ := os.ReadFile(filepath.Join(dir, "policy-gaps.md"))
	if !strings.Contains(string(body4), "## gcp") {
		t.Errorf("gcp section missing")
	}
}
