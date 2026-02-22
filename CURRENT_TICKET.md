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
