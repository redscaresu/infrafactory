package runstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	if list[0].RunID != "run-002" || list[1].RunID != "run-001" {
		t.Fatalf("expected deterministic sort by run id, got %+v", list)
	}
	if list[0].Schema != RunMetadataSchemaVersion || list[1].Schema != RunMetadataSchemaVersion {
		t.Fatalf("expected versioned run metadata entries, got %+v", list)
	}
}

func TestFilesystemStoreListRunsSkipsIncompleteDirectories(t *testing.T) {
	t.Parallel()

	store := NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	if err := store.WriteRunMetadata(RunMetadata{
		Scenario:  "web-app-paris",
		RunID:     "run-001",
		Status:    "success",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write run metadata: %v", err)
	}

	incompleteDir := filepath.Join(store.Root, "web-app-paris", "run-002")
	if err := os.MkdirAll(incompleteDir, 0o755); err != nil {
		t.Fatalf("mkdir incomplete dir: %v", err)
	}

	list, err := store.ListRuns("web-app-paris")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 complete run, got %d", len(list))
	}
	if list[0].RunID != "run-001" {
		t.Fatalf("unexpected run list: %+v", list)
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

func TestFilesystemStoreListScenarios(t *testing.T) {
	t.Parallel()

	store := NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	if err := store.WriteRunMetadata(RunMetadata{
		Scenario:  "a-scenario",
		RunID:     "run-1",
		Status:    "success",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write metadata 1: %v", err)
	}
	if err := store.WriteRunMetadata(RunMetadata{
		Scenario:  "b-scenario",
		RunID:     "run-2",
		Status:    "failed",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write metadata 2: %v", err)
	}

	scenarios, err := store.ListScenarios()
	if err != nil {
		t.Fatalf("list scenarios: %v", err)
	}
	if len(scenarios) != 2 {
		t.Fatalf("expected 2 scenarios, got %d", len(scenarios))
	}
	if scenarios[0] != "a-scenario" || scenarios[1] != "b-scenario" {
		t.Fatalf("unexpected scenarios ordering: %+v", scenarios)
	}
}

func TestFilesystemStoreReadIterationArtifact(t *testing.T) {
	t.Parallel()

	store := NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	payload := []byte(`{"iteration":1}`)
	if err := store.WriteIterationArtifact("web-app-paris", "run-001", 1, "iteration.json", payload); err != nil {
		t.Fatalf("write iteration artifact: %v", err)
	}

	read, err := store.ReadIterationArtifact("web-app-paris", "run-001", 1)
	if err != nil {
		t.Fatalf("read iteration artifact: %v", err)
	}
	if string(read) != string(payload) {
		t.Fatalf("unexpected iteration artifact payload: %s", string(read))
	}
}

func TestFilesystemStoreReadRunLog(t *testing.T) {
	t.Parallel()

	store := NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	runDir := filepath.Join(store.Root, "web-app-paris", "run-001")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	payload := []byte("{\"event\":\"run_start\"}\n")
	if err := os.WriteFile(filepath.Join(runDir, "app.log"), payload, 0o644); err != nil {
		t.Fatalf("write app log: %v", err)
	}

	read, err := store.ReadRunLog("web-app-paris", "run-001")
	if err != nil {
		t.Fatalf("read run log: %v", err)
	}
	if string(read) != string(payload) {
		t.Fatalf("unexpected run log payload: %s", string(read))
	}
}

func TestFilesystemStoreWriteListReadGeneratedFiles(t *testing.T) {
	t.Parallel()

	store := NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	if err := store.WriteGeneratedFiles("web-app-paris", "run-001", map[string][]byte{
		"main.tf":                []byte("terraform {}\n"),
		"modules/network/vpc.tf": []byte("resource \"x\" \"y\" {}\n"),
	}); err != nil {
		t.Fatalf("write generated files: %v", err)
	}

	files, err := store.ListGeneratedFiles("web-app-paris", "run-001")
	if err != nil {
		t.Fatalf("list generated files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0] != "main.tf" || files[1] != "modules/network/vpc.tf" {
		t.Fatalf("unexpected file list: %+v", files)
	}

	payload, err := store.ReadGeneratedFile("web-app-paris", "run-001", "modules/network/vpc.tf")
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	if !strings.Contains(string(payload), `resource "x" "y"`) {
		t.Fatalf("unexpected generated file contents: %s", string(payload))
	}
}

func TestFilesystemStoreGeneratedFilesRejectTraversal(t *testing.T) {
	t.Parallel()

	store := NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	err := store.WriteGeneratedFiles("web-app-paris", "run-001", map[string][]byte{
		"../../etc/passwd": []byte("bad"),
	})
	if err == nil {
		t.Fatal("expected traversal error")
	}

	if _, err := store.ReadGeneratedFile("web-app-paris", "run-001", "../../etc/passwd"); err == nil {
		t.Fatal("expected traversal read error")
	}
}

func TestFilesystemStoreGeneratedFilesAreIsolatedPerRun(t *testing.T) {
	t.Parallel()

	store := NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	if err := store.WriteGeneratedFiles("web-app-paris", "run-001", map[string][]byte{
		"main.tf": []byte("resource \"scaleway_vpc\" \"old\" {}\n"),
	}); err != nil {
		t.Fatalf("write first run files: %v", err)
	}
	if err := store.WriteGeneratedFiles("web-app-paris", "run-002", map[string][]byte{
		"main.tf": []byte("resource \"scaleway_vpc\" \"new\" {}\n"),
	}); err != nil {
		t.Fatalf("write second run files: %v", err)
	}

	first, err := store.ReadGeneratedFile("web-app-paris", "run-001", "main.tf")
	if err != nil {
		t.Fatalf("read first run file: %v", err)
	}
	second, err := store.ReadGeneratedFile("web-app-paris", "run-002", "main.tf")
	if err != nil {
		t.Fatalf("read second run file: %v", err)
	}

	if !strings.Contains(string(first), `"old"`) {
		t.Fatalf("expected first run content to be preserved, got: %s", string(first))
	}
	if !strings.Contains(string(second), `"new"`) {
		t.Fatalf("expected second run content to be preserved, got: %s", string(second))
	}
}

func TestFilesystemStoreIterationGeneratedFilesAndIterations(t *testing.T) {
	t.Parallel()

	store := NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	if err := store.WriteIterationGeneratedFiles("web-app-paris", "run-001", 2, map[string][]byte{
		"main.tf": []byte("resource \"scaleway_vpc\" \"iter2\" {}\n"),
		"vars.tf": []byte("variable \"region\" {}\n"),
	}); err != nil {
		t.Fatalf("write iteration 2 generated files: %v", err)
	}
	if err := store.WriteIterationGeneratedFiles("web-app-paris", "run-001", 1, map[string][]byte{
		"main.tf": []byte("resource \"scaleway_vpc\" \"iter1\" {}\n"),
	}); err != nil {
		t.Fatalf("write iteration 1 generated files: %v", err)
	}

	iterations, err := store.ListIterations("web-app-paris", "run-001")
	if err != nil {
		t.Fatalf("list iterations: %v", err)
	}
	if len(iterations) != 2 || iterations[0] != 1 || iterations[1] != 2 {
		t.Fatalf("unexpected iterations: %+v", iterations)
	}

	files, err := store.ListIterationGeneratedFiles("web-app-paris", "run-001", 2)
	if err != nil {
		t.Fatalf("list iteration files: %v", err)
	}
	if len(files) != 2 || files[0] != "main.tf" || files[1] != "vars.tf" {
		t.Fatalf("unexpected iteration files: %+v", files)
	}

	payload, err := store.ReadIterationGeneratedFile("web-app-paris", "run-001", 1, "main.tf")
	if err != nil {
		t.Fatalf("read iteration file: %v", err)
	}
	if !strings.Contains(string(payload), `"iter1"`) {
		t.Fatalf("unexpected iteration payload: %s", string(payload))
	}
}
