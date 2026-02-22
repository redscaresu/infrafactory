package harness

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/redscaresu/infrafactory/internal/runstore"
)

func TestPersistDestroyRunSuccessAndFailure(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	startedAt := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)

	successResult := &DestroyResult{
		StateSnapshot: []byte(`{"instance":{"servers":[]}}`),
	}
	if err := PersistDestroyRun(store, "web-app-paris", "run-success", startedAt, successResult, nil); err != nil {
		t.Fatalf("persist success run: %v", err)
	}

	successMeta, err := store.ReadRunMetadata("web-app-paris", "run-success")
	if err != nil {
		t.Fatalf("read success run metadata: %v", err)
	}
	if successMeta.Status != "success" {
		t.Fatalf("expected success status, got %q", successMeta.Status)
	}
	successStatePath := filepath.Join(store.Root, "web-app-paris", "run-success", "iterations", "1", "destroy_state.json")
	if _, err := os.Stat(successStatePath); err != nil {
		t.Fatalf("expected destroy state artifact for success run: %v", err)
	}

	failureErr := errors.New("destroy failed")
	if err := PersistDestroyRun(store, "web-app-paris", "run-failed", startedAt, nil, failureErr); err != nil {
		t.Fatalf("persist failed run: %v", err)
	}

	failureMeta, err := store.ReadRunMetadata("web-app-paris", "run-failed")
	if err != nil {
		t.Fatalf("read failed run metadata: %v", err)
	}
	if failureMeta.Status != "failed" {
		t.Fatalf("expected failed status, got %q", failureMeta.Status)
	}

	failureArtifactPath := filepath.Join(store.Root, "web-app-paris", "run-failed", "iterations", "1", "failures.json")
	payload, err := os.ReadFile(failureArtifactPath)
	if err != nil {
		t.Fatalf("read failure artifact: %v", err)
	}
	if string(payload) != `{"error":"destroy failed"}` {
		t.Fatalf("unexpected failure artifact content: %s", string(payload))
	}
}
