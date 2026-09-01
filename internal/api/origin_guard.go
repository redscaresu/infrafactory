package api

import (
	"net"
	"net/http"
	"net/url"
)

// guardCrossOriginRequests refuses any request carrying an `Origin` that
// is not this server's own.
//
// # Why this exists
//
// The UI is an unauthenticated HTTP server on loopback, and loopback is
// not a boundary a browser respects. Any page the operator has open can
// issue a request to `127.0.0.1:4173`, and until this guard existed
// `POST /api/runs/<scenario>/start` with `{"layer3_enabled": true}` would
// start a REAL Scaleway apply: `ui_command.go` uses that field to set
// `SandboxDeploy.Enabled`, and credentials come from the process
// environment, which holds live keys whenever the server was started from
// a shell that sourced `layer3.env`.
//
// Nothing about that attack needs to read the response — CORS withholding
// the body is irrelevant when the side effect IS the goal, and the side
// effect is real infrastructure and real money. Nor does it need a
// preflight: a `fetch` with `Content-Type: text/plain` is a CORS *simple*
// request, and the handlers decode the body without inspecting the
// content type.
//
// # Why it wraps the mux instead of guarding the endpoints
//
// The alternative is to list today's state-changing endpoints and guard
// each. That reads as coverage and is a snapshot: the next handler is
// written by someone who never saw the list, and it is unguarded from the
// moment it is registered. Above the routing, a handler cannot opt out by
// being forgotten.
//
// # Why every method, not only the unsafe ones
//
// Filtering to POST/PUT/DELETE would leave the guard resting on "no GET
// handler ever mutates anything", which is an invariant nothing enforces
// and which a future handler can quietly break. Refusing a mismatched
// origin outright removes the question. It costs nothing: a cross-origin
// GET could not read the response anyway.
func guardCrossOriginRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// An ABSENT Origin is allowed, and that is a decision rather
		// than a hole. Browsers always send it on cross-origin
		// requests, so absence means a non-browser client -- curl, a
		// test, a script -- and a non-browser attacker is not the
		// threat this guards against. Refusing absent origins would
		// break every script that drives this API and would prove
		// nothing: an attacker who can already run a process on the
		// machine can simply send whatever origin it likes.
		origin := r.Header.Get("Origin")
		if origin == "" || isLoopbackOrigin(origin) {
			next.ServeHTTP(w, r)
			return
		}

		// Deliberately says nothing about what the endpoint would have
		// done, or whether it exists. The refusal is the whole message.
		writeJSONError(w, http.StatusForbidden,
			"cross-origin request refused: this server is reachable only from a page served on loopback")
	})
}

// isLoopbackOrigin reports whether an Origin header names a loopback
// address.
//
// # Why loopback rather than "the same origin as this request"
//
// Comparing the Origin against the request's own Host looks like the
// stricter rule and is the weaker one, because it is defeated by DNS
// REBINDING. An attacker domain whose record flips to 127.0.0.1 makes the
// browser send `Origin: http://evil.example` AND `Host: evil.example:4173`
// -- they agree, so an equality check passes them, and the request lands
// on the local server anyway.
//
// A rebound request cannot forge a LOOPBACK origin: the page was served
// from the attacker's name, so that name is what the browser reports.
// Requiring loopback therefore defeats rebinding and the ordinary
// drive-by with one rule.
//
// It also keeps development working, which an equality check does not:
// the Vite dev server serves the UI on :5173 and calls this API on :4173,
// a legitimately cross-ORIGIN pair that is entirely loopback.
//
// # What this deliberately refuses
//
// A UI served to a LAN or tailnet address -- `--addr 0.0.0.0:4173`
// browsed from another machine -- is refused, because that origin is not
// loopback. That is a stance rather than an oversight: this server spends
// real money with no credential check, and the UI arc's safety model
// already proposes refusing a non-loopback bind outright. Making the
// guard stretch to cover a shape the next slice intends to forbid would
// be building a hole to spec.
func isLoopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}

	// Covers the literal `null` a sandboxed iframe or a `file://` page
	// sends: it names no host, so it can never be a loopback one.
	host := u.Hostname()
	if host == "" {
		return false
	}

	// `localhost` is not an IP and does not parse as one, but it is the
	// name the dev server and most operators actually use. Everything
	// else must resolve to a loopback literal IN THE HEADER -- resolving
	// a name here would reintroduce exactly the rebinding this rule
	// exists to defeat.
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
