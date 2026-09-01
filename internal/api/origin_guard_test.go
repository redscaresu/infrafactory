package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func guarded(t *testing.T) http.Handler {
	t.Helper()
	reached := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"reached":true}`))
	})
	return guardCrossOriginRequests(reached)
}

// The attack, exactly as a page would run it: a cross-origin fetch with
// Content-Type text/plain, which is a CORS SIMPLE request and so never
// preflights. The body turns on real-cloud apply.
func TestGuardRefusesTheCrossOriginSimpleRequestThatWouldSpendMoney(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/runs/block-paris/start",
		strings.NewReader(`{"layer3_enabled":true}`))
	req.Host = "127.0.0.1:4173"
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Content-Type", "text/plain")

	rec := httptest.NewRecorder()
	guarded(t).ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	assert.NotContains(t, rec.Body.String(), "reached", "the handler must never run")
}

// The UI's own requests must keep working -- including the Vite dev
// server on :5173 calling the API on :4173, which is cross-ORIGIN and
// entirely loopback.
func TestGuardAllowsEveryLoopbackOrigin(t *testing.T) {
	for _, origin := range []string{
		"http://127.0.0.1:4173",
		"http://localhost:4173",
		"http://127.0.0.1:5173",
		"http://localhost:5173",
		"http://[::1]:4173",
		"https://127.0.0.1:4173",
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/runs/block-paris/start", strings.NewReader(`{}`))
		req.Host = "127.0.0.1:4173"
		req.Header.Set("Origin", origin)

		rec := httptest.NewRecorder()
		guarded(t).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code, "%s is the operator's own machine", origin)
	}
}

// DNS rebinding is why the rule is "loopback" and not "same origin as
// this request". A rebound domain sends an Origin and a Host that AGREE,
// so an equality check passes it -- and the request lands on the local
// server anyway.
func TestGuardRefusesARebinding_WhereOriginAndHostAgree(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/runs/block-paris/start",
		strings.NewReader(`{"layer3_enabled":true}`))
	req.Host = "evil.example:4173"
	req.Header.Set("Origin", "http://evil.example:4173")

	rec := httptest.NewRecorder()
	guarded(t).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code,
		"the two agreeing is exactly what makes an equality check useless here")
}

// A stance, not an oversight: this server spends real money with no
// credential check, and the safety model already proposes refusing a
// non-loopback bind outright.
func TestGuardRefusesAUIServedToTheLAN(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/runs/block-paris/start", strings.NewReader(`{}`))
	req.Host = "192.168.1.20:4173"
	req.Header.Set("Origin", "http://192.168.1.20:4173")

	rec := httptest.NewRecorder()
	guarded(t).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// Absence is a decision, not a gap: browsers always send Origin on
// cross-origin requests, so no Origin means a script, and refusing those
// would break every caller while proving nothing.
func TestGuardAllowsRequestsWithNoOriginAtAll(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/runs/block-paris/start", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:4173"

	rec := httptest.NewRecorder()
	guarded(t).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "curl and the test suite must keep working")
}

// A sandboxed iframe or a file:// page sends the literal string `null`,
// which names no host and so can never be this one.
func TestGuardRefusesTheNullOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/runs/block-paris/start", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:4173"
	req.Header.Set("Origin", "null")

	rec := httptest.NewRecorder()
	guarded(t).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// A name that merely LOOKS loopback is not: only a literal loopback
// address in the header counts, because resolving a name here would
// reintroduce the rebinding the rule exists to defeat.
func TestGuardRefusesNamesThatOnlyLookLoopback(t *testing.T) {
	for _, origin := range []string{
		"http://localhost.evil.example",
		"http://notlocalhost",
		"http://127.0.0.1.evil.example",
		"http://evil.example",
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/runs/block-paris/start", strings.NewReader(`{}`))
		req.Host = "127.0.0.1:4173"
		req.Header.Set("Origin", origin)

		rec := httptest.NewRecorder()
		guarded(t).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code, "%s is not loopback", origin)
	}
}

// Not only the unsafe methods. Filtering by method would rest the guard
// on "no GET handler ever mutates", which nothing enforces.
func TestGuardAppliesToEveryMethod(t *testing.T) {
	for _, method := range []string{
		http.MethodGet, http.MethodHead, http.MethodPost,
		http.MethodPut, http.MethodPatch, http.MethodDelete,
	} {
		req := httptest.NewRequest(method, "/api/scenarios", nil)
		req.Host = "127.0.0.1:4173"
		req.Header.Set("Origin", "https://evil.example")

		rec := httptest.NewRecorder()
		guarded(t).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code, "%s must be guarded too", method)
	}
}

// The guard sits ABOVE routing, so a handler registered tomorrow is
// covered without anyone remembering to cover it. A path that routes
// nowhere still being refused is what proves the position.
func TestGuardSitsAboveRoutingSoNewHandlersCannotForgetIt(t *testing.T) {
	srv := NewServer(ServerConfig{Addr: "127.0.0.1:4173"})

	for _, path := range []string{
		"/api/runs/block-paris/start",
		"/api/scenarios/validate",
		"/api/pitfalls/scaleway",
		"/api/a-handler-that-does-not-exist-yet",
		"/",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		req.Host = "127.0.0.1:4173"
		req.Header.Set("Origin", "https://evil.example")

		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code,
			"%s is refused by position, not by having been listed", path)
	}
}
