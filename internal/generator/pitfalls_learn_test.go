package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestExtractLearnedPitfall_PasswordConstraint(t *testing.T) {
	detail := `Error: scaleway_redis_cluster.main: password does not respect constraint: minimum 8 characters`
	got := ExtractLearnedPitfall(detail, "redis-paris")
	if got == nil {
		t.Fatal("expected pitfall, got nil")
	}
	if got.Resource != "scaleway_redis_cluster" {
		t.Errorf("resource = %q, want scaleway_redis_cluster", got.Resource)
	}
	if got.DiscoveredFrom != "redis-paris" {
		t.Errorf("discovered_from = %q, want redis-paris", got.DiscoveredFrom)
	}
	if got.Rule == "" {
		t.Error("expected non-empty rule")
	}
}

func TestExtractLearnedPitfall_K8sVersionAutoUpgrade(t *testing.T) {
	detail := `exit status 1 | stderr: Error: minor version x.y must only be used with auto upgrade enabled with scaleway_k8s_cluster.main`
	got := ExtractLearnedPitfall(detail, "k8s-cluster-paris")
	if got == nil {
		t.Fatal("expected pitfall, got nil")
	}
	if got.Resource != "scaleway_k8s_cluster" {
		t.Errorf("resource = %q, want scaleway_k8s_cluster", got.Resource)
	}
	if got.Rule == "" {
		t.Error("expected non-empty rule")
	}
}

// TestExtractLearnedPitfall_GoogleResource pins multi-cloud regex
// support: a GCP-flavoured failure detail must extract the google_*
// resource type so the run-loop's cross-cloud guard accepts the
// pitfall on a `cloud: gcp` scenario.
func TestExtractLearnedPitfall_GoogleResource(t *testing.T) {
	detail := `Error: Unsupported argument "labels" on google_container_cluster.main`
	got := ExtractLearnedPitfall(detail, "gcp-gke-cluster")
	if got == nil {
		t.Fatal("expected pitfall, got nil")
	}
	if got.Resource != "google_container_cluster" {
		t.Errorf("resource = %q, want google_container_cluster", got.Resource)
	}
}

// TestExtractLearnedPitfall_PasswordWithoutResourceReturnsNil pins the
// review-11 fix: when the password failure detail names no resource,
// the function must NOT fabricate a `scaleway_redis_cluster` default.
// Otherwise the run loop's cross-cloud guard would silently drop the
// learning on GCP.
func TestExtractLearnedPitfall_PasswordWithoutResourceReturnsNil(t *testing.T) {
	detail := `password does not respect constraint`
	if got := ExtractLearnedPitfall(detail, "any-scenario"); got != nil {
		t.Fatalf("expected nil for resource-less password failure, got %+v", got)
	}
}

func TestExtractLearnedPitfall_UnsupportedArgument(t *testing.T) {
	detail := `Error: Unsupported argument "zone" on scaleway_lb_backend.main`
	got := ExtractLearnedPitfall(detail, "web-app-paris")
	if got == nil {
		t.Fatal("expected pitfall, got nil")
	}
	if got.Resource != "scaleway_lb_backend" {
		t.Errorf("resource = %q, want scaleway_lb_backend", got.Resource)
	}
	if got.DiscoveredFrom != "web-app-paris" {
		t.Errorf("discovered_from = %q, want web-app-paris", got.DiscoveredFrom)
	}
	if got.Rule == "" {
		t.Error("expected non-empty rule")
	}
}

func TestExtractLearnedPitfall_AtLeastOneOf(t *testing.T) {
	detail := `Error: scaleway_rdb_instance.main: at least one of 'ip_net' or 'enable_ipam' (set to true) must be set`
	got := ExtractLearnedPitfall(detail, "rdb-paris")
	if got == nil {
		t.Fatal("expected pitfall, got nil")
	}
	if got.Resource != "scaleway_rdb_instance" {
		t.Errorf("resource = %q, want scaleway_rdb_instance", got.Resource)
	}
	if got.Rule == "" {
		t.Error("expected non-empty rule")
	}
}

