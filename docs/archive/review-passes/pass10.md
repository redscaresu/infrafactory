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
