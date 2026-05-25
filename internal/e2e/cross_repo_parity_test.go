package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestCrossRepoParity_EveryLandedServiceHasScenario asserts that every
// service-id named in a sibling fake's `LandedServices` manifest is
// either:
//   - mapped to ≥1 infrafactory training scenario whose file is
//     present on disk, OR
//   - explicitly exempted (with a written reason) below.
//
// When a fake repo (fakeaws / fakegcp / mockway) lands a new
// service, this test fails until either:
//   - infrafactory adds a `scenarios/training/<cloud>-<service>.yaml`
//     AND a matching entry in cloudParityMap below, OR
//   - the service is explicitly added to exemptServices with a
//     reason (e.g., meta-services like GCP serviceusage that every
//     scenario implicitly exercises).
//
// The test reads each fake's `handlers/regression_manifest.go`
// directly from disk via regex so infrafactory doesn't need a Go-
// module dependency on the fakes (they're sibling repos checked out
// alongside infrafactory, not imports). If a sibling repo isn't
// present (CI checkouts that only fetch infrafactory), the per-cloud
// subtest skips with a structured marker.
//
// To regenerate cloudParityMap after adding scenarios, run:
//
//	UPDATE_CROSS_REPO_PARITY=1 go test -run TestCrossRepoParity ./internal/e2e/
//
// That mode prints the current scenarios-per-service inventory but
// does not write a file — the map below is intentionally hand-curated
// so reviewers see scope changes in PR diffs.
func TestCrossRepoParity_EveryLandedServiceHasScenario(t *testing.T) {
	t.Parallel()

	repoRoot := RepoRoot(t)
	scenariosDir := filepath.Join(repoRoot, "scenarios", "training")

	for _, fake := range fakeRepoSpecs() {
		fake := fake
		t.Run(fake.name, func(t *testing.T) {
			t.Parallel()

			manifestPath := filepath.Join(repoRoot, "..", fake.name, "handlers", "regression_manifest.go")
			payload, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Skipf("sibling repo %q not checked out at %s — skipping cross-repo parity check", fake.name, manifestPath)
				return
			}

			landed, err := parseLandedServices(payload)
			if err != nil {
				t.Fatalf("parse LandedServices from %s: %v", manifestPath, err)
			}
			if len(landed) == 0 {
				t.Fatalf("LandedServices is empty in %s — manifest parse likely broken", manifestPath)
			}

			var missing []string
			for _, service := range landed {
				if reason, exempt := fake.exempt[service]; exempt {
					if reason == "" {
						t.Errorf("service %q is in exempt list with empty reason — every exemption must explain why no scenario exists", service)
					}
					continue
				}
				scenarios, mapped := fake.mapping[service]
				if !mapped {
					missing = append(missing, service+" (no entry in cloudParityMap — add a scenario or an exemption)")
					continue
				}
				for _, scenario := range scenarios {
					if _, err := os.Stat(filepath.Join(scenariosDir, scenario)); err != nil {
						missing = append(missing, service+" → "+scenario+" (mapped but file missing)")
					}
				}
			}
			if len(missing) > 0 {
				sort.Strings(missing)
				t.Fatalf("%s landed services without infrafactory scenario coverage:\n  - %s\n\nFix by adding scenarios/training/<cloud>-<service>.yaml AND a cloudParityMap entry, OR by adding the service to exemptServices with a reason.", fake.name, strings.Join(missing, "\n  - "))
			}
		})
	}
}

type fakeRepoSpec struct {
	name    string
	mapping map[string][]string
	exempt  map[string]string
}

