# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: M20
- title: Plan Slice 13 full app-logic logging/observability backlog before implementation
- status: done
- classification: decision-impacting

## 1) Problem Statement
- What is broken or missing?
  There was no dedicated slice for full application-logic logging, making observability work under-scoped and mixed with unrelated iteration-contract migration tasks.
- Why does it matter now?
  A standalone logging slice is needed so instrumentation can be delivered coherently with contract-first guarantees, regression coverage, and operator runbooks.

## 2) Scope
- In scope:
  Define/refine a new Slice 13 ticket set for full app logging/observability, and wire it into roadmap/status/fresh-context guidance.
- Out of scope:
  Any logging implementation changes.

## 3) Acceptance Criteria
1. Backlog contains explicit Slice 13 tickets (`S13-T1`..`S13-T6`) for logging contract, implementation, tests, and docs.
2. Roadmap includes Slice 13 milestone and execution sequencing after Slice 12.
3. Status and session-start guidance include Slice 13 fresh-context requirements.

## 4) Impacted Areas
- Packages/files changed:
  `BACKLOG.md`,
  `ROADMAP.md`,
  `STATUS.md`,
  `SESSION_START.md`,
  `CURRENT_TICKET.md`.
- External contracts affected (CLI/schema/policy):
  yes (planned CLI/config contract migration; implementation pending).

## 5) Test Plan
- Unit tests:
  n/a (planning/docs-only change)
- Integration checks:
  `bash scripts/check_all.sh`
- Manual verification:
  confirm Slice 13 execution order and ticket dependencies are explicit and implementation-ready.

## 6) Risks and Rollback
- Primary risks:
  logging scope could sprawl unless contract-first boundaries are explicit in `S13-T1`.
- Rollback approach:
  revert planning entries and keep prior milestone ordering.

## 7) Done Definition
- Slice 13 app-logging plan documented and tracked.
- Tracking docs updated.

## Test plan
- `bash scripts/check_all.sh`

## Progress notes
- Added planning ticket `M20` and Slice 13 execution tickets `S13-T1`..`S13-T6` in `BACKLOG.md`.
- Updated `ROADMAP.md` with dedicated Slice 13 milestone and near-term sequencing.
- Updated `STATUS.md` next-actions to queue Slice 13 immediately after Slice 12 closure.
- Updated `SESSION_START.md` fresh-context notes for Slice 13 logging contract-first and redaction/correlation requirements.
- Refinement pass added explicit Slice 13 sink expectations (`stderr` + run-scoped log artifact path) and required fresh-context log-inspection command guidance.
- Refinement pass 2: no additional improvements identified.
- Refinement pass 3: no additional improvements identified (second consecutive no-change pass).

## Blocker (if any)
- blocker: none.
