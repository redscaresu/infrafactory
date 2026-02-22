# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: M7
- title: Add fresh-context repo-state preflight guidance and verify consecutive no-change passes
- status: done
- classification: implementation-only

## 1) Problem Statement
- What is broken or missing?
  `SESSION_START.md` still lacked explicit repo-state preflight commands and ticket-discipline reminders that reduce fresh-context execution mistakes.
- Why does it matter now?
  Fresh sessions often fail early due to unnoticed dirty worktrees or ambiguous active-ticket state.

## 2) Scope
- In scope:
  Add startup preflight checks and ticket-discipline reminders, then validate via iterative no-change review passes.
- Out of scope:
  Implementing generator transport features themselves.

## 3) Acceptance Criteria
1. `SESSION_START.md` includes repo-state preflight commands and explicit unexpected-change halt reminder.
2. `SESSION_START.md` includes explicit single-`in_progress` ticket discipline reminder.
3. Review loop reached two consecutive no-change passes.

## 4) Impacted Areas
- Packages/files changed:
  `SESSION_START.md`,
  `BACKLOG.md`,
  `STATUS.md`,
  `CURRENT_TICKET.md`.
- External contracts affected (CLI/schema/policy):
  no.

## 5) Test Plan
- Unit tests:
  n/a (docs-only ticket).
- Integration checks:
  n/a
- Manual verification:
  startup-guidance pass-by-pass review with explicit consecutive no-change confirmation.

## 6) Risks and Rollback
- Primary risks:
  startup doc drift as workflows evolve.
- Rollback approach:
  revert startup doc synchronization changes.

## 7) Done Definition
- Fresh-context startup guidance improved and validated through consecutive no-change passes.
- Required docs updated (`STATUS.md`, `BACKLOG.md`, `CURRENT_TICKET.md`).
- Remaining follow-up captured explicitly.

## Test plan
- `go test ./...`
- `bash scripts/check_all.sh`

## Progress notes
- Pass 1: added startup preflight guidance in `SESSION_START.md`:
  - `git status --short`, `git branch --show-current`, `git log -1 --oneline`
  - explicit reminder to stop and ask on unexpected local changes
  - explicit reminder to keep exactly one `in_progress` ticket in `BACKLOG.md`
- Pass 2: no further improvements identified.
- Pass 3: no further improvements identified (second consecutive no-change pass; review complete).

## Blocker (if any)
- blocker: none.
