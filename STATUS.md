# STATUS

Last updated: 2026-02-22

## Current phase
- Active milestone: Slice 1 + Slice 2
- Next gate: runnable CLI commands + config/scenario parsing + generator parser tests
- Current ticket: S1-T1
- Next ticket: S1-T2

## In progress
- `internal/cli` command wiring
- `internal/config` and `internal/scenario` implementations
- `internal/generator` parser and prompt helpers

## Known blockers
- `go test ./...` fails: missing `go.sum` entry for `github.com/santhosh-tekuri/jsonschema/v6`

## Next actions
1. Run `go mod tidy`.
2. Implement Slice 1 packages + tests.
3. Implement Slice 2 parser/template core + tests.

## Update policy
- Update at end of each meaningful coding session.
- Keep concise and factual.
- Move old detail to `docs/status/ARCHIVE.md`.
- Put durable architecture decisions in ADRs and `CONCEPT.md`.
- Keep startup/read-order instructions only in `SESSION_START.md` to avoid duplication.

## Recent updates
- Added reusable prompt at `docs/process/EXECUTION_PROMPT.md`.
- Linked reusable prompt from `README.md` and `SESSION_START.md`.
- Added README kickoff instruction for fresh sessions.
- Added `BACKLOG.md` as single ticket status source.
- Added `CURRENT_TICKET.md` as session execution stub.
- Added `scripts/check_all.sh` to run tests + doc hygiene in one command.
- Updated startup/contributor flow to use backlog + current-ticket files.
- Set `BACKLOG.md` active work state: `S1-T1` is `in_progress` (single active ticket).
- Added Apache-2.0 `LICENSE` and linked license section in `README.md`.