// TestExtractLearnedPitfall_M97Templates pins each of the 5 M97
// prescriptive-rule templates. Each test feeds a real-shape failure
// detail and asserts the produced rule is PRESCRIPTIVE — contains a
// verb the LLM can act on ("Set", "Add", "Omit", "Do NOT use") —
// not just a descriptive echo of the failure.
func TestExtractLearnedPitfall_M97_MissingSubnetwork(t *testing.T) {
	cases := []string{
		`google_compute_instance.api_server has no network_interface.subnetwork — must be attached to an explicit VPC subnetwork`,
		`google_container_cluster.primary has no network or subnetwork — GKE clusters must reference an explicit VPC`,
	}
	for _, detail := range cases {
		got := ExtractLearnedPitfall(detail, "gcp-full-stack")
		if got == nil {
			t.Errorf("nil for %q", detail[:50])
			continue
		}
		if !strings.Contains(got.Rule, "google_compute_subnetwork") {
			t.Errorf("rule not prescriptive enough — missing 'google_compute_subnetwork' for %q", detail[:50])
		}
	}
}

func TestExtractLearnedPitfall_M97_MissingEncryption_Disabled(t *testing.T) {
	// M97 follow-up: this template is intentionally a no-op until M98
	// lands cross-policy awareness. Previous version told the LLM to
	// "omit CMEK" but policies/gcp/encryption.rego REQUIRES CMEK —
	// giving the LLM the opposite of what the gate enforces poisoned
	// the learning loop. Pin the disabled state so we don't silently
	// re-enable wrong advice.
	detail := `google_storage_bucket.app_assets has no encryption.default_kms_key_name — customer-managed encryption not configured`
	got := matchMissingEncryption(detail, "gcp-storage")
	if got != nil {
		t.Fatalf("CMEK template should be disabled (M97 follow-up); got: %+v", got)
	}
	// The descriptive fallback still produces SOMETHING for this detail
	// (so the LLM still sees the failure in context); just not the
	// wrong prescriptive form.
	fallback := ExtractLearnedPitfall(detail, "gcp-storage")
	if fallback == nil {
		t.Fatal("expected descriptive fallback for encryption detail when template disabled")
	}
}

func TestExtractLearnedPitfall_M97_NotImplemented(t *testing.T) {
	detail := `exit status 1 | stderr: Error creating instance template: googleapi: Error 501: Not implemented for google_compute_instance_template`
	got := ExtractLearnedPitfall(detail, "gcp-iam")
	if got == nil {
		t.Fatal("nil for 501 shape")
	}
	if !strings.Contains(got.Rule, "Do NOT use") {
		t.Errorf("rule not prescriptive: %q", got.Rule)
	}
}

func TestExtractLearnedPitfall_M97_OAuthEscape(t *testing.T) {
	detail := `Error creating google_service_account: googleapi: Error 401: Request had invalid authentication credentials. Expected OAuth 2 access token`
	got := ExtractLearnedPitfall(detail, "gcp-iam")
	if got == nil {
		t.Fatal("nil for OAuth-escape shape")
	}
	if !strings.Contains(got.Rule, "custom_endpoint") {
		t.Errorf("rule not prescriptive: %q", got.Rule)
	}
}

func TestExtractLearnedPitfall_M97_DestroyBlockers(t *testing.T) {
	// deletion_protection
	got := ExtractLearnedPitfall(`Error destroying aws_db_instance.main: deletion_protection is enabled, set to true`, "aws-rds")
	if got == nil || !strings.Contains(got.Rule, "deletion_protection = false") {
		t.Errorf("deletion_protection template failed: %+v", got)
	}
	// BucketNotEmpty
	got = ExtractLearnedPitfall(`Error destroying aws_s3_bucket.assets: BucketNotEmpty: The bucket you tried to delete is not empty`, "aws-s3")
	if got == nil || !strings.Contains(got.Rule, "force_destroy = true") {
		t.Errorf("force_destroy template failed: %+v", got)
	}
	// skip_final_snapshot
	got = ExtractLearnedPitfall(`Error destroying aws_db_instance.main: final snapshot must be specified or skip_final_snapshot = true`, "aws-rds")
	if got == nil || !strings.Contains(got.Rule, "skip_final_snapshot") {
		t.Errorf("skip_final_snapshot template failed: %+v", got)
	}
}

