# S160a, S160b, S157a, S156d — a block of work (planned 2026-09-01)

Four slices, ordered by what it costs to not do them. Sized deliberately small:
S155b's seven passes were a slice-size problem, not a review-effort problem, and
S156c converged in four passes on a slice with one question in it.

## Revised the same day, before any of it was built

The block was written as S157a → S156d → S160, with S160 ("decide the deploy
safety model before the capability exists") last because the deploy button does
not exist yet.

**The button already exists. It is spelled `layer3_enabled`.**

`POST /api/runs/<scenario>/start` accepts a `layer3_enabled` field, and
`ui_command.go:194` uses it to *set* `Validation.Layers.SandboxDeploy.Enabled` —
so that request triggers a real Scaleway apply. Nothing guards it:

- the **websocket** handler checks `Origin` (`server.go:238`); the **POST**
  handlers check nothing — no origin, no CSRF token;
- `startRunHandler` decodes the body without inspecting `Content-Type`, so a
  cross-origin `fetch` with `Content-Type: text/plain` is a CORS *simple*
  request and never triggers a preflight;
- credentials come from `os.Getenv("SCW_SECRET_KEY")`, so the UI process holds
  live Scaleway keys whenever it was started from a shell that sourced
  `layer3.env` — which is the state an operator is in while rehearsing a Layer 3
  demo.

The attacker cannot read the response. They do not need to: the side effect is
the attack, and the side effect is real infrastructure and real money.

Chrome's Private Network Access work blocks some public→private requests, so this
is not universally exploitable today. That is a mitigation to note and not one to
depend on — it is neither universal nor something this repo controls.

So the safety model is not a prerequisite for a future arc. It is a live hole,
and it moves to the front.

### What this changed about the UI arc's readiness

Asked "how much more before the UI?", the honest answer is **less than this plan
originally proposed, plus one thing it did not contain**:

- **S156d does not block the UI at all.** It is the live-learning arc.
- **S157a does not strictly block it either** — the reconcile hole simply gets
  worse when the UI succeeds.
- **S160 does block it**, and now blocks everything, for the reason above.

Two things the UI arc plan does not name and should:

1. **S159 needs a cobra-free seam.** It reads as "add `/api/deployments`
   handlers", but `runDeployCommand` takes a `*cobra.Command`: it reads the TTL
   from flags and writes through `writeCommandOutput(cmd, …)`. Something must
   become callable without one, mirroring how `uiRunStarter` implements the API's
   interface from `internal/cli`. That extraction is probably the largest part of
   S159.
2. **Concurrency.** `uiRunStarter` serialises with a `busy` mutex — one run at a
   time. Deployments are many and long-lived, and `RecordObservation` is a
   read-modify-write with no locking, so two concurrent observers lose an
   observation. Rare from a CLI; not rare from a page that polls.

## S160a — an origin guard on state-changing requests

**The one question: what makes a state-changing request safe on an
unauthenticated localhost server?**

Not "what makes the deploy endpoint safe". The guard belongs at the **mux seam**,
wrapping every handler, so an endpoint added later cannot forget it. An audit
that lists today's mutating endpoints and guards each one reads as coverage while
being a snapshot — and the next handler is written by someone who never saw the
list.

The rule:

- a state-changing method (anything but GET/HEAD/OPTIONS) with an `Origin` that
  is not the server's own is **rejected**;
- an `Origin` that is absent is **allowed**, and that is deliberate rather than a
  gap. Browsers always send `Origin` on cross-origin state-changing requests, so
  absence means a non-browser client — curl, a test, a script — and a non-browser
  attacker is not the threat model here. The threat is a page in the operator's
  browser. Refusing absent origins would break every script and prove nothing;
- the allowlist derives from the **configured bind address** plus loopback, so
  `--addr` on a tailnet or a LAN address does not silently lock the operator out
  of their own UI.

## S160b — real cloud is decided at start time, not on the wire

**The one question: who decides to spend money?**

The `layer3_enabled` field is removed. Real-cloud apply becomes
`infrafactory ui --allow-layer3`: a decision the operator makes in the shell that
already holds the credentials, rather than a field any request can set.

An origin guard closes the drive-by. This closes something else — the idea that a
*request* can escalate a server into spending money. Those are different
properties, and the second survives a bug in the first.

## S157a — reconcile against the API (report-only)

**The one question: what is running that nothing knows about?**

`internal/livestore/livestore.go:33` states, as fact, that the reaper
"reconciles against the API rather than trusting this file alone." It does not.
`runLiveReapCommand` calls `store.Reapable()` and never contacts Scaleway.
ADR-0024 already names this as "the largest remaining hole" in it.

The failure is quiet and expensive. `.infrafactory/live` sits inside the working
directory. Delete the directory, check out a different branch, run from a fresh
clone — and the records are gone while the load balancer, the instance and the
two public IPv4s keep running, with a TTL nobody will ever enforce. **Every
signal in the system reports clean**, because every signal reads the store.

This is the shape of D6: a leak whose only symptom is the bill.

### Why it can be precise rather than heuristic

Since the S166/S167 cutover, every live deployment gets its own project through
`ensureRunProject`, stamped with `RunProjectNamePrefix` (`if-run-`) and the fixed
`RunProjectDescription`. `ProjectProvenance` and `Describe` already read that
stamp — it is what `assertRunProjectOurs` guards teardown with.

So reconciliation is a set difference over a stamp infrafactory itself wrote:

- **stamped projects the store does not know** → running infrastructure with no
  record. The expensive case.
- **records naming projects that no longer exist** → a record outliving its
  resources. Harmless to the bill, but it makes `live ls` a lie.

A project without the stamp is never considered, in either direction. The
existing containment rule is unchanged and unweakened: infrafactory does not
reason about projects it did not create.

### Report-only, and that is the design

`live reconcile` **never destroys anything.** An orphan is by definition a thing
the system's records do not explain, and destroying what you cannot explain is
how a reconciler becomes the incident. It prints project ids and what it can
determine about each, and a human decides.

Scope:
- `ScalewayRunProject.List` — paginated, filtered to the stamp.
- `live reconcile` (`--output json` like every other command), plus its findings
  surfaced by `live ls` so the drift is visible without a second command.
- Fix the comment at `livestore.go:33` to describe what is true.
- The reaper stays store-driven; making it API-driven is a bigger change and
  wants the report to exist first.

## S156d — prescriptive rules from upgrade diffs

**The one question: is the before/after pair a usable diff?**

S156c writes **descriptive** rules — what was observed, and nothing about what to
do. That is the weakest useful class, and ADR-0019's `fix` entries are the strong
one. `ExtractFixPitfall` produces them by diffing a failing configuration against
a passing one.

A live failure has no "next iteration that fixed it". An **upgrade** does: S155b
stashes what was running into `.infrafactory-previous/` for exactly this, and the
comment there says so.

The honest risk, stated up front: an upgrade diff is a diff between two things
that both applied successfully. It is not obviously the same shape as
"this failed, then this passed", and the slice may conclude that the pair is
**not** usable unchanged. That is a real outcome and not a failed slice — it is
cheaper to learn it here than to ship a prescriptive rule that prescribes the
wrong thing.

## S160 — the deploy safety model, as an ADR

**The one question: what makes a deploy button safe on an unauthenticated
localhost web app?**

Decided before the capability exists, per the UI arc plan. No endpoint in this
slice — a decision document and the config flag it names, so S159 and S162 have
something to build against rather than something to argue with.

The candidates the UI plan already lists: `ui.allow_deploy: false` by default, an
origin/CSRF guard against drive-by POSTs from any page in the user's browser, a
typed confirmation naming the scenario, and refusing a non-loopback bind while
deploy is enabled.

## Not in this block

**S156e, the validation run** — one live-sourced pitfall that demonstrably
prevents a repeat, proven by generating the scenario with it absent and then
present. It is the arc's proof and it needs real cloud time and real money, so it
is the user's call rather than something to slot into a work block.

## Order

**S160a, S160b, S157a, S156d.**

The guard first, because it is the only one of the four that is open right now
rather than latent. Then S157a, which is the only one where *not* doing it costs
money silently and whose absence is currently documented as a presence. S156d
last: it is the only one with no safety content at all.
