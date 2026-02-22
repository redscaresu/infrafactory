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
				if sc.Constraints["region"] != "fr-par" {
					t.Fatalf("expected region constraint fr-par, got %#v", sc.Constraints["region"])
				}
				if sc.Constraints["encryption_at_rest"] != true {
					t.Fatalf("expected encryption_at_rest=true, got %#v", sc.Constraints["encryption_at_rest"])
				}
				if len(sc.AcceptanceCriteria) != 4 {
					t.Fatalf("expected 4 acceptance criteria, got %d", len(sc.AcceptanceCriteria))
				}

				connectivity := sc.AcceptanceCriteria[0]
				if connectivity.Type != "connectivity" || connectivity.From != "compute" || connectivity.To != "database" {
					t.Fatalf("unexpected connectivity criterion: %+v", connectivity)
				}
				if connectivity.Port == nil || *connectivity.Port != 5432 {
					t.Fatalf("expected connectivity port 5432, got %+v", connectivity.Port)
				}

				policy := sc.AcceptanceCriteria[1]
				if policy.Type != "policy" || policy.Check != "encryption_at_rest" || policy.Target != "database" {
					t.Fatalf("unexpected policy criterion: %+v", policy)
				}

				dns := sc.AcceptanceCriteria[2]
				if dns.Type != "dns_resolution" || dns.Domain != "{{scenario_name}}.example.com" {
					t.Fatalf("unexpected dns criterion: %+v", dns)
				}

				destruction := sc.AcceptanceCriteria[3]
				if destruction.Type != "destruction" || destruction.Expect != "no_orphans" {
					t.Fatalf("unexpected destruction criterion: %+v", destruction)
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
