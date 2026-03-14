package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/redscaresu/infrafactory/internal/config"
)

func TestServerServesSPAWhenAssetsEmbedded(t *testing.T) {
	t.Parallel()

	assets := fstest.MapFS{
		"ui/build/index.html": &fstest.MapFile{Data: []byte("<html>InfraFactory Dashboard</html>")},
	}
	srv := NewServer(ServerConfig{
		Assets: assets,
		Config: config.Default(),
	})

	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "InfraFactory Dashboard") {
		t.Fatalf("expected placeholder HTML, got %q", string(body))
	}
}

func TestServerReturnsDevModeMessageWhenAssetsMissing(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerConfig{Config: config.Default()})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), uiAssetsMissingMessage) {
		t.Fatalf("expected missing assets message, got %q", string(body))
	}
}

func TestServerRoutesUnimplementedAPITo501(t *testing.T) {
	t.Parallel()

	srv := NewServer(ServerConfig{Config: config.Default()})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/unknown")
	if err != nil {
		t.Fatalf("get /api/unknown: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", resp.StatusCode)
	}
}
