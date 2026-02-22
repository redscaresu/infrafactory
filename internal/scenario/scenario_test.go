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
	}{
		{
			name:             "valid scenario",
			scenarioPath:     filepath.Join("testdata", "valid.yaml"),
			expectedScenario: "web-app-paris",
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
