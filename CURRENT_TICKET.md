# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: S7-T1
- title: Wire `init` command scaffold generation + next-step output
- status: in_progress

## Goal
- Make `infrafactory init` produce a minimal valid scenario scaffold and clear deterministic next steps.

## Scope
- In scope: `internal/cli` init command behavior, scaffold content generator, next-step command hints.
- Out of scope: generate/validate/test/run orchestration.

## Acceptance criteria
1. `infrafactory init` writes a minimal valid scenario scaffold.
2. Scaffold includes required schema fields and comments/hints for next edits.
3. Command prints deterministic next-step commands.

## Slice 7 Defaults (Fresh Context)
- Execution order:
  `S7-T1 -> S7-T2 -> S7-T12 -> S7-T16 -> S7-T3/S7-T4/S7-T5 -> S7-T6 -> S7-T7 -> S7-T8 -> S7-T9 -> S7-T11 -> S7-T15`
- Parallel lane:
  `S7-T10` (`mock start`) can proceed after `S7-T12`.
- Supporting/optional lane:
  `S7-T13` is supporting infrastructure; `S7-T14` is opt-in smoke only.
- Output contract defaults:
  deterministic human summary + deterministic JSON schema/output ordering.
- Test policy:
  hermetic tests are the default path; real-tool smoke tests stay opt-in.

## Test plan
- `go test ./...`
- `bash scripts/check_all.sh`

## Progress notes
- Completed slices 1 through 6 internal primitives and focused tests.
- Created and optimized Slice 7 orchestration backlog with shared runtime ticketing, reduced serial deps, and explicit output-contract freeze (`S7-T16`).
- Starting `S7-T1` implementation now.

## Blocker (if any)
- blocker: none
- attempts: n/a
- required input: none