// fakeRepoSpecs returns the cross-repo parity definition: one entry
// per sibling fake. The mapping is intentionally hand-curated so PRs
// that add a new service to a fake force a visible diff here.
func fakeRepoSpecs() []fakeRepoSpec {
	return []fakeRepoSpec{
		{
			name: "fakeaws",
			mapping: map[string][]string{
				"dynamodb":       {"aws-dynamodb.yaml"},
				"ec2":            {"aws-instance.yaml", "aws-vpc-network.yaml"},
				"eks":            {"aws-eks.yaml"},
				"iam":            {"aws-iam.yaml"},
				"rds":            {"aws-rds.yaml"},
				"route53":        {"aws-route53.yaml"},
				"s3":             {"aws-s3.yaml"},
				"secretsmanager": {"aws-secrets-manager.yaml"},
				"sqs":            {"aws-sqs.yaml"},
			},
			exempt: map[string]string{},
		},
		{
			name: "fakegcp",
			mapping: map[string][]string{
				"cloudrun":      {"gcp-cloud-run.yaml"},
				"compute":       {"gcp-vm-network.yaml", "gcp-full-stack.yaml"},
				"container":     {"gcp-gke-cluster.yaml"},
				"dns":           {"gcp-dns.yaml"},
				"iam":           {"gcp-iam.yaml"},
				"loadbalancer":  {"gcp-load-balancer.yaml"},
				"memorystore":   {"gcp-memorystore.yaml"},
				"pubsub":        {"gcp-pubsub.yaml"},
				"secretmanager": {"gcp-secret-manager.yaml"},
				"sql":           {"gcp-cloud-sql.yaml"},
				"storage":       {"gcp-storage.yaml"},
			},
			exempt: map[string]string{
				// serviceusage is the GCP meta-API that enables/disables
				// other APIs per project — terraform-provider-google
				// implicitly invokes it on every google_project_service
				// reference. Every gcp-*.yaml scenario exercises it
				// transitively, so a dedicated scenario would only test
				// the stub in isolation (M70).
				"serviceusage": "meta-API exercised transitively by every GCP scenario via google_project_service",
			},
		},
		{
			name: "mockway",
			mapping: map[string][]string{
				"block":    {"block-paris.yaml"},
				"domain":   {"domain-paris.yaml"},
				"iam":      {"iam-policies-paris.yaml", "public-registry-iam-paris.yaml"},
				"instance": {"compute-lb-multi-paris.yaml", "web-app-paris.yaml", "full-stack-paris.yaml"},
				"k8s":      {"k8s-cluster-paris.yaml", "k8s-medium-override-paris.yaml"},
				"lb":       {"lb-paris.yaml", "compute-lb-multi-paris.yaml", "private-lb-db-paris.yaml"},
				"rdb":      {"mysql-ha-paris.yaml", "private-lb-db-paris.yaml"},
				"redis":    {"redis-paris.yaml", "redis-xlarge-session-paris.yaml"},
				"registry": {"registry-paris.yaml", "public-registry-iam-paris.yaml"},
				"vpc":      {"compute-lb-multi-paris.yaml", "private-lb-db-paris.yaml", "full-stack-paris.yaml"},
			},
			exempt: map[string]string{
				// ipam is mockway's IP-address-management surface. The
				// Scaleway provider's compute/lb/vpc resources call IPAM
				// transitively when allocating addresses, so every
				// scenario that touches instances or load balancers
				// exercises it. No dedicated scenario is needed.
				"ipam": "exercised transitively by instance/lb/vpc scenarios — has no standalone Scaleway resource type",
				// marketplace is the catalog API (instance images,
				// marketplace listings). The Scaleway provider reads it
				// during instance creation to resolve image IDs; every
				// scenario that creates a server hits it. There's no
				// scaleway_marketplace_* resource type to model in
				// isolation.
				"marketplace": "read-only catalog API exercised by every instance/k8s scenario — no standalone resource type",
			},
		},
	}
}

// landedServicesRE matches the slice literal in regression_manifest.go.
// The manifest format is stable across all three fakes (mirrored from
// fakeaws's S52-T1 pattern) so a single regex extracts all of them.
var landedServicesRE = regexp.MustCompile(`(?s)var\s+LandedServices\s*=\s*\[\]string\{([^}]+)\}`)
var landedServiceTokenRE = regexp.MustCompile(`"([a-z][a-z0-9]*)"`)

func parseLandedServices(payload []byte) ([]string, error) {
	match := landedServicesRE.FindSubmatch(payload)
	if len(match) < 2 {
		return nil, nil
	}
	tokens := landedServiceTokenRE.FindAllSubmatch(match[1], -1)
	out := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		out = append(out, string(tok[1]))
	}
	return out, nil
}
