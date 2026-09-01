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

## Amendment, 2026-09-01 (S160b): real cloud is decided at start time

The origin guard closes the drive-by. It does not close the separate idea that a
**request** can escalate the server into spending money, and those are different
properties: the second survives a bug in the first.

`layer3_enabled` is removed from `StartRunRequest`. Real-cloud apply for runs
started from the UI is now settled by `infrafactory ui --allow-layer3`, read once
when the server starts — a decision made in the shell that already holds the
credentials, by the person who typed the command.

### The config file does not get a vote either

This is the stricter half, and it has a live failure mode rather than a
theoretical one.

The per-run configuration is **re-read from disk on every run**, which is what
lets an operator edit `infrafactory.yaml` without restarting the UI. So a file
carrying `sandbox_deploy.enabled: true` would walk real-cloud apply straight back
in on a server started *without* `--allow-layer3` — silently, and on every run
after it. `uiRunStarter.configLoader` re-applies the server's decision over the
freshly loaded file for exactly this reason.

`sandbox_deploy.enabled` is a checked-in setting that says what this repository
does when somebody runs a scenario deliberately. It should not also mean "and the
web server may spend money on its own", because nobody re-reads a config file at
the moment they start a UI.

### What the UI shows instead of a checkbox

The scenario page had a Layer 3 checkbox. It now reports what the server decided
and says how to change it, because a control implies this page can change
something it cannot. The status field was renamed `config_default_enabled` →
`server_allows_layer3`: the old name described a *default* a client could
override, and no client can. A field named for an overridable default, serving a
value nothing can override, is a lie the next reader has to discover for
themselves.

`run.json` keeps its own `layer3_enabled` field. That is a **record of what a run
did**, not a request for what it should do — the same word for two different
things, and only the request side is removed.

### Consequence

An operator who ran the UI with `sandbox_deploy.enabled: true` in their config and
used the checkbox now gets no real-cloud apply until they restart with
`--allow-layer3`. That is the intended behaviour change and the whole point of
the slice: it makes the moment of authorisation explicit and attributable to a
person, rather than implicit in a file.

## Amendment, 2026-09-02 (S159b): destructive actions are start-time too

`DELETE /api/deployments/{id}` and `POST /api/deployments/reap` exist only when
the server was started with `infrafactory ui --allow-teardown`. Without it the
actor is nil and the routes answer **404** — the capability does not exist, so
there is nothing to bypass.

That is the same rule as `--allow-layer3`, applied to the other direction of
harm. Teardown is not safe merely because it removes rather than creates: it is
irreversible, it acts on real infrastructure, and a demo torn down mid-talk costs
something even though the bill goes down.

**404, not 501.** Announcing "not implemented" would advertise a capability the
operator declined.

**And it fails loudly when asked for.** If `--allow-teardown` is given and the
teardown runtime cannot be built, the server refuses to start rather than coming
up without it. A UI silently missing a capability the operator requested is a
guard that stops without saying why (ADR-0023).

### The guards are not re-expressed at the seam

`LiveActions` calls `tearDownDeployment` — the same function the CLI calls — and
does nothing but translate its staged output. Teardown refuses unless a run-owned
marker *and* the API's own provenance both say the project is infrafactory's, and
it declines to mark a deployment released when its state has vanished, because
that would retire the only record saying its resources may still exist. None of
that is restated here. **A second implementation of a guard is a second thing
that can be wrong.**

### A partial teardown is a 409

ADR-0024's rule is that a teardown which cannot *prove* the account clean must
not report success. `ActionResult.Clean` is its own field rather than "no
failures", and a result carrying failures answers 409 — a page rendering a green
tick over *"the state file has vanished and the resources may still be running"*
is exactly the false green this project exists to avoid.

### A destroy is not cancellable by the caller

`r.Context()` is cancelled when the client disconnects — closing the tab,
navigating away, a flaky wifi hop. A teardown cancelled halfway has already
deleted some resources and not others, and the live record then describes neither
the old state nor the new one.

So the destructive handlers run on a context detached from the request, with a
generous timeout as a backstop against a hung provider call rather than as a
deadline. This is the same rule `ensureRunProject` applies to creating a run's
project: **once an operation begins changing real infrastructure, the caller
going away must not stop it.** Whoever asked can leave; the destroy finishes and
the record ends up describing what is actually there.
