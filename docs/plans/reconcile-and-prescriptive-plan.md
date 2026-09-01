# S157a, S156d, S160 — a block of work (planned 2026-09-01)

Three slices, ordered by what it costs to not do them. Sized deliberately small:
S155b's seven passes were a slice-size problem, not a review-effort problem, and
S156c converged in four passes on a slice with one question in it.

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

S157a first: it is the only one where *not* doing it costs money silently, and
the only one whose absence is currently documented as a presence.
