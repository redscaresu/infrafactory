package scenario

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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
