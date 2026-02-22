# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: M23
- title: Plan Slice 15 adaptive retry and transport-resilience policy
- status: done
- classification: decision-impacting

## 1) Problem Statement
- What is broken or missing?
  Current planning covers feedback fidelity (Slice 14) but does not yet dedicate a slice to adaptive retry behavior when failures are transport-dominated (timeouts, killed subprocess, dependency outages).
- Why does it matter now?
  Without explicit retry-governance policy, run loops can consume iteration budget on non-model-correctable failures and emit low-actionability terminal outcomes.

## 2) Scope
- In scope:
  Define/refine a new Slice 15 ticket set for adaptive retry and transport-resilience behavior and wire it into roadmap/status/fresh-context guidance.
- Out of scope:
  Runtime implementation changes for retry policy.

## 3) Acceptance Criteria
1. Backlog contains explicit Slice 15 tickets (`S15-T1`..`S15-T6`) covering contract, implementation, tests, artifacts, and docs.
2. Roadmap includes Slice 15 milestone and execution sequencing after Slice 14.
3. Status and session-start guidance include Slice 15 fresh-context requirements.

## 4) Impacted Areas
- Packages/files changed:
  `BACKLOG.md`,
  `ROADMAP.md`,
  `STATUS.md`,
  `SESSION_START.md`,
  `CURRENT_TICKET.md`.
- External contracts affected (CLI/schema/policy):
  yes (planned run retry-governance and transport diagnostics behavior; implementation pending).

## 5) Test Plan
- Unit tests:
  n/a (planning/docs-only change)
- Integration checks:
  `bash scripts/check_all.sh`
- Manual verification:
  confirm Slice 15 sequence, dependencies, and acceptance criteria are implementation-ready and aligned with existing Slice 12-14 direction.

## 6) Risks and Rollback
- Primary risks:
  retry-governance scope can overlap with Slice 14 unless boundaries remain explicit (`S14` = feedback fidelity, `S15` = adaptive retry policy).
- Rollback approach:
  revert Slice 15 planning entries and preserve prior milestone ordering.

## 7) Done Definition
- Slice 15 adaptive-retry plan documented and tracked.
- Tracking docs updated.

## Test plan
- `bash scripts/check_all.sh`

## Progress notes
- Added planning ticket `M23` and Slice 15 execution tickets `S15-T1`..`S15-T6` in `BACKLOG.md`.
- Updated `ROADMAP.md` with dedicated Slice 15 milestone and near-term sequencing after Slice 14.
- Updated `STATUS.md` next-actions to queue Slice 15 adaptive retry/transport-resilience work after Slice 14 closure.
- Updated `SESSION_START.md` fresh-context notes with Slice 15 adaptive retry and transport diagnostics guidance.

## Blocker (if any)
- blocker: none.
