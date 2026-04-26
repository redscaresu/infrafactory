package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
)

func TestPitfallsHandlerReturnsEmptyWhenDirectoryUnset(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Paths.Pitfalls = ""

	req := httptest.NewRequest(http.MethodGet, "/api/pitfalls", nil)
	rec := httptest.NewRecorder()
	pitfallsHandler(&serverState{cfg: cfg}).ServeHTTP(rec, req)

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
	pitfallsHandler(&serverState{cfg: cfg}).ServeHTTP(rec, req)

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
	pitfallsHandler(&serverState{cfg: cfg}).ServeHTTP(rec, req)

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
		pitfallsHandler(&serverState{cfg: cfg}).ServeHTTP(rec, req)
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
	pitfallsHandler(&serverState{cfg: cfg}).ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for malformed yaml, got %d", rec.Code)
	}
}

func TestPitfallsEditWritesProviderFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.Default()
	cfg.Paths.Pitfalls = dir

	body := strings.NewReader(`{
  "pitfalls": [
    {"resource": "google_compute_instance", "rule": "Use subnetwork.", "source": "static"},
    {"resource": "google_storage_bucket", "rule": "Set uniform_bucket_level_access.", "source": "static"}
  ]
}`)
	req := httptest.NewRequest(http.MethodPut, "/api/pitfalls/gcp", body)
	rec := httptest.NewRecorder()
	pitfallsHandler(&serverState{cfg: cfg}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	written, err := os.ReadFile(filepath.Join(dir, "gcp.yaml"))
	if err != nil {
		t.Fatalf("expected pitfalls file written: %v", err)
	}
	for _, want := range []string{
		"provider: gcp",
		"resource: google_compute_instance",
		"rule: Use subnetwork.",
		"source: static",
		"resource: google_storage_bucket",
	} {
		if !strings.Contains(string(written), want) {
			t.Fatalf("expected file to contain %q, got:\n%s", want, string(written))
		}
	}
}

func TestPitfallsEditValidatesEntries(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Paths.Pitfalls = t.TempDir()

	cases := []struct {
		name string
		body string
		want int
	}{
		{name: "missing resource", body: `{"pitfalls":[{"resource":"","rule":"x"}]}`, want: http.StatusUnprocessableEntity},
		{name: "missing rule", body: `{"pitfalls":[{"resource":"x","rule":""}]}`, want: http.StatusUnprocessableEntity},
		{name: "unknown field", body: `{"pitfalls":[{"resource":"x","rule":"y","extra":1}]}`, want: http.StatusBadRequest},
		{name: "malformed json", body: `not json`, want: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/pitfalls/gcp", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			pitfallsHandler(&serverState{cfg: cfg}).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("body %q: expected %d, got %d (%s)", tc.body, tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestPitfallsEditRejectsTraversalProvider(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Paths.Pitfalls = t.TempDir()

	for _, name := range []string{"../etc", "a/b", "..\\evil"} {
		req := httptest.NewRequest(http.MethodPut, "/api/pitfalls/"+name, strings.NewReader(`{"pitfalls":[]}`))
		rec := httptest.NewRecorder()
		pitfallsHandler(&serverState{cfg: cfg}).ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for provider %q, got %d", name, rec.Code)
		}
	}
}

func TestPitfallsEditRejectsNonPut(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Paths.Pitfalls = t.TempDir()

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/pitfalls/gcp", strings.NewReader(`{"pitfalls":[]}`))
		rec := httptest.NewRecorder()
		pitfallsHandler(&serverState{cfg: cfg}).ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 for %s, got %d", method, rec.Code)
		}
	}
}
