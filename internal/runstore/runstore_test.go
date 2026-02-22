package runstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFilesystemStoreWriteReadList(t *testing.T) {
	t.Parallel()

	store := NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	baseTime := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)

	metas := []RunMetadata{
		{
			Scenario:  "web-app-paris",
			RunID:     "run-001",
			Status:    "failed",
			StartedAt: baseTime,
		},
		{
			Scenario:  "web-app-paris",
			RunID:     "run-002",
			Status:    "success",
			StartedAt: baseTime.Add(time.Minute),
		},
	}

	for _, meta := range metas {
		if err := store.WriteRunMetadata(meta); err != nil {
			t.Fatalf("write run metadata: %v", err)
		}
	}

	read, err := store.ReadRunMetadata("web-app-paris", "run-001")
	if err != nil {
		t.Fatalf("read run metadata: %v", err)
	}
	if read.RunID != "run-001" || read.Status != "failed" {
		t.Fatalf("unexpected read metadata: %+v", read)
	}

	list, err := store.ListRuns("web-app-paris")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(list))
	}
	if list[0].RunID != "run-001" || list[1].RunID != "run-002" {
		t.Fatalf("expected deterministic sort by run id, got %+v", list)
	}
}

func TestFilesystemStoreWriteIterationArtifact(t *testing.T) {
	t.Parallel()

	store := NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))

	if err := store.WriteIterationArtifact("web-app-paris", "run-001", 2, "failures.json", []byte(`{"count":1}`)); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	path := filepath.Join(store.Root, "web-app-paris", "run-001", "iterations", "2", "failures.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if string(content) != `{"count":1}` {
		t.Fatalf("unexpected artifact content: %s", string(content))
	}
}
