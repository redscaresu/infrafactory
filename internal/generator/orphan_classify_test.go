package generator

import (
	"strings"
	"testing"
)

// TestClassifyOrphans_AWSSecretsManagerSoftDelete pins the
// motivating case for N9: aws-full-stack iter 1 destroyed but left
// 1 row in secretsmanager.secrets because the LLM didn't set
// recovery_window_in_days = 0. The classifier must emit a
// prescriptive pitfall for aws_secretsmanager_secret (sub-shape #1).
func TestClassifyOrphans_AWSSecretsManagerSoftDelete(t *testing.T) {
	state := []byte(`{
		"secretsmanager": {
			"secrets": [{"arn": "arn:aws:secretsmanager:us-east-1:0:secret:x"}],
			"versions": []
		}
	}`)
	routing := ClassifyOrphans(state, "aws", "aws-full-stack", "20260601T130000Z")
	if len(routing.Pitfalls) != 1 {
		t.Fatalf("expected 1 pitfall, got %d", len(routing.Pitfalls))
	}
	p := routing.Pitfalls[0]
	if p.Resource != "aws_secretsmanager_secret" {
		t.Errorf("resource = %q, want aws_secretsmanager_secret", p.Resource)
	}
	if !strings.Contains(p.Rule, "recovery_window_in_days") {
		t.Errorf("rule missing the prescription: %q", p.Rule)
	}
	if len(routing.MockGaps) != 0 {
		t.Errorf("expected 0 mock gaps, got %d", len(routing.MockGaps))
	}
}

// TestClassifyOrphans_AWSKMSKeySoftDelete — same shape, different
// resource. force_destroy guidance should appear.
func TestClassifyOrphans_AWSKMSKeySoftDelete(t *testing.T) {
	state := []byte(`{
		"kms": {"keys": [{"key_id": "k1"}]}
	}`)
	routing := ClassifyOrphans(state, "aws", "aws-kms", "ts")
	if len(routing.Pitfalls) != 1 {
		t.Fatalf("expected 1 pitfall, got %d", len(routing.Pitfalls))
	}
	if routing.Pitfalls[0].Resource != "aws_kms_key" {
		t.Errorf("resource = %q, want aws_kms_key", routing.Pitfalls[0].Resource)
	}
	if !strings.Contains(routing.Pitfalls[0].Rule, "force_destroy") {
		t.Errorf("rule missing force_destroy guidance: %q", routing.Pitfalls[0].Rule)
	}
}

// TestClassifyOrphans_GCPSecretManagerSoftDelete confirms cross-
// cloud lookup isolates correctly — same service name "secretmanager"
// as GCP, distinct from AWS "secretsmanager".
func TestClassifyOrphans_GCPSecretManagerSoftDelete(t *testing.T) {
	state := []byte(`{"secretmanager": {"secrets": [{"name": "projects/x/secrets/y"}]}}`)
	routing := ClassifyOrphans(state, "gcp", "gcp-secret-manager", "ts")
	if len(routing.Pitfalls) != 1 {
		t.Fatalf("expected 1 pitfall, got %d", len(routing.Pitfalls))
	}
	if routing.Pitfalls[0].Resource != "google_secret_manager_secret" {
		t.Errorf("resource = %q", routing.Pitfalls[0].Resource)
	}
}

// TestClassifyOrphans_GCPKMSCryptoKeyProviderSoftDelete — sub-shape
// #5: provider-side soft-delete, routes to mock-gap channel.
func TestClassifyOrphans_GCPKMSCryptoKeyProviderSoftDelete(t *testing.T) {
	state := []byte(`{"kms": {"crypto_keys": [{"name": "k"}]}}`)
	routing := ClassifyOrphans(state, "gcp", "gcp-storage", "ts")
	if len(routing.Pitfalls) != 0 {
		t.Errorf("expected 0 pitfalls (mock-side fix), got %d", len(routing.Pitfalls))
	}
	if len(routing.MockGaps) != 1 {
		t.Fatalf("expected 1 mock gap, got %d", len(routing.MockGaps))
	}
	if routing.MockGaps[0].Cloud != "gcp" {
		t.Errorf("cloud = %q", routing.MockGaps[0].Cloud)
	}
	if !strings.Contains(routing.MockGaps[0].Signal, "google_kms_crypto_key") {
		t.Errorf("signal missing resource name: %q", routing.MockGaps[0].Signal)
	}
}

