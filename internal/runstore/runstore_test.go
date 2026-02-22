package runstore

import (
	"encoding/json"
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
	if read.Schema != RunMetadataSchemaVersion {
		t.Fatalf("expected run metadata schema %q, got %q", RunMetadataSchemaVersion, read.Schema)
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
	if list[0].Schema != RunMetadataSchemaVersion || list[1].Schema != RunMetadataSchemaVersion {
		t.Fatalf("expected versioned run metadata entries, got %+v", list)
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

func TestFilesystemStoreReadRunMetadataLegacyWithoutSchema(t *testing.T) {
	t.Parallel()

	store := NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	runDir := filepath.Join(store.Root, "web-app-paris", "run-legacy")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}

	legacy := map[string]any{
		"scenario":   "web-app-paris",
		"run_id":     "run-legacy",
		"status":     "failed",
		"started_at": "2026-02-22T12:00:00Z",
	}
	payload, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("marshal legacy payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "run.json"), payload, 0o644); err != nil {
		t.Fatalf("write legacy run metadata: %v", err)
	}

	read, err := store.ReadRunMetadata("web-app-paris", "run-legacy")
	if err != nil {
		t.Fatalf("read run metadata: %v", err)
	}
	if read.Schema != RunMetadataSchemaLegacy {
		t.Fatalf("expected legacy schema %q, got %q", RunMetadataSchemaLegacy, read.Schema)
	}
	if read.RunID != "run-legacy" || read.Scenario != "web-app-paris" {
		t.Fatalf("unexpected legacy metadata decode: %+v", read)
	}
}
