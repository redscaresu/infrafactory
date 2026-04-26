package harness

import (
	"context"
	"fmt"
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

func TestEvaluatePlanPoliciesWithConstraints(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	policyPath := filepath.Join(tmp, "region.rego")
	policy := `package test.region

import rego.v1

deny contains msg if {
	allowed := input.constraints.region
	resource := input.planned_values.root_module.resources[_]
	region := resource.values.region
	region != allowed
	msg := sprintf("%s region %s not allowed", [resource.address, region])
}
`
	if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
		t.Fatalf("write policy fixture: %v", err)
	}

	planJSON := []byte(`{
  "planned_values": {
    "root_module": {
      "resources": [
        {"address": "scaleway_instance_server.web", "values": {"region": "nl-ams"}}
      ]
    }
  }
}`)

	failures, err := EvaluatePlanPoliciesWithConstraints(
		context.Background(),
		planJSON,
		map[string]any{"region": "fr-par"},
		[]string{policyPath},
	)
	if err != nil {
		t.Fatalf("evaluate policies: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("expected one failure, got %d (%+v)", len(failures), failures)
	}
	expected := "scaleway_instance_server.web region nl-ams not allowed"
	if failures[0].Detail != expected {
		t.Fatalf("expected detail %q, got %q", expected, failures[0].Detail)
	}
}

func TestScalewayPoliciesPlanEvaluation(t *testing.T) {
	t.Parallel()

	policiesRoot := filepath.Join("..", "..", "policies", "scaleway")
	cases := []struct {
		name          string
		policy        string
		planJSON      string
		constraints   map[string]any
		expectedCount int
	}{
		{
			name:   "region restriction triggers with constraints",
			policy: filepath.Join(policiesRoot, "region_restriction.rego"),
			planJSON: `{
  "planned_values": {"root_module": {"resources": [
    {"address":"scaleway_instance_server.web","values":{"region":"nl-ams"}}
  ]}}
}`,
			constraints:   map[string]any{"region": "fr-par"},
			expectedCount: 1,
		},
		{
			name:   "vpc required passes when private nic references server",
			policy: filepath.Join(policiesRoot, "vpc_required.rego"),
			planJSON: `{
  "planned_values": {"root_module": {"resources": [
    {"address":"scaleway_instance_server.web","type":"scaleway_instance_server","values":{}}
  ]}},
  "configuration": {"root_module": {"resources": [
    {"type":"scaleway_instance_private_nic","expressions":{"server_id":{"references":["scaleway_instance_server.web.id"]}}}
  ]}}
}`,
			expectedCount: 0,
		},
		{
			name:   "vpc required fails without private nic references",
			policy: filepath.Join(policiesRoot, "vpc_required.rego"),
			planJSON: `{
  "planned_values": {"root_module": {"resources": [
    {"address":"scaleway_instance_server.web","type":"scaleway_instance_server","values":{}}
  ]}},
  "configuration": {"root_module": {"resources": []}}
}`,
			expectedCount: 1,
		},
		{
			name:   "no public endpoints checks server attribute",
			policy: filepath.Join(policiesRoot, "no_public_endpoints.rego"),
			planJSON: `{
  "planned_values": {"root_module": {"resources": [
    {"address":"scaleway_instance_ip.public","type":"scaleway_instance_ip","values":{"server":"srv-id"}}
  ]}}
}`,
			expectedCount: 1,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			failures, err := EvaluatePlanPoliciesWithConstraints(
				context.Background(),
				[]byte(tc.planJSON),
				tc.constraints,
				[]string{tc.policy},
			)
			if err != nil {
				t.Fatalf("evaluate policy: %v", err)
			}
			if got := len(failures); got != tc.expectedCount {
				t.Fatalf("expected %d failures, got %d (%s)", tc.expectedCount, got, fmt.Sprintf("%+v", failures))
			}
		})
	}
}

