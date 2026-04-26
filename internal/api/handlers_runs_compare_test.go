package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/runstore"
)

func TestRunCompareHandlerReturnsFileLevelDiff(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	for _, runID := range []string{"run-1", "run-2"} {
		if err := store.WriteRunMetadata(runstore.RunMetadata{
			Scenario:  "web-app-paris",
			RunID:     runID,
			Status:    "success",
			StartedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("write run metadata %s: %v", runID, err)
		}
	}
	if err := store.WriteGeneratedFiles("web-app-paris", "run-1", map[string][]byte{
		"main.tf":      []byte("resource \"a\" \"x\" {}\n"),
		"network.tf":   []byte("resource \"vpc\" \"main\" {}\n"),
		"removed.tf":   []byte("resource \"old\" \"x\" {}\n"),
		"unchanged.tf": []byte("# stable\n"),
	}); err != nil {
		t.Fatalf("write run-1 files: %v", err)
	}
	if err := store.WriteGeneratedFiles("web-app-paris", "run-2", map[string][]byte{
		"main.tf":      []byte("resource \"a\" \"x\" {\n  count = 2\n}\n"),
		"network.tf":   []byte("resource \"vpc\" \"main\" {}\n"),
		"added.tf":     []byte("resource \"new\" \"x\" {}\n"),
		"unchanged.tf": []byte("# stable\n"),
	}); err != nil {
		t.Fatalf("write run-2 files: %v", err)
	}

	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/runs/web-app-paris/compare?run1=run-1&run2=run-2")
	if err != nil {
		t.Fatalf("compare request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(body))
	}
	var got compareResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Run1 != "run-1" || got.Run2 != "run-2" {
		t.Fatalf("expected echoed run ids, got %+v", got)
	}
	statuses := make(map[string]string, len(got.Diffs))
	for _, d := range got.Diffs {
		statuses[d.Filename] = d.Status
	}
	for name, want := range map[string]string{
		"added.tf":     "added",
		"removed.tf":   "removed",
		"main.tf":      "modified",
		"network.tf":   "unchanged",
		"unchanged.tf": "unchanged",
	} {
		if got, ok := statuses[name]; !ok || got != want {
			t.Errorf("expected %s=%s, got %s", name, want, got)
		}
	}

	// Modified entry must include unified diff with the count = 2 line.
	for _, d := range got.Diffs {
		if d.Filename == "main.tf" {
			if d.UnifiedDiff == "" {
				t.Fatalf("expected unified diff for modified file")
			}
			if !strings.Contains(d.UnifiedDiff, "count = 2") {
				t.Fatalf("expected diff to contain new line, got:\n%s", d.UnifiedDiff)
			}
		}
		if d.Status == "unchanged" && d.UnifiedDiff != "" {
			t.Fatalf("expected unchanged file %s to have empty diff", d.Filename)
		}
	}
}

func TestRunCompareHandlerRequiresBothRunIDs(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	for _, q := range []string{"", "?run1=foo", "?run2=bar"} {
		resp, err := http.Get(ts.URL + "/api/runs/web-app-paris/compare" + q)
		if err != nil {
			t.Fatalf("request %s: %v", q, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("query %q expected 400, got %d", q, resp.StatusCode)
		}
	}
}

func TestRunCompareHandlerReturns404WhenRunMissing(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:  "web-app-paris",
		RunID:     "run-1",
		Status:    "success",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	if err := store.WriteGeneratedFiles("web-app-paris", "run-1", map[string][]byte{
		"main.tf": []byte("a"),
	}); err != nil {
		t.Fatalf("write files: %v", err)
	}

	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/runs/web-app-paris/compare?run1=run-1&run2=ghost")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing run, got %d", resp.StatusCode)
	}
}

func TestRunCompareHandlerRejectsPathTraversal(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	for _, run := range []string{"../etc", "a/b", "..\\..\\evil"} {
		resp, err := http.Get(ts.URL + "/api/runs/web-app-paris/compare?run1=" + run + "&run2=run-2")
		if err != nil {
			t.Fatalf("request %s: %v", run, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for run %q, got %d", run, resp.StatusCode)
		}
	}
}
