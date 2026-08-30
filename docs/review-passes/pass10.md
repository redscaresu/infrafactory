# S153a/b review pass 10 — one finding acted on, several declined

`codex exec review --base main` on `s153a-deploy-leak-fixes`.

Codex returned **one** finding. A Claude `/code-review` pass run beforehand
returned fifteen; the triage below records which of those were acted on anyway
and which were declined, because "another reviewer said so" is not a reason on
its own.

## Acted on

### [P1] Preserve prior sweep stray failures before releasing — `live_teardown.go:88`

Raised independently by both reviewers, which is why it led the triage. The
empty-state release path rebuilt the sweep target as
`&SweepTarget{ProjectID: d.ProjectID}` with **nil `Strays`**. Where the first
pass failed on resources *outside* the run project, those strays are computed by
`CaptureSweepTarget` from state that destroy has since emptied, so they cannot
be recomputed. The re-verification would find the project already gone, report
clean, and release — laundering the failure the branch was added to prevent.

Fixed with a sticky `SweepVerificationFailed` flag on the deployment record: set
when a sweep fails, and checked before the empty-state path, which now refuses
and points at `live forget`. Positive verification, never the absence of contrary
evidence.

### `live forget` could release a healthy deployment (Claude pass only)

Acted on despite Codex not raising it: the command took any id and released it
with no state check and exit 0, so a mistyped-but-existing id would make
`Reapable()` false forever and leave the project billing untracked. It now
refuses anything teardown could still handle.

### `MarkReleased` wrote outside the store before rejecting (Claude pass only)

`validateID` ran inside `write`, after the fallback path had already
`ReadFile`/`WriteFile` through `s.path(id)`. Validation moved ahead of every side
effect.

### `CLAUDE_CODE_` prefix stripped the credential (Claude pass only)

The bare prefix took `CLAUDE_CODE_OAUTH_TOKEN` — how `claude` authenticates
where no `ANTHROPIC_API_KEY` is set — plus `USE_BEDROCK`/`USE_VERTEX`. An
explicit keep-list now shields credentials and provider routing. This would have
broken generation in CI, which is worse than the hang it was fixing.

### `WaitDelay` dropped from `exec_runner` (Claude pass only)

It applied to **every** command, and its timer also starts when the child exits
normally — so a provider plugin still holding the output pipe would turn a
successful apply into `exec.ErrWaitDelay` with truncated output. `Cancel`
(SIGINT rather than SIGKILL) is kept; the interactive double-interrupt caveat is
documented at the call site rather than silently accepted.

## Declined

- **`LiveStateFilename` duplicated from `harness`.** True that no import cycle
  forces it, but the constant is pinned by a test and the duplication keeps
  `livestore` dependency-free. Cosmetic; the drift check already exists.
- **`randomIDSuffix`'s `"000000"` fallback is dead code.** Correct — `crypto/rand.Read`
  no longer returns an error — so it is unreachable rather than a silent-degradation
  risk. Not worth the churn.
- **Deployment id validated at `Put` rather than before the apply.** The stated
  threat model does not hold: `scenario.schema.json` constrains `scenario` to
  `^[a-z][a-z0-9-]*[a-z0-9]$`, so a traversing id cannot arrive by that route. The
  reviewer noted this itself while raising it.
- **`DefaultHealthPath` / `ServiceSpec.HealthPath` unread.** Parsed from the
  schema and consumed by S154's soak probe. Deleting and re-adding is churn.
- **Style nitpicks.** Per AGENTS.md triage: act on correctness and safety, not on
  taste.

## Outcome

One P1 acted on, four Claude-only safety findings acted on, five declined with
reasons. Full suite and `go vet` clean. Re-running for a clean pass.

---

# Pass 11 — three findings, all acted on

`codex exec review --base main` after the pass-10 fixes. No nitpicks this time;
all three were real.

### [P1] Sweep-failure marker write was dropped — `live_teardown.go:153`

`_ = store.Put(d)`. If the write failed (read-only store, full disk), the sticky
flag stayed false, so the next pass would take the empty-state path and release
while the unseen strays kept billing — silently undoing the pass-10 fix. A failed
marker write is now itself a teardown failure, and says not to expect a re-run to
refuse.

### [P2] `forget` rejected exactly the record `teardown` refused

A dead end created while closing the previous one: teardown refuses a sticky
empty-state record and tells the operator to use `live forget`, but `reclaimable`
returned true for any existing state file — so forget bounced it back to
teardown. No CLI escape hatch at all without hand-editing files. `reclaimable`
now returns false for that exact combination.

### [P2] No kill fallback for a cancelled command

Dropping `WaitDelay` in pass 10 removed the false-failure risk but also removed
the escape: a `tofu` that ignores SIGINT would hang forever, and on `deploy` that
prevents registration — recreating the unrecorded-live-resource path the signal
guard exists to close. `Cancel` now sends SIGINT and arms a SIGKILL fallback,
scoped to cancellation only, so a normal exit can never trip it.

---

# Pass 12 — one finding, acted on

### [P2] `forget` rejected invalid records teardown cannot reclaim

The pass-11 fix closed the dead end for records whose sweep had failed, but not
for records that decode while missing what teardown needs. A record with an empty
`project_id` counted as reclaimable purely because a state file existed, so
`forget` refused it while teardown failed at `AssertProjectDeletable` — the same
no-escape loop, one class along.

`reclaimable` now also requires a project id. The general rule, now stated in
ADR-0024: **whenever a guard refuses and names a remedy, the remedy must accept
exactly that case** — and it wants a test that walks the pair, not each half
alone.

---

# Pass 13 — clean

`codex exec review --base main`:

> I did not find any discrete correctness, safety, or maintainability issues
> introduced by the diff. The changed live-deployment paths are covered by
> focused tests and the touched Go packages pass their test suites.

Converged after four Codex passes (10–13). Three earlier passes used Claude's
`/code-review` skill and are not counted: they found real defects, but a
same-family reviewer shares the blind spots that produced them, and each round of
fixes then reproduced the failure it targeted. The Codex passes behaved
differently — pass 10 returned **one** finding where the Claude pass had returned
fifteen, and that one finding was the most serious of the lot.

---

# Pass 14 (#172, ADR-0025) — one finding, acted on

### [P2] `docs/NEXT_SESSION.md` not repointed

A docs-only planning PR that announces a new arc while the documented
fresh-context handoff still names a closed one. `AGENTS.md:26` makes
`NEXT_SESSION.md` the fresh-session entry point and `AGENTS.md:43` explicitly
requires repointing it, so a fresh agent would have started the *presentable*
arc — closed — instead of the live-services work.

Verified before acting: the file was dated **2026-08-22**, predating the entire
presentable arc close as well as everything since. Rewritten to carry the active
arc, the two-gates blocker that stops any Scaleway compute scenario today, the
three planned arcs, and the operational gotchas that cost real time this session
(Codex vs `/code-review`, the `CLAUDE_CODE_*` env family, where Layer 3
credentials live, and the EUR 0.042/hour figure).

A good catch for a process rule rather than a code path: the plan was correct and
would still have been invisible to the next reader.
