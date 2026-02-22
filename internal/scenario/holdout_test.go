package scenario

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverCriteriaOnlyHoldouts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	trainingPath := "scenarios/training/web-app-paris.yaml"

	writeFile(t, filepath.Join(dir, "criteria-only.yaml"), `scenario: holdout-criteria
type: holdout
references: scenarios/training/web-app-paris.yaml
acceptance_criteria:
  - type: policy
    check: no_public_database
    expect: pass
`)

	writeFile(t, filepath.Join(dir, "full-holdout.yaml"), `scenario: holdout-full
type: holdout
references: scenarios/training/web-app-paris.yaml
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
references: scenarios/training/other.yaml
acceptance_criteria:
  - type: policy
    check: no_public_database
    expect: pass
`)

	holdouts, err := DiscoverCriteriaOnlyHoldouts(dir, trainingPath)
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
