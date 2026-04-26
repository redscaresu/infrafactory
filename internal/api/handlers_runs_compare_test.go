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

// TestRunCompareHandlerRejectsLeadingDotRunID guards against the
// pre-pass-2 traversal hole where `run1=.` resolved to the scenario
// root because the validator only blocked `..`/`/`/`\`.
func TestRunCompareHandlerRejectsLeadingDotRunID(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	// All four start with non-alphanumeric characters and are rejected
	// at the regex gate before any filesystem call.
	for _, id := range []string{".", ".git", "-leading-dash", "_leading-underscore"} {
		resp, err := http.Get(ts.URL + "/api/runs/web-app-paris/compare?run1=" + id + "&run2=run-1")
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("run id %q: expected 400, got %d", id, resp.StatusCode)
		}
	}
}

// TestRunCompareHandlerReturns404WhenRunMetadataMissing guards the
// pass-2 metadata pre-check. The previous incarnation of this test
// relied on `ListGeneratedFiles` returning os.ErrNotExist for run-2's
// missing `generated/` directory — but that path was already covered by
// the earlier "WhenRunMissing" test. To uniquely exercise the metadata
// gate, write run-1 and run-2 generated/ snapshots but NO metadata for
// run-2. Without the pre-check, the handler would silently diff the
// two file sets; with it, run-2's missing run.json triggers 404.
func TestRunCompareHandlerReturns404WhenRunMetadataMissing(t *testing.T) {
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
	// Both runs have generated files, but only run-1 has run.json.
	for _, runID := range []string{"run-1", "run-2"} {
		if err := store.WriteGeneratedFiles("web-app-paris", runID, map[string][]byte{
			"main.tf": []byte(runID),
		}); err != nil {
			t.Fatalf("write %s files: %v", runID, err)
		}
	}

	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/runs/web-app-paris/compare?run1=run-1&run2=run-2")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for run with missing metadata, got %d", resp.StatusCode)
	}
	// Body must name the offending run (run-2), so a future regression
	// that silently swapped the loop order would still trip this test.
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "run-2") {
		t.Fatalf("expected body to mention run-2, got %s", string(body))
	}
}

// TestRunCompareHandlerRejectsNonGet pins the method gate so a typoed
// POST/PUT/DELETE doesn't accidentally trigger any side effect.
func TestRunCompareHandlerRejectsNonGet(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req, err := http.NewRequest(method, ts.URL+"/api/runs/web-app-paris/compare?run1=a&run2=b", nil)
		if err != nil {
			t.Fatalf("build %s request: %v", method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("send %s: %v", method, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("expected 405 for %s, got %d", method, resp.StatusCode)
		}
	}
}

// TestRunCompareHandlerRejectsOversizedRunID guards the 64-char cap on
// validRunID — an oversized run-id query parameter should surface as a
// 400 client error instead of a 500 from os.ReadFile ENAMETOOLONG.
func TestRunCompareHandlerRejectsOversizedRunID(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	long := strings.Repeat("a", 256)
	resp, err := http.Get(ts.URL + "/api/runs/web-app-paris/compare?run1=" + long + "&run2=run-1")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized run id, got %d", resp.StatusCode)
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