// TestGCPPoliciesScopedToGoogleResources guards the P0 fix where
// policies/gcp/region_restriction.rego previously fired on every
// resource (including Scaleway ones) and broke the default-config
// Scaleway run path. Each GCP policy must only deny google_* resource
// types.
func TestGCPPoliciesScopedToGoogleResources(t *testing.T) {
	t.Parallel()

	policiesRoot := filepath.Join("..", "..", "policies", "gcp")
	policies := []string{
		filepath.Join(policiesRoot, "region_restriction.rego"),
		filepath.Join(policiesRoot, "no_public_sql.rego"),
		filepath.Join(policiesRoot, "vpc_required.rego"),
		filepath.Join(policiesRoot, "encryption.rego"),
	}

	// A plan with only Scaleway resources MUST produce zero GCP-policy
	// denials. region_restriction in particular used to deny `nl-ams`
	// and `pl-waw` regions because the rule didn't scope to google_*.
	scalewayOnlyPlan := `{
  "planned_values": {"root_module": {"resources": [
    {"address":"scaleway_instance_server.web","type":"scaleway_instance_server","values":{"region":"nl-ams","zone":"nl-ams-1"}},
    {"address":"scaleway_rdb_instance.main","type":"scaleway_rdb_instance","values":{"region":"pl-waw","encryption_at_rest":false}}
  ]}}
}`

	failures, err := EvaluatePlanPoliciesWithConstraints(
		context.Background(),
		[]byte(scalewayOnlyPlan),
		nil,
		policies,
	)
	if err != nil {
		t.Fatalf("evaluate gcp policies: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("expected zero gcp-policy denials on a Scaleway-only plan, got %d:\n%+v", len(failures), failures)
	}
}

// TestGCPRegionRestrictionDeniesOutsideAllowlist confirms the policy
// still fires when it should — a google_compute_instance in an
// out-of-allowlist region.
func TestGCPRegionRestrictionDeniesOutsideAllowlist(t *testing.T) {
	t.Parallel()

	policy := filepath.Join("..", "..", "policies", "gcp", "region_restriction.rego")
	plan := `{
  "planned_values": {"root_module": {"resources": [
    {"address":"google_compute_instance.web","type":"google_compute_instance","values":{"zone":"asia-east1-a"}},
    {"address":"google_sql_database_instance.main","type":"google_sql_database_instance","values":{"region":"asia-east1"}}
  ]}}
}`

	failures, err := EvaluatePlanPoliciesWithConstraints(
		context.Background(),
		[]byte(plan),
		nil,
		[]string{policy},
	)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(failures) == 0 {
		t.Fatalf("expected at least one denial for asia-east1 outside allowlist, got 0")
	}
}

func TestScalewayEncryptionPolicyMatchesEncryptionSemantics(t *testing.T) {
	t.Parallel()

	policy := filepath.Join("..", "..", "policies", "scaleway", "encryption_at_rest.rego")
	cases := []struct {
		name          string
		planJSON      string
		expectedCount int
	}{
		{
			name: "rdb encryption disabled fails",
			planJSON: `{
  "planned_values": {"root_module": {"resources": [
    {"address":"scaleway_rdb_instance.db","type":"scaleway_rdb_instance","values":{"encryption_at_rest": false}}
  ]}}
}`,
			expectedCount: 1,
		},
		{
			name: "bucket without versioning does not fail encryption policy",
			planJSON: `{
  "planned_values": {"root_module": {"resources": [
    {"address":"scaleway_object_bucket.logs","type":"scaleway_object_bucket","values":{"versioning": false}}
  ]}}
}`,
			expectedCount: 0,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			failures, err := EvaluatePlanPoliciesWithConstraints(context.Background(), []byte(tc.planJSON), nil, []string{policy})
			if err != nil {
				t.Fatalf("evaluate policy: %v", err)
			}
			if got := len(failures); got != tc.expectedCount {
				t.Fatalf("expected %d failures, got %d (%+v)", tc.expectedCount, got, failures)
			}
		})
	}
}