// TestExtractLearnedPitfall_AwsResource pins the M92 fix: the resource
// regex was scaleway_*|google_* only, so every AWS failure detail
// produced "no resource extracted" and the learning loop silently
// dropped it. M88's sweep showed 11/11 AWS scenarios failed without
// growing pitfalls/aws.yaml even with M86+M90 active.
func TestExtractLearnedPitfall_AwsResource(t *testing.T) {
	detail := "exit status 1 | stderr: Error: invalid value for aws_db_instance.main: deletion_protection must be false to destroy. Resource: aws_db_instance.main"
	got := ExtractLearnedPitfall(detail, "aws-rds")
	if got == nil {
		t.Fatal("M92 regression: AWS resource not extracted from tofu envelope")
	}
	if got.Resource != "aws_db_instance" {
		t.Errorf("resource = %q, want aws_db_instance", got.Resource)
	}
}

// TestExtractLearnedPitfall_TofuEnvelopeWithResource regression-pins
// the M86 bug fix. Every tofu apply failure detail starts with
// "exit status 1 | stderr: ..." — "exit status" is in genericPatterns.
// The prior ordering substring-rejected the whole detail before the
// resource-extraction fallback could fire, so every apply-time
// learning was silently dropped. The fix runs the resource-extraction
// fallback BEFORE the generic-pattern rejection so the actionable
// google_*/scaleway_* substring is honored.
func TestExtractLearnedPitfall_TofuEnvelopeWithResource(t *testing.T) {
	// Real M83 iter-1 failure detail (gcp-memorystore run).
	detail := "exit status 1 | stderr: Error when reading or editing Project Service infrafactory-test/redis.googleapis.com: googleapi: Error 401: Request had invalid authentication credentials. Resource: google_project_service.redis"
	got := ExtractLearnedPitfall(detail, "gcp-memorystore")
	if got == nil {
		t.Fatal("expected learned pitfall for tofu envelope with google_project_service in detail, got nil — M86 ordering bug has regressed")
	}
	if got.Resource != "google_project_service" {
		t.Errorf("resource = %q, want google_project_service", got.Resource)
	}
	if got.Rule == "" {
		t.Error("expected non-empty rule")
	}
}

func TestExtractLearnedPitfall_GenericError(t *testing.T) {
	cases := []string{
		"test checks failed",
		"validation failed with exit status 1",
		"exit status 1",
		"context deadline exceeded",
		"command failed",
	}
	for _, detail := range cases {
		got := ExtractLearnedPitfall(detail, "test-scenario")
		if got != nil {
			t.Errorf("detail=%q: expected nil, got %+v", detail, got)
		}
	}
}

func TestExtractLearnedPitfall_ResourceNotFound(t *testing.T) {
	detail := "resource with ID abc-123 not found"
	got := ExtractLearnedPitfall(detail, "test-scenario")
	if got != nil {
		t.Errorf("expected nil for generic not found, got %+v", got)
	}
}

func TestExtractLearnedPitfall_EmptyDetail(t *testing.T) {
	got := ExtractLearnedPitfall("", "test-scenario")
	if got != nil {
		t.Errorf("expected nil for empty detail, got %+v", got)
	}
}

