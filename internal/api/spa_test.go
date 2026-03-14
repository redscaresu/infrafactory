package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAHandlerServesKnownFile(t *testing.T) {
	t.Parallel()

	handler := SPAHandler(fstest.MapFS{
		"ui/build/index.html": &fstest.MapFile{Data: []byte("<html>index</html>")},
		"ui/build/app.js":     &fstest.MapFile{Data: []byte("console.log('ok');")},
	})

	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "console.log") {
		t.Fatalf("expected js body, got %q", rec.Body.String())
	}
}

func TestSPAHandlerFallsBackToIndexHTML(t *testing.T) {
	t.Parallel()

	handler := SPAHandler(fstest.MapFS{
		"ui/build/index.html": &fstest.MapFile{Data: []byte("<html>dashboard</html>")},
	})

	req := httptest.NewRequest(http.MethodGet, "/scenarios/training/web-app-paris", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "dashboard") {
		t.Fatalf("expected index fallback, got %q", rec.Body.String())
	}
}

func TestSPAHandlerDoesNotCatchAPIPaths(t *testing.T) {
	t.Parallel()

	handler := SPAHandler(fstest.MapFS{
		"ui/build/index.html": &fstest.MapFile{Data: []byte("<html>dashboard</html>")},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
