package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
)

func TestPitfallsHandlerReturnsEmptyWhenDirectoryUnset(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Paths.Pitfalls = ""

	req := httptest.NewRequest(http.MethodGet, "/api/pitfalls", nil)
	rec := httptest.NewRecorder()
	listPitfallsHandler(&serverState{cfg: cfg}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp pitfallsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Providers) != 0 {
		t.Fatalf("expected empty providers, got %+v", resp.Providers)
	}
}

func TestPitfallsHandlerReturnsEmptyWhenDirectoryMissing(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Paths.Pitfalls = filepath.Join(t.TempDir(), "nonexistent")

	req := httptest.NewRequest(http.MethodGet, "/api/pitfalls", nil)
	rec := httptest.NewRecorder()
	listPitfallsHandler(&serverState{cfg: cfg}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp pitfallsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Providers) != 0 {
		t.Fatalf("expected empty providers when dir missing, got %+v", resp.Providers)
	}
}

func TestPitfallsHandlerGroupsByProviderAlphabetically(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "scaleway.yaml"), []byte(`provider: scaleway
pitfalls:
  - resource: scaleway_redis_cluster
    rule: Password must meet complexity requirements.
    source: seed
  - resource: scaleway_k8s_cluster
    rule: Use full patch version when auto_upgrade is disabled.
    source: learned
    discovered_from: k8s-cluster-paris
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gcp.yaml"), []byte(`provider: gcp
pitfalls:
  - resource: google_container_cluster
    rule: Set initial_node_count or use a separate node pool.
    source: seed
`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-yaml files must be ignored.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# notes"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Paths.Pitfalls = dir

	req := httptest.NewRequest(http.MethodGet, "/api/pitfalls", nil)
	rec := httptest.NewRecorder()
	listPitfallsHandler(&serverState{cfg: cfg}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp pitfallsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(resp.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d (%+v)", len(resp.Providers), resp.Providers)
	}
	if resp.Providers[0].Provider != "gcp" || resp.Providers[1].Provider != "scaleway" {
		t.Fatalf("expected alphabetical order, got %s, %s",
			resp.Providers[0].Provider, resp.Providers[1].Provider)
	}
	if len(resp.Providers[0].Pitfalls) != 1 {
		t.Fatalf("expected 1 gcp pitfall, got %d", len(resp.Providers[0].Pitfalls))
	}
	scw := resp.Providers[1].Pitfalls
	if len(scw) != 2 {
		t.Fatalf("expected 2 scaleway pitfalls, got %d", len(scw))
	}
	if scw[1].Source != "learned" || scw[1].DiscoveredFrom != "k8s-cluster-paris" {
		t.Fatalf("expected source/discovered_from preserved, got %+v", scw[1])
	}
}

func TestPitfallsHandlerRejectsNonGet(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Paths.Pitfalls = t.TempDir()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/pitfalls", nil)
		rec := httptest.NewRecorder()
		listPitfallsHandler(&serverState{cfg: cfg}).ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 for %s, got %d", method, rec.Code)
		}
	}
}

func TestPitfallsHandlerErrorsOnMalformedYaml(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("not: [valid yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Paths.Pitfalls = dir

	req := httptest.NewRequest(http.MethodGet, "/api/pitfalls", nil)
	rec := httptest.NewRecorder()
	listPitfallsHandler(&serverState{cfg: cfg}).ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for malformed yaml, got %d", rec.Code)
	}
}
