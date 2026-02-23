# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: S16-T1
- title: Propagate `cmd.Context()` through all command handlers and runtime operations
- status: in_progress
- classification: implementation-only

## 1) Problem Statement
- What is broken or missing?
  Multiple command handlers call runtime/harness/generator APIs using `context.Background()` instead of `cmd.Context()`.
- Why does it matter now?
  Signal cancellation does not propagate cleanly to long-running subprocesses/HTTP calls, causing poor interrupt behavior during `run`, `validate`, `test`, and mock operations.

## 2) Scope
- In scope:
  Replace `context.Background()` command-path usage with `cmd.Context()` (or derived context) across command adapters and runtime call sites, plus focused tests where practical.
- Out of scope:
  Broader retry-policy or schema/policy semantics changes tracked in later Slice 16 tickets.

## 3) Acceptance Criteria
1. Command handlers in `ISSUES.md` item #16 no longer use `context.Background()` for runtime operations.
2. Runtime/harness/generator/mock operations receive command context for cancellation propagation.
3. Focused regression coverage is added/updated for context propagation behavior.

## 4) Impacted Areas
- Packages/files changed:
  `internal/cli/validate_command.go`,
  `internal/cli/test_command.go`,
  `internal/cli/generate_command.go`,
  `internal/cli/mock_start_command.go`,
  `internal/cli/mock_stop_command.go`,
  `internal/cli/mock_status_command.go`,
  `internal/cli/mock_logs_command.go`,
  related tests.
- External contracts affected (CLI/schema/policy):
  no (behavioral robustness only).

## 5) Test Plan
- Unit tests:
  `go test ./internal/cli`
- Integration checks:
  `go test ./...`
  `bash scripts/check_all.sh`
- Manual verification:
  run long-running command flow and confirm Ctrl+C cancellation propagates to in-flight operations.

## 6) Risks and Rollback
- Primary risks:
  incomplete call-site coverage causing mixed context behavior.
- Rollback approach:
  revert context-wiring changes and reapply with stricter call-site audit.

## 7) Done Definition
- Command-path context propagation is complete for scope listed in issue #16.
- Focused tests and full checks pass.

## Progress notes
- Completed maintenance planning ticket `M28`: Slice 16 defined and ticketed (`S16-T1`..`S16-T8`) from `ISSUES.md`.
- Fresh-context docs synced for Slice 16 startup and execution constraints.
- Slice 16 planning refinement loop completed with two consecutive no-change passes.

## Blocker (if any)
- blocker: none.