// TestClassifyOrphans_AWSSubnetMockProviderDivergence pins sub-shape
// #4: a known mock gap (T8 MapPublicIpOnLaunch persistence) routes
// to mock-gap channel, not pitfalls.
func TestClassifyOrphans_AWSSubnetMockProviderDivergence(t *testing.T) {
	state := []byte(`{"ec2": {"subnets": [{"subnet_id": "subnet-1"}]}}`)
	routing := ClassifyOrphans(state, "aws", "aws-vpc", "ts")
	if len(routing.Pitfalls) != 0 {
		t.Errorf("expected 0 pitfalls, got %d", len(routing.Pitfalls))
	}
	if len(routing.MockGaps) != 1 {
		t.Fatalf("expected 1 mock gap, got %d", len(routing.MockGaps))
	}
}

// TestClassifyOrphans_MultipleSubshapesInOneFailure exercises a run
// where destroy left orphans across two different resource types —
// the classifier must emit one entry per (resource, channel) pair.
func TestClassifyOrphans_MultipleSubshapesInOneFailure(t *testing.T) {
	state := []byte(`{
		"secretsmanager": {"secrets": [{"arn": "a"}]},
		"ec2": {"subnets": [{"subnet_id": "s"}]}
	}`)
	routing := ClassifyOrphans(state, "aws", "aws-full-stack", "ts")
	if len(routing.Pitfalls) != 1 {
		t.Errorf("expected 1 pitfall (secrets), got %d", len(routing.Pitfalls))
	}
	if len(routing.MockGaps) != 1 {
		t.Errorf("expected 1 mock gap (subnet), got %d", len(routing.MockGaps))
	}
}

// TestClassifyOrphans_UnclassifiedRecorded confirms unknown
// (cloud, service, collection) tuples land in Unclassified for
// human triage / table-seeding decisions rather than getting silently
// dropped.
func TestClassifyOrphans_UnclassifiedRecorded(t *testing.T) {
	state := []byte(`{"some_unknown_service": {"weird_collection": [{}, {}]}}`)
	routing := ClassifyOrphans(state, "aws", "scenario", "ts")
	if len(routing.Pitfalls) != 0 || len(routing.MockGaps) != 0 {
		t.Errorf("unknown should not emit pitfall or mock-gap")
	}
	if len(routing.Unclassified) != 2 {
		t.Errorf("expected 2 unclassified (per item), got %d", len(routing.Unclassified))
	}
}

// TestClassifyOrphans_IgnoresSystemRoots — bookkeeping/audit tables
// must never be classified as orphans (same exclusion set as
// countOrphans in the destroy harness).
func TestClassifyOrphans_IgnoresSystemRoots(t *testing.T) {
	state := []byte(`{
		"audit": {"entries": [{"op": "x"}]},
		"operations": {"operations": [{"id": "1"}, {"id": "2"}]},
		"metadata": {"version": 1}
	}`)
	routing := ClassifyOrphans(state, "aws", "scenario", "ts")
	if len(routing.Pitfalls) != 0 || len(routing.MockGaps) != 0 || len(routing.Unclassified) != 0 {
		t.Errorf("system roots must not surface — got pitfalls=%d gaps=%d unclassified=%d",
			len(routing.Pitfalls), len(routing.MockGaps), len(routing.Unclassified))
	}
}

// TestClassifyOrphans_EmptyOrMalformedInputIsSafe — defensive.
// Malformed JSON or missing fields return empty routing, not an
// error.
func TestClassifyOrphans_EmptyOrMalformedInputIsSafe(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		[]byte("not json"),
		[]byte("null"),
		[]byte(`{"iam": "not-an-object"}`),
	}
	for i, c := range cases {
		routing := ClassifyOrphans(c, "aws", "scenario", "ts")
		if len(routing.Pitfalls) != 0 || len(routing.MockGaps) != 0 {
			t.Errorf("case %d: expected empty routing, got pitfalls=%d gaps=%d",
				i, len(routing.Pitfalls), len(routing.MockGaps))
		}
	}
}

// TestClassifyOrphans_CrossCloudIsolation — same service name "kms"
// exists under aws and gcp with distinct sub-shapes (#1 vs #5).
// Lookup must scope by cloud or the wrong branch fires.
func TestClassifyOrphans_CrossCloudIsolation(t *testing.T) {
	state := []byte(`{"kms": {"keys": [{"k": 1}]}}`)
	awsRouting := ClassifyOrphans(state, "aws", "s", "t")
	if len(awsRouting.Pitfalls) != 1 {
		t.Errorf("aws.kms.keys should be sub-shape #1 (LLM soft-delete)")
	}
	// GCP uses "crypto_keys" not "keys" so this state with cloud=gcp
	// is unclassified (correct — different shape).
	gcpRouting := ClassifyOrphans(state, "gcp", "s", "t")
	if len(gcpRouting.Pitfalls) != 0 {
		t.Errorf("gcp lookup for aws-shaped data should not match")
	}
}
