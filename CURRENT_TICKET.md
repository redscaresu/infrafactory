# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: S10-T1
- title: Freeze output contract with golden snapshots for all commands/modes
- status: in_progress
- classification: implementation-only

## 1) Problem Statement
- What is broken or missing?
  We do not yet have end-to-end golden snapshots that lock final command output shapes across human/json modes.
- Why does it matter now?
  Slice 10 contract hardening depends on freezing this output contract before remaining reliability work.

## 2) Scope
- In scope:
  deterministic golden snapshots for command outputs and schema assertions in command/output tests.
- Out of scope:
  behavior redesign unrelated to output contract stabilization.

## 3) Acceptance Criteria
1. Command outputs are covered by deterministic golden fixtures in both human and JSON modes where applicable.
2. Snapshot tests fail on output regressions and are straightforward to update intentionally.
3. Existing contract tests continue to validate output schema/version semantics.

## 4) Impacted Areas
- Packages/files expected to change:
  `internal/cli/*_test.go`, snapshot fixtures under `internal/cli/testdata` as needed.
- External contracts affected (CLI/schema/policy):
  no (contract freeze and regression guardrails only).

## 5) Test Plan
- Unit tests:
  focused command/output snapshot tests.
- Integration checks:
  `go test ./...`
- Manual verification:
  `bash scripts/check_all.sh`

## 6) Risks and Rollback
- Primary risks:
  brittle snapshots or accidental normalization drift.
- Rollback approach:
  revert fixture/test changes and regenerate intentionally.

## 7) Done Definition
- Code and tests complete.
- Required docs updated (`STATUS.md`, `BACKLOG.md`, `CURRENT_TICKET.md`).
- Remaining follow-up captured explicitly.

## Test plan
- `go test ./...`
- `bash scripts/check_all.sh`

## Progress notes
- Completed `S10-T2`: normalized CLI error taxonomy with explicit code constants (`usage`, `config_invalid`, `scenario_malformed`, `scenario_invalid`, `dependency_unavailable`, `command_failed`) and representative command-path tests.
- Completed `S10-T3`: versioned run artifacts (`run.json` + `iteration.json`) and added backward-compatible run metadata reader behavior for legacy pre-schema artifacts.
- Completed `S10-T6`: added deterministic explainability summaries for criteria/policy-related failures in human and JSON output contracts.

## Blocker (if any)
- blocker: none.