func TestAppendPitfall_NewPitfall(t *testing.T) {
	dir := t.TempDir()

	// Seed with an existing pitfall.
	initial := PitfallsFile{
		Provider: "scaleway",
		Pitfalls: []PitfallEntry{
			{Resource: "scaleway_instance_server", Rule: "Use exact instance type.", Source: "static"},
		},
	}
	data, err := yaml.Marshal(&initial)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scaleway.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	pitfall := LearnedPitfall{
		Resource:       "scaleway_redis_cluster",
		Rule:           "The password must meet complexity requirements.",
		DiscoveredFrom: "redis-paris",
	}
	if err := AppendPitfall(dir, "scaleway", pitfall); err != nil {
		t.Fatal(err)
	}

	// Verify file is valid YAML with 2 entries.
	result, err := os.ReadFile(filepath.Join(dir, "scaleway.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var pf PitfallsFile
	if err := yaml.Unmarshal(result, &pf); err != nil {
		t.Fatalf("invalid YAML after append: %v", err)
	}
	if len(pf.Pitfalls) != 2 {
		t.Fatalf("expected 2 pitfalls, got %d", len(pf.Pitfalls))
	}
	added := pf.Pitfalls[1]
	if added.Source != "learned" {
		t.Errorf("source = %q, want learned", added.Source)
	}
	if added.DiscoveredFrom != "redis-paris" {
		t.Errorf("discovered_from = %q, want redis-paris", added.DiscoveredFrom)
	}
}

func TestAppendPitfall_Duplicate(t *testing.T) {
	dir := t.TempDir()

	initial := PitfallsFile{
		Provider: "scaleway",
		Pitfalls: []PitfallEntry{
			{
				Resource: "scaleway_redis_cluster",
				Rule:     "The password must meet complexity requirements including uppercase, lowercase, digit, special character.",
				Source:   "learned",
			},
		},
	}
	data, err := yaml.Marshal(&initial)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scaleway.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Try to append a similar pitfall — should be deduplicated.
	pitfall := LearnedPitfall{
		Resource:       "scaleway_redis_cluster",
		Rule:           "The password must meet the provider's complexity requirements (minimum length, uppercase, lowercase, digit, and special character).",
		DiscoveredFrom: "redis-paris-2",
	}
	if err := AppendPitfall(dir, "scaleway", pitfall); err != nil {
		t.Fatal(err)
	}

	result, err := os.ReadFile(filepath.Join(dir, "scaleway.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var pf PitfallsFile
	if err := yaml.Unmarshal(result, &pf); err != nil {
		t.Fatal(err)
	}
	if len(pf.Pitfalls) != 1 {
		t.Fatalf("expected 1 pitfall (duplicate skipped), got %d", len(pf.Pitfalls))
	}
}

func TestAppendPitfall_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "pitfalls")

	pitfall := LearnedPitfall{
		Resource:       "scaleway_redis_cluster",
		Rule:           "The password must meet complexity requirements.",
		DiscoveredFrom: "redis-paris",
	}
	if err := AppendPitfall(subDir, "scaleway", pitfall); err != nil {
		t.Fatal(err)
	}

	result, err := os.ReadFile(filepath.Join(subDir, "scaleway.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var pf PitfallsFile
	if err := yaml.Unmarshal(result, &pf); err != nil {
		t.Fatalf("invalid YAML: %v", err)
	}
	if pf.Provider != "scaleway" {
		t.Errorf("provider = %q, want scaleway", pf.Provider)
	}
	if len(pf.Pitfalls) != 1 {
		t.Fatalf("expected 1 pitfall, got %d", len(pf.Pitfalls))
	}
}

func TestAppendPitfall_EmptyDirOrCloud(t *testing.T) {
	if err := AppendPitfall("", "scaleway", LearnedPitfall{Rule: "test"}); err != nil {
		t.Errorf("empty dir should be no-op, got error: %v", err)
	}
	if err := AppendPitfall("/tmp", "", LearnedPitfall{Rule: "test"}); err != nil {
		t.Errorf("empty cloud should be no-op, got error: %v", err)
	}
}
