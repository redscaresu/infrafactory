package scenario

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestLoadWithSchemaPaths(t *testing.T) {
	t.Parallel()

	schemaPath := filepath.Join("..", "..", "scenario.schema.json")
	cases := []struct {
		name                string
		scenarioPath        string
		expectedScenario    string
		expectedErr         error
		expectedProblemPath string
		assert              func(*testing.T, Scenario)
	}{
		{
			name:             "valid scenario",
			scenarioPath:     filepath.Join("testdata", "valid.yaml"),
			expectedScenario: "web-app-paris",
			assert: func(t *testing.T, sc Scenario) {
				t.Helper()
				if sc.Type != "" {
					t.Fatalf("expected empty type for training scenario, got %q", sc.Type)
				}
				if sc.References != "" {
					t.Fatalf("expected empty references for training scenario, got %q", sc.References)
				}
				if len(sc.AcceptanceCriteria) != 1 {
					t.Fatalf("expected one acceptance criterion, got %d", len(sc.AcceptanceCriteria))
				}
				if sc.Resources.Compute == nil {
					t.Fatal("expected compute resource to decode")
				}
			},
		},
		{
			name:             "typed model scenario fields",
			scenarioPath:     filepath.Join("testdata", "typed-fields.yaml"),
			expectedScenario: "holdout-routing",
			assert: func(t *testing.T, sc Scenario) {
				t.Helper()

				if sc.Type != "holdout" {
					t.Fatalf("expected holdout type, got %q", sc.Type)
				}
				if sc.References != "scenarios/training/web-app-paris.yaml" {
					t.Fatalf("unexpected references: %q", sc.References)
				}
				if len(sc.AcceptanceCriteria) != 5 {
					t.Fatalf("expected 5 acceptance criteria, got %d", len(sc.AcceptanceCriteria))
				}
				// S51: region constraint moved into a region_restriction
				// criterion's params; encryption_at_rest decorative
				// constraint dropped (policy doesn't read it).
				regionCriterion := sc.AcceptanceCriteria[1]
				if regionCriterion.Type != "policy" || regionCriterion.Check != "region_restriction" {
					t.Fatalf("expected criterion[1] to be policy/region_restriction, got %+v", regionCriterion)
				}
				if regionCriterion.Params["region"] != "fr-par" {
					t.Fatalf("expected region_restriction params.region=fr-par, got %#v", regionCriterion.Params["region"])
				}

				connectivity := sc.AcceptanceCriteria[0]
				if connectivity.Type != "connectivity" || connectivity.From != "compute" || connectivity.To != "database" {
					t.Fatalf("unexpected connectivity criterion: %+v", connectivity)
				}
				if connectivity.Port == nil || *connectivity.Port != 5432 {
					t.Fatalf("expected connectivity port 5432, got %+v", connectivity.Port)
				}

				policy := sc.AcceptanceCriteria[2]
				if policy.Type != "policy" || policy.Check != "encryption_at_rest" || policy.Target != "database" {
					t.Fatalf("unexpected policy criterion: %+v", policy)
				}

				dns := sc.AcceptanceCriteria[3]
				if dns.Type != "dns_resolution" || dns.Domain != "{{scenario_name}}.example.com" {
					t.Fatalf("unexpected dns criterion: %+v", dns)
				}

				destruction := sc.AcceptanceCriteria[4]
				if destruction.Type != "destruction" || destruction.Expect != "no_orphans" {
					t.Fatalf("unexpected destruction criterion: %+v", destruction)
				}
			},
		},
		{
			name:             "new resource types decode correctly",
			scenarioPath:     filepath.Join("testdata", "new-resource-types.yaml"),
			expectedScenario: "new-resources",
			assert: func(t *testing.T, sc Scenario) {
				t.Helper()
				if sc.Resources.Kubernetes == nil {
					t.Fatal("expected kubernetes resource")
				}
				if sc.Resources.Kubernetes.Size != "small" {
					t.Fatalf("expected k8s size small, got %q", sc.Resources.Kubernetes.Size)
				}
				if sc.Resources.Kubernetes.Override.NodeType != "DEV1-M" {
					t.Fatalf("expected k8s node_type DEV1-M, got %q", sc.Resources.Kubernetes.Override.NodeType)
				}
				if sc.Resources.Kubernetes.Override.NodeCount != 2 {
					t.Fatalf("expected k8s node_count 2, got %d", sc.Resources.Kubernetes.Override.NodeCount)
				}
				if sc.Resources.IAM == nil {
					t.Fatal("expected IAM resource")
				}
				if sc.Resources.IAM.Purpose != "ci-cd" {
					t.Fatalf("expected IAM purpose ci-cd, got %q", sc.Resources.IAM.Purpose)
				}
				if !sc.Resources.IAM.Application {
					t.Fatal("expected IAM application=true")
				}
				if !sc.Resources.IAM.APIKey {
					t.Fatal("expected IAM api_key=true")
				}
				if sc.Resources.IAM.Policy {
					t.Fatal("expected IAM policy=false when explicitly set")
				}
				if sc.Resources.Registry == nil {
					t.Fatal("expected registry resource")
				}
				if sc.Resources.Registry.Purpose != "container-images" {
					t.Fatalf("expected registry purpose container-images, got %q", sc.Resources.Registry.Purpose)
				}
				if !sc.Resources.Registry.IsPublic {
					t.Fatal("expected registry is_public=true")
				}
				if sc.Resources.Redis == nil {
					t.Fatal("expected redis resource")
				}
				if sc.Resources.Redis.Purpose != "cache" {
					t.Fatalf("expected redis purpose cache, got %q", sc.Resources.Redis.Purpose)
				}
				if sc.Resources.Redis.Override.NodeType != "RED1-S" {
					t.Fatalf("expected redis override node_type RED1-S, got %q", sc.Resources.Redis.Override.NodeType)
				}
			},
		},
		{
			name:             "IAM defaults applied when omitted",
			scenarioPath:     filepath.Join("testdata", "iam-defaults.yaml"),
			expectedScenario: "iam-defaults",
			assert: func(t *testing.T, sc Scenario) {
				t.Helper()
				if sc.Resources.IAM == nil {
					t.Fatal("expected IAM resource")
				}
				if !sc.Resources.IAM.Application {
					t.Fatal("expected IAM application to default to true")
				}
				if !sc.Resources.IAM.APIKey {
					t.Fatal("expected IAM api_key to default to true")
				}
				if !sc.Resources.IAM.Policy {
					t.Fatal("expected IAM policy to default to true")
				}
			},
		},
		{
			name:             "IAM explicit false preserved",
			scenarioPath:     filepath.Join("testdata", "iam-explicit-false.yaml"),
			expectedScenario: "iam-explicit-false",
			assert: func(t *testing.T, sc Scenario) {
				t.Helper()
				if sc.Resources.IAM == nil {
					t.Fatal("expected IAM resource")
				}
				if sc.Resources.IAM.Application {
					t.Fatal("expected IAM application=false when explicitly set")
				}
				if sc.Resources.IAM.APIKey {
					t.Fatal("expected IAM api_key=false when explicitly set")
				}
				if !sc.Resources.IAM.Policy {
					t.Fatal("expected IAM policy=true when explicitly set")
				}
			},
		},
		{
			name:                "invalid scenario schema",
			scenarioPath:        filepath.Join("testdata", "invalid-schema.yaml"),
			expectedErr:         ErrInvalidScenario,
			expectedProblemPath: "/acceptance_criteria/0/expect",
		},
		{
			name:         "malformed yaml",
			scenarioPath: filepath.Join("testdata", "malformed.yaml"),
			expectedErr:  ErrMalformedScenario,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sc, err := LoadWithSchema(tc.scenarioPath, schemaPath)
			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("expected error %v, got %v", tc.expectedErr, err)
			}

			if tc.expectedErr == nil {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				if sc.Name != tc.expectedScenario {
					t.Fatalf("expected scenario name %q, got %q", tc.expectedScenario, sc.Name)
				}
				if tc.assert != nil {
					tc.assert(t, sc)
				}
				return
			}

			if tc.expectedProblemPath != "" {
				var validationErr *ValidationError
				if !errors.As(err, &validationErr) {
					t.Fatalf("expected *ValidationError, got %T (%v)", err, err)
				}

				found := false
				for _, violation := range validationErr.Violations {
					if violation.Path == tc.expectedProblemPath {
						found = true
					}
				}
				if !found {
					t.Fatalf("expected violation path %q, got %+v", tc.expectedProblemPath, validationErr.Violations)
				}
			}
		})
	}
}
