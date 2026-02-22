package feedback

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/redscaresu/infrafactory/internal/runstore"
)

func TestRunLoopConvergesBeforeMaxIterations(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	runner := &fakeIterationRunner{
		results: []IterationResult{
			{Success: false, Failures: []Failure{{Check: "policy", Detail: "failed"}}},
			{Success: true},
		},
	}

	out, err := RunLoop(context.Background(), LoopConfig{
		MaxIterations: 5,
		Runner:        runner,
		Store:         store,
		Scenario:      "web-app-paris",
		RunID:         "run-001",
	})
	if err != nil {
		t.Fatalf("run loop: %v", err)
	}
	if !out.Converged || out.CompletedIterations != 2 {
		t.Fatalf("unexpected loop result: %+v", out)
	}

	for _, iteration := range []string{"1", "2"} {
		path := filepath.Join(store.Root, "web-app-paris", "run-001", "iterations", iteration, "iteration.json")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected persisted iteration artifact for iteration %s: %v", iteration, err)
		}
	}
}

func TestRunLoopStopsAtMaxIterations(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	runner := &fakeIterationRunner{
		results: []IterationResult{
			{Success: false, Failures: []Failure{{Check: "policy", Detail: "failed-1"}}},
			{Success: false, Failures: []Failure{{Check: "policy", Detail: "failed-2"}}},
			{Success: false, Failures: []Failure{{Check: "policy", Detail: "failed-3"}}},
		},
	}

	out, err := RunLoop(context.Background(), LoopConfig{
		MaxIterations: 3,
		Runner:        runner,
		Store:         store,
		Scenario:      "web-app-paris",
		RunID:         "run-002",
	})
	if err != nil {
		t.Fatalf("run loop: %v", err)
	}
	if out.Converged {
		t.Fatalf("expected non-converged result, got %+v", out)
	}
	if out.CompletedIterations != 3 {
		t.Fatalf("expected 3 completed iterations, got %+v", out)
	}
}

type fakeIterationRunner struct {
	results []IterationResult
}

func (f *fakeIterationRunner) RunIteration(_ context.Context, input IterationInput) (IterationResult, error) {
	idx := input.Iteration - 1
	if idx < 0 || idx >= len(f.results) {
		return IterationResult{Success: false}, nil
	}
	return f.results[idx], nil
}
