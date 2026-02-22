# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: M18
- title: Strengthen fresh-context startup documentation with run-loop and Mockway operational guardrails
- status: done
- classification: implementation-only

## 1) Problem Statement
- What is broken or missing?
  Fresh sessions still required rediscovering several operational realities (prefer `run` loop, avoid heuristic normalization, Mockway port collision diagnosis, artifact locations).
- Why does it matter now?
  This creates avoidable churn at session start and increases time-to-first-correct-change.

## 2) Scope
- In scope:
  Update session-start docs with high-signal operational addenda for current workflow and troubleshooting.
- Out of scope:
  Runtime behavior changes.

## 3) Acceptance Criteria
1. Fresh-context guidance explicitly calls out `run` as the iterative feedback path.
2. Fresh-context guidance explicitly warns against new heuristic normalization patches.
3. Fresh-context guidance includes concrete Mockway and artifact-inspection operational guardrails.

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
  n/a (docs-only change)
- Integration checks:
  `bash scripts/check_all.sh`
- Manual verification:
  read startup guide and confirm key operational guardrails are available without additional repository spelunking.

## 6) Risks and Rollback
- Primary risks:
  low; documentation drift if behavior changes and startup docs are not kept in sync.
- Rollback approach:
  revert startup addenda section.

## 7) Done Definition
- Fresh-context startup guardrails documented and tracked.
- Tracking docs updated.

## Test plan
- `bash scripts/check_all.sh`

## Progress notes
- Added `Fresh Context Addenda (Operational)` section to `SESSION_START.md`.
- Documented canonical iterative path (`run`), anti-normalization direction, Mockway port-collision handling, and run artifact debug path.

## Blocker (if any)
- blocker: none.
