package scenario

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadStorageResourcePopulatesField guards against a regression
// where the schema accepts `resources.storage` but the Go Resources
// struct lacked a Storage field, so the storage block was silently
// dropped during JSON round-trip.
func TestLoadStorageResourcePopulatesField(t *testing.T) {
	t.Parallel()

	schemaPath := filepath.Join("..", "..", "scenario.schema.json")
	dir := t.TempDir()
	scenarioPath := filepath.Join(dir, "scenario.yaml")
	if err := os.WriteFile(scenarioPath, []byte(`scenario: gcp-storage-test
version: "1.0"
cloud: gcp
description: storage resource round-trip test.
resources:
  storage:
    purpose: app-assets
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`), 0o600); err != nil {
		t.Fatal(err)
	}

	sc, err := LoadWithSchema(scenarioPath, schemaPath)
	if err != nil {
		t.Fatalf("expected scenario to validate, got %v", err)
	}
	if sc.Resources.Storage == nil {
		t.Fatal("expected sc.Resources.Storage to be populated")
	}
	if sc.Resources.Storage.Purpose != "app-assets" || sc.Resources.Storage.Size != "small" {
		t.Fatalf("expected storage.purpose=app-assets size=small, got %+v", sc.Resources.Storage)
	}
}

// TestLoadGCPTrainingScenarios verifies all checked-in GCP training
// fixtures (S36-T10) validate against the live scenario schema.
func TestLoadGCPTrainingScenarios(t *testing.T) {
	t.Parallel()

	schemaPath := filepath.Join("..", "..", "scenario.schema.json")
	scenariosDir := filepath.Join("..", "..", "scenarios", "training")

	for _, name := range []string{
		"gcp-vm-network.yaml",
		"gcp-gke-cluster.yaml",
		"gcp-cloud-sql.yaml",
		"gcp-full-stack.yaml",
	} {
		path := filepath.Join(scenariosDir, name)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sc, err := LoadWithSchema(path, schemaPath)
			if err != nil {
				t.Fatalf("expected %s to validate, got %v", path, err)
			}
			if sc.Cloud != "gcp" {
				t.Fatalf("expected cloud=gcp, got %q", sc.Cloud)
			}
		})
	}
}

// TestLoadWithSchemaCloudGCP exercises the schema's cloud enum widening to
// accept "gcp" alongside "scaleway", and verifies that GCP-flavored resource
// shapes (including the new storage resource) validate against the schema
// (S36-T2).
func TestLoadWithSchemaCloudGCP(t *testing.T) {
	t.Parallel()

	schemaPath := filepath.Join("..", "..", "scenario.schema.json")

	cases := []struct {
		name        string
		yaml        string
		expectedErr error
	}{
		{
			name: "valid gcp scenario validates",
			yaml: `scenario: gcp-full-stack-test
version: "1.0"
cloud: gcp
description: GCP scenario covering all resource shapes for schema validation.
resources:
  compute:
    purpose: web-server
    size: small
    count: 2
  networking:
    vpc: true
    private_network: true
    load_balancer:
      exposure: public
      backends:
        - port: 80
          protocol: http
  database:
    engine: postgresql
    size: small
    high_availability: false
  kubernetes:
    size: small
  redis:
    purpose: cache
    size: small
  storage:
    purpose: backups
    size: small
  iam:
    purpose: ci-cd
    application: true
    api_key: true
    policy: true
constraints:
  region: europe-west1
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`,
			expectedErr: nil,
		},
		{
			name: "cloud aws rejected",
			yaml: `scenario: aws-not-supported
version: "1.0"
cloud: aws
description: AWS is not in the cloud enum.
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`,
			expectedErr: ErrInvalidScenario,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			scenarioPath := filepath.Join(dir, "scenario.yaml")
			if err := os.WriteFile(scenarioPath, []byte(tc.yaml), 0o600); err != nil {
				t.Fatalf("write temp scenario: %v", err)
			}

			sc, err := LoadWithSchema(scenarioPath, schemaPath)
			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("expected error %v, got %v", tc.expectedErr, err)
			}

			if tc.expectedErr == nil {
				if sc.Cloud != "gcp" {
					t.Fatalf("expected cloud=gcp, got %q", sc.Cloud)
				}
			}
		})
	}
}
