package scenario

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestParseAcceptanceCriteria(t *testing.T) {
	t.Parallel()

	port5432 := 5432
	port80 := 80

	criteria := []AcceptanceCriterion{
		{
			Type:   "connectivity",
			From:   "compute",
			To:     "database",
			Port:   &port5432,
			Expect: "success",
		},
		{
			Type:   "http_probe",
			Target: "load_balancer",
			Port:   &port80,
			Expect: "reachable",
		},
		{
			Type:   "policy",
			Check:  "encryption_at_rest",
			Target: "database",
			Expect: "pass",
		},
		{
			Type:   "dns_resolution",
			Domain: "{{scenario_name}}.example.com",
			Expect: "resolves",
		},
		{
			Type:   "destruction",
			Expect: "no_orphans",
		},
	}

	specs, err := ParseAcceptanceCriteria(criteria)
	if err != nil {
		t.Fatalf("parse acceptance criteria: %v", err)
	}
	if len(specs) != 5 {
		t.Fatalf("expected 5 specs, got %d", len(specs))
	}

	if specs[0].Connectivity == nil || specs[0].Connectivity.Port != 5432 {
		t.Fatalf("unexpected connectivity spec: %+v", specs[0])
	}
	if specs[1].HTTPProbe == nil || specs[1].HTTPProbe.Target != "load_balancer" || specs[1].HTTPProbe.Port != 80 {
		t.Fatalf("unexpected http probe spec: %+v", specs[1])
	}
	if specs[2].Policy == nil || specs[2].Policy.Check != "encryption_at_rest" {
		t.Fatalf("unexpected policy spec: %+v", specs[2])
	}
	if specs[3].DNSResolution == nil || specs[3].DNSResolution.Domain != "{{scenario_name}}.example.com" {
		t.Fatalf("unexpected dns_resolution spec: %+v", specs[3])
	}
	if specs[4].Destruction == nil {
		t.Fatalf("unexpected destruction spec: %+v", specs[4])
	}
}

func TestParseAcceptanceCriteriaReturnsTypedErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		criteria      []AcceptanceCriterion
		expectedField string
	}{
		{
			name: "unknown type",
			criteria: []AcceptanceCriterion{
				{Type: "unknown", Expect: "pass"},
			},
			expectedField: "type",
		},
		{
			name: "missing expect",
			criteria: []AcceptanceCriterion{
				{Type: "destruction"},
			},
			expectedField: "expect",
		},
		{
			name: "connectivity missing from",
			criteria: []AcceptanceCriterion{
				{Type: "connectivity", To: "database", Expect: "success"},
			},
			expectedField: "from",
		},
		{
			name: "http probe missing port",
			criteria: []AcceptanceCriterion{
				{Type: "http_probe", Target: "load_balancer", Expect: "reachable"},
			},
			expectedField: "port",
		},
		{
			name: "policy missing check",
			criteria: []AcceptanceCriterion{
				{Type: "policy", Expect: "pass"},
			},
			expectedField: "check",
		},
		{
			name: "dns missing domain",
			criteria: []AcceptanceCriterion{
				{Type: "dns_resolution", Expect: "resolves"},
			},
			expectedField: "domain",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseAcceptanceCriteria(tc.criteria)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, ErrInvalidCriterionSpec) {
				t.Fatalf("expected ErrInvalidCriterionSpec, got: %v", err)
			}

			var specErr *CriterionSpecError
			if !errors.As(err, &specErr) {
				t.Fatalf("expected *CriterionSpecError, got %T (%v)", err, err)
			}
			if specErr.Field != tc.expectedField {
				t.Fatalf("expected field %q, got %q", tc.expectedField, specErr.Field)
			}
		})
	}
}

func TestScenarioExecutableChecksUsesLoadedCriteria(t *testing.T) {
	t.Parallel()

	schemaPath := filepath.Join("..", "..", "scenario.schema.json")
	sc, err := LoadWithSchema(filepath.Join("testdata", "typed-fields.yaml"), schemaPath)
	if err != nil {
		t.Fatalf("load scenario: %v", err)
	}

	specs, err := sc.ExecutableChecks()
	if err != nil {
		t.Fatalf("map executable checks: %v", err)
	}
	if len(specs) != len(sc.AcceptanceCriteria) {
		t.Fatalf("expected %d specs, got %d", len(sc.AcceptanceCriteria), len(specs))
	}
}

