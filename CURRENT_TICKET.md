# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: M29
- title: Scope opt-in LLM raw stage-response capture ticket with redaction/size safeguards and closure-pass notes
- status: done
- classification: implementation-only

## 1) Problem Statement
- What is broken or missing?
  There was no scoped backlog ticket for opt-in persistence of raw LLM stage responses for run debugging.
- Why does it matter now?
  Operators need high-fidelity model-response artifacts to debug stage failures without weakening default secret-safety posture.

## 2) Scope
- In scope:
  Define one implementation ticket that is opt-in, artifact-scoped, redaction-aware, and bounded by size limits; sync planning/state docs.
- Out of scope:
  Runtime implementation changes.

## 3) Acceptance Criteria
1. A dedicated implementation ticket exists in `BACKLOG.md` with scope, acceptance criteria, and required tests.
2. Tracking docs reflect the new planned work and next actionable ticket.
3. Refinement loop is recorded for this planning turn.

## 4) Impacted Areas
- Packages/files changed:
  `BACKLOG.md`,
  `STATUS.md`,
  `CURRENT_TICKET.md`.
- External contracts affected (CLI/schema/policy):
  no.

## 5) Test Plan
- Unit tests:
  n/a (docs-only planning ticket).
- Integration checks:
  optional docs hygiene check.
- Manual verification:
  review backlog row + ticket-details entry consistency.

## 6) Risks and Rollback
- Primary risks:
  underspecified capture scope causing secret leakage or artifact bloat during implementation.
- Rollback approach:
  narrow ticket scope to response-only capture and enforce explicit redaction/cap checks in acceptance criteria.

## 7) Done Definition
- `S17-T1` is scoped and ready for implementation.
- Docs are synchronized to this planning outcome.

## Progress notes
- Added `S17-T1` with explicit scope: opt-in raw response capture only, disabled by default, redaction required, byte caps required, and focused test requirements.
- Updated tracking docs to set `S17-T1` as next unblocked work item.
- Refinement pass 1: improved ticket wording to make redaction and truncation requirements explicit and testable.
- Refinement pass 2: no additional improvements identified.
- Fresh-context approach pass 1: added concrete `S17-T1` execution playbook (activation, artifact layout, safety caps/redaction, and ordered implementation/test steps) to `SESSION_START.md`.
- Fresh-context approach pass 2: no additional improvements identified.

## Blocker (if any)
- blocker: none.