func TestCommonNamingPolicyAllowsSingleCharacterNames(t *testing.T) {
	t.Parallel()

	policy := filepath.Join("..", "..", "policies", "common", "naming.rego")
	cases := []struct {
		name          string
		resourceName  string
		expectedCount int
	}{
		{name: "single character passes", resourceName: "a", expectedCount: 0},
		{name: "trailing hyphen fails", resourceName: "a-", expectedCount: 1},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			planJSON := fmt.Sprintf(`{
  "planned_values": {"root_module": {"resources": [
    {"address":"scaleway_instance_server.web","type":"scaleway_instance_server","values":{"name":"%s"}}
  ]}}
}`, tc.resourceName)
			failures, err := EvaluatePlanPoliciesWithConstraints(context.Background(), []byte(planJSON), nil, []string{policy})
			if err != nil {
				t.Fatalf("evaluate policy: %v", err)
			}
			if got := len(failures); got != tc.expectedCount {
				t.Fatalf("expected %d failures, got %d (%+v)", tc.expectedCount, got, failures)
			}
		})
	}
}

// TestCommonNamingPolicyGCPExemptions pins the narrow set of
// resource-type-gated exemptions added when GCP coverage landed.
// A regression in the heuristic (e.g. accidentally exempting every
// "projects/" name, or every trailing-dot name) would let real
// misconfigurations slip through; this table-driven test makes
// that surface immediately.
func TestCommonNamingPolicyGCPExemptions(t *testing.T) {
	t.Parallel()

	policy := filepath.Join("..", "..", "policies", "common", "naming.rego")
	cases := []struct {
		name          string
		resourceType  string
		resourceName  string
		expectedCount int
	}{
		// google_secret_manager_secret.name is server-assigned to the
		// fully-qualified path — the slug check must skip it.
		{
			name:          "secret manager full path passes",
			resourceType:  "google_secret_manager_secret",
			resourceName:  "projects/p/secrets/db-credentials",
			expectedCount: 0,
		},
		// google_secret_manager_secret_version.name is also a path —
		// "projects/.../secrets/.../versions/<n>".
		{
			name:          "secret manager version full path passes",
			resourceType:  "google_secret_manager_secret_version",
			resourceName:  "projects/p/secrets/db-credentials/versions/1",
			expectedCount: 0,
		},
		// google_service_account.name is "projects/.../serviceAccounts/email".
		{
			name:          "service account full path passes",
			resourceType:  "google_service_account",
			resourceName:  "projects/p/serviceAccounts/ci@p.iam.gserviceaccount.com",
			expectedCount: 0,
		},
		// google_service_account_key.name embeds the parent SA path
		// + a uuid.
		{
			name:          "service account key full path passes",
			resourceType:  "google_service_account_key",
			resourceName:  "projects/p/serviceAccounts/ci@p.iam.gserviceaccount.com/keys/abc",
			expectedCount: 0,
		},
		// google_dns_record_set.name is an FQDN — trailing dot is
		// expected, not a misconfiguration.
		{
			name:          "dns record set FQDN passes",
			resourceType:  "google_dns_record_set",
			resourceName:  "host.example.invalid.",
			expectedCount: 0,
		},
		// google_dns_managed_zone.name is a slug, NOT an FQDN. A
		// trailing dot is a real mistake and must fail.
		{
			name:          "dns managed zone trailing dot fails",
			resourceType:  "google_dns_managed_zone",
			resourceName:  "zone.",
			expectedCount: 1,
		},
		// A non-exempt resource with a name starting "projects/" is
		// a typo — must fail.
		{
			name:          "pubsub topic projects-prefixed fails",
			resourceType:  "google_pubsub_topic",
			resourceName:  "projects/p/topics/events",
			expectedCount: 1,
		},
		// A non-exempt resource with a trailing dot is a typo too.
		{
			name:          "storage bucket trailing dot fails",
			resourceType:  "google_storage_bucket",
			resourceName:  "bucket.",
			expectedCount: 1,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			planJSON := fmt.Sprintf(`{
  "planned_values": {"root_module": {"resources": [
    {"address":"%s.x","type":"%s","values":{"name":"%s"}}
  ]}}
}`, tc.resourceType, tc.resourceType, tc.resourceName)
			failures, err := EvaluatePlanPoliciesWithConstraints(context.Background(), []byte(planJSON), nil, []string{policy})
			if err != nil {
				t.Fatalf("evaluate policy: %v", err)
			}
			if got := len(failures); got != tc.expectedCount {
				t.Fatalf("expected %d failures, got %d (%+v)", tc.expectedCount, got, failures)
			}
		})
	}
}
