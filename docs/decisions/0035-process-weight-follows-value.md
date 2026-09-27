# ADR-0035: Process weight follows value

## Status
Accepted — 2026-09-27 (S196)

## Context

Measured on 2026-09-27, before this arc:

- A fresh session's mandatory reading was **~80k tokens**, 63k of it `STATUS.md` (3,938 lines,
  about +41 per PR, never pruned). `docs/NEXT_SESSION.md`, which the checklist says to read
  *first*, had not been touched since 2026-08-31 and pointed at a slice that had shipped.
- S194 added ~2 KB of logic and 6 KB of tests, and **~25 KB of prose**: one story told in the
  ADR, `STATUS.md`, the commit, the PR body, the review pass and the code comments.
- **18 of the last 30 PRs** touched an ADR, because the hygiene check demanded one for any
  change under `internal/cli/`. Amendments had become changelogs.
- Pre-commit ran the full suite (**~3-4 min**), which CI then runs again as a required check.

The quality came from the gates: CI, the codex loop, the audit tests, never merging red. The
cost came mostly from prose and duplicated runs. So the cuts go there and the gates stay.

## Decision

1. **Pre-commit is fast; CI owns the full suite.** The hook runs its checks and tests only the
   Go packages the commit touches (plus UI unit tests for `ui/src`). `PRECOMMIT_FULL=1`
   restores `make test`. CI is unchanged and still blocks merge.
2. **An ADR edit is required for a decision, not for a path.** A change under the decision
   paths that crosses no threshold carries `ADR: none — <reason>` in a commit message instead.
   The reason is mandatory, so the classification still has to be made and written down.
3. **One entry point, current state only.** `STATUS.md` holds the present: the active arc,
   open items, and recent slices as one line each. It is capped, and CI enforces the cap. History
   lives in `docs/status/ARCHIVE.md` and git. `docs/NEXT_SESSION.md` is removed. (S197)
4. **Each story is told once.** The PR body is the full account; the commit is its summary;
   `STATUS.md` gets a line; an ADR only for a decision; a review-pass file only when findings
   were declined; comments state the current rule and its reason, not its history. (S198)

## Consequences

- A broken change in one package can pass pre-commit and fail in CI. That is where it was
  always caught; the hook stops pretending to be the gate.
- Most history stops being written, not just stops being loaded. Nothing that was a
  decision is lost: decisions still get ADRs, and every PR body is kept by GitHub.
- The ADR index is generated (S195), so an ADR costs one file, not a file plus an index edit.
