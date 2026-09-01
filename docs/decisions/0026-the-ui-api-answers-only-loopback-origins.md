# ADR-0026: the UI API answers only loopback origins

Status: accepted (2026-09-01, S160a)

## Context

The UI is an unauthenticated HTTP server bound to `127.0.0.1:4173` by default.
That was defensible while it only read run history and edited YAML. It stopped
being defensible without anyone deciding it should.

`POST /api/runs/<scenario>/start` accepts a `layer3_enabled` field, and
`internal/cli/ui_command.go` uses it to set
`Validation.Layers.SandboxDeploy.Enabled`. That request therefore starts a **real
Scaleway apply**. Credentials come from `os.Getenv("SCW_SECRET_KEY")`, so the
server process holds live keys whenever it was started from a shell that sourced
`layer3.env` — which is precisely the state an operator is in while rehearsing a
Layer 3 demo.

Nothing guarded it. The websocket handler checked `Origin`; the POST handlers
checked nothing, and `startRunHandler` decodes the request body without
inspecting `Content-Type`. A cross-origin `fetch` with `Content-Type: text/plain`
is a CORS **simple** request, so it never triggers a preflight and arrives
intact. The attacker cannot read the response, and does not need to: the side
effect is the goal, and the side effect is infrastructure and money.

**Loopback is not a boundary a browser respects.** It is only a boundary against
processes, and the attacker here is a page, not a process.

## Decision

**A request carrying an `Origin` header is served only if that origin is a
loopback address.** The check wraps the mux, above routing.

Three sub-decisions, each of which could reasonably have gone the other way:

### The guard wraps the mux rather than the endpoints that mutate

Listing today's state-changing endpoints and guarding each reads as coverage and
is a snapshot. The next handler is written by someone who never saw the list, and
is unguarded from the moment it is registered. Above the routing, a handler
cannot opt out by being forgotten.

### Every method, not only the unsafe ones

Filtering to POST/PUT/DELETE rests the guard on "no GET handler ever mutates
anything" — an invariant nothing enforces and that a future handler can quietly
break. Refusing a mismatched origin outright removes the question, and costs
nothing, because a cross-origin GET could not read the response anyway.

### Loopback, not "the same origin as this request"

Comparing `Origin` against the request's own `Host` looks stricter and is weaker,
because **DNS rebinding defeats it**. An attacker domain whose record flips to
`127.0.0.1` makes the browser send `Origin: http://evil.example` *and*
`Host: evil.example:4173`. They agree, an equality check passes them, and the
request lands on the local server.

A rebound request cannot forge a *loopback* origin: the page was served from the
attacker's name, so that name is what the browser reports. Requiring loopback
defeats rebinding and the ordinary drive-by with one rule.

It is also the only rule that keeps development working. The Vite dev server
serves the UI on `:5173` and calls this API on `:4173` — legitimately
cross-origin, entirely loopback. An equality check would have broken it, which is
how the weaker rule announced itself: three existing websocket tests failed.

## Consequences

**An absent `Origin` is allowed.** Browsers always send it on cross-origin
requests, so absence means a non-browser client — curl, a test, a script — and a
non-browser attacker is not the threat. Refusing absent origins would break every
caller that drives this API and prove nothing: a process already running on the
machine can send whatever origin it likes.

**A UI served to a LAN or tailnet address is refused.** `--addr 0.0.0.0:4173`
browsed from another machine sends a non-loopback origin. This is a stance, not
an oversight: this server spends real money with no credential check, and S160b
makes real-cloud apply a start-time decision precisely because a request should
not be able to make it. Stretching the guard to cover a shape the safety model
intends to forbid would be building a hole to spec.

**This is one layer and is not the safety model.** It closes the drive-by. It
does not stop a request escalating the server into spending money, which is a
different property and is S160b's: `layer3_enabled` is removed and real-cloud
apply becomes `infrafactory ui --allow-layer3`, decided in the shell that already
holds the credentials. The second property survives a bug in the first.

**Private Network Access is not a substitute.** Chrome's work blocking
public→private requests would stop some of these, which is why this was
exploitable-in-principle rather than exploitable-everywhere. It is neither
universal nor something this repository controls, and a mitigation you do not own
is not a control.
