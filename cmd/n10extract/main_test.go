package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAutodiscoverIterPairPicksLastTwoIters pins the run-dir
// shorthand: the highest-numbered iteration becomes passingDir, the
// previous one becomes failedDir, so the diff captures the
// just-before-success transition. Mirrors the convention used by
// internal/cli/run_command.go's iterationGeneratedDir helper.
func TestAutodiscoverIterPairPicksLastTwoIters(t *testing.T) {
	runDir := t.TempDir()
	iters := filepath.Join(runDir, "iterations")
	for _, n := range []string{"1", "2", "3"} {
		gen := filepath.Join(iters, n, "generated")
		if err := os.MkdirAll(gen, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(gen, "main.tf"), []byte("// iter "+n+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	failedDir, passingDir, err := autodiscoverIterPair(runDir)
	if err != nil {
		t.Fatalf("autodiscover: %v", err)
	}
	wantFailed := filepath.Join(iters, "2", "generated")
	wantPassing := filepath.Join(iters, "3", "generated")
	if failedDir != wantFailed {
		t.Errorf("failedDir = %q, want %q", failedDir, wantFailed)
	}
	if passingDir != wantPassing {
		t.Errorf("passingDir = %q, want %q", passingDir, wantPassing)
	}
}

// TestAutodiscoverIterPairTooFewIters guards the single-iteration
// case — there's no "previous" iter to diff against, so the helper
// must return a clear error rather than guessing.
func TestAutodiscoverIterPairTooFewIters(t *testing.T) {
	runDir := t.TempDir()
	gen := filepath.Join(runDir, "iterations", "1", "generated")
	if err := os.MkdirAll(gen, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := autodiscoverIterPair(runDir)
	if err == nil {
		t.Fatal("expected error for single-iter run dir, got nil")
	}
}

// TestAutodiscoverIterPairIgnoresNonNumericDirs ensures stray
// directories (e.g. a future "summary/" or accidental Finder/IDE
// artifacts) don't pollute the numeric sort.
func TestAutodiscoverIterPairIgnoresNonNumericDirs(t *testing.T) {
	runDir := t.TempDir()
	iters := filepath.Join(runDir, "iterations")
	for _, name := range []string{"1", "2", "summary", ".DS_Store"} {
		if err := os.MkdirAll(filepath.Join(iters, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"1", "2"} {
		if err := os.MkdirAll(filepath.Join(iters, n, "generated"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	failedDir, passingDir, err := autodiscoverIterPair(runDir)
	if err != nil {
		t.Fatalf("autodiscover: %v", err)
	}
	if filepath.Base(filepath.Dir(failedDir)) != "1" {
		t.Errorf("failedDir parent = %q, want 1", filepath.Base(filepath.Dir(failedDir)))
	}
	if filepath.Base(filepath.Dir(passingDir)) != "2" {
		t.Errorf("passingDir parent = %q, want 2", filepath.Base(filepath.Dir(passingDir)))
	}
}
