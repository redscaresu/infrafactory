package scenario

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverCriteriaOnlyHoldouts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	trainingName := "web-app-paris"

	writeFile(t, filepath.Join(dir, "criteria-only.yaml"), `scenario: holdout-criteria
type: holdout
references: web-app-paris
acceptance_criteria:
  - type: policy
    check: no_public_database
    expect: pass
`)

	writeFile(t, filepath.Join(dir, "full-holdout.yaml"), `scenario: holdout-full
type: holdout
references: web-app-paris
resources:
  compute:
    purpose: web
    size: small
acceptance_criteria:
  - type: policy
    check: no_public_database
    expect: pass
`)

	writeFile(t, filepath.Join(dir, "other-reference.yaml"), `scenario: holdout-other
type: holdout
references: other
acceptance_criteria:
  - type: policy
    check: no_public_database
    expect: pass
`)
	writeFile(t, filepath.Join(dir, "empty-resources.yaml"), `scenario: holdout-empty-resources
type: holdout
references: web-app-paris
resources: {}
acceptance_criteria:
  - type: policy
    check: no_public_database
    expect: pass
`)
	// A PATH where a name belongs. Matching moved to the scenario name
	// because the path form varies by caller -- the CLI passes what the
	// operator typed, the UI server builds one by joining its configured
	// scenarios dir -- and a mismatch is SILENT, reported as "0
	// holdouts" and indistinguishable from having none.
	writeFile(t, filepath.Join(dir, "path-reference.yaml"), `scenario: holdout-path-ref
type: holdout
references: scenarios/training/web-app-paris.yaml
acceptance_criteria:
  - type: policy
    check: no_public_database
    expect: pass
`)

	holdouts, err := DiscoverCriteriaOnlyHoldouts(dir, trainingName)
	if err != nil {
		t.Fatalf("discover holdouts: %v", err)
	}
	if len(holdouts) != 1 {
		t.Fatalf("expected exactly one criteria-only holdout, got %d (%+v)", len(holdouts), holdouts)
	}
	if filepath.Base(holdouts[0].Path) != "criteria-only.yaml" {
		t.Fatalf("unexpected holdout discovered: %+v", holdouts[0])
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
