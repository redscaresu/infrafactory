package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/runstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The static audit says no CALL SITE writes an error. This says the
// bodies are actually clean when the filesystem fails, which is the
// thing anyone cares about -- and it is the probe that found the class.
//
// An unreadable run store is the realistic trigger. The not-found paths
// all have explicit stable branches and always did; it was the fallback
// for unexpected errors that leaked, so the store has to EXIST and be
// unreadable rather than be missing.
func TestNoResponseBodyNamesAServerPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "secret-dir-name", "runs")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "web-app-paris", "run-1"), 0o755))
	require.NoError(t, os.Chmod(filepath.Join(root, "web-app-paris"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "web-app-paris"), 0o755) })

	// SKIP where the chmod does not actually deny, rather than fail.
	//
	// Root has DAC_OVERRIDE and Windows ignores the mode bits, so in a
	// container, a devcontainer or under `sudo make test` every endpoint
	// succeeds, nothing calls logf, and the final assertion fails
	// pointing at a logging bug that does not exist. The probe asks the
	// filesystem instead of trusting the chmod.
	if _, probeErr := os.ReadDir(filepath.Join(root, "web-app-paris")); probeErr == nil {
		t.Skip("this filesystem does not enforce the mode bits, so nothing here can fail")
	}

	var logged int
	srv := NewServer(ServerConfig{
		Config: config.Default(),
		Store:  runstore.NewFilesystemStore(root),
		Logf:   func(string, ...any) { logged++ },
	})

	for _, path := range []string{
		"/api/runs",
		"/api/runs/web-app-paris",
		"/api/runs/web-app-paris/run-1",
		"/api/runs/web-app-paris/run-1/log",
		"/api/runs/web-app-paris/run-1/files",
	} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			body := rec.Body.String()

			assert.NotContains(t, body, root, "the store's path must not reach a client")
			assert.NotContains(t, body, "secret-dir-name",
				"nor any component of it")
			assert.NotContains(t, body, "permission denied",
				"nor the operating system's own words about our filesystem")
		})
	}

	// And the cause was not merely swallowed. A response that hides the
	// detail while nothing records it leaves an operator with a stable
	// sentence and no way to act on it -- which is the failure this
	// package spent a review round removing from the deploy handlers.
	assert.Positive(t, logged, "the withheld causes must reach the log")
}
