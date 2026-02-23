# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: M27
- title: Re-baseline Slice 12 planning/docs for dual iteration controls and fresh-context readiness
- status: done
- classification: decision-impacting

## 1) Problem Statement
- What is broken or missing?
  Slice 12 planning and fresh-context instructions still describe single-control iteration migration (`iterations` + legacy compatibility) and do not encode the requested dual-control behavior.
- Why does it matter now?
  Fresh contexts need one unambiguous contract before implementation starts, and closure workflow must explicitly include README optimization after slice completion.

## 2) Scope
- In scope:
  Add ADR for dual controls, re-baseline Slice 12 planning docs (`BACKLOG.md`, `ROADMAP.md`, `SESSION_START.md`, `STATUS.md`), and run refinement passes until two consecutive no-change outcomes.
- Out of scope:
  Code implementation of Slice 12 runtime/CLI behavior.

## 3) Acceptance Criteria
1. Dual-control ADR is added and indexed.
2. Slice 12 planning/fresh-context docs align to `repair_iterations_max` + `iterations_target` semantics.
3. Refinement loop records one improvement pass and two consecutive no-change passes.

## 4) Impacted Areas
- Packages/files changed:
  `docs/decisions/0005-dual-iteration-controls.md`,
  `docs/decisions/README.md`,
  `BACKLOG.md`,
  `ROADMAP.md`,
  `SESSION_START.md`,
  `STATUS.md`,
  `CURRENT_TICKET.md`.
- External contracts affected (CLI/schema/policy):
  yes (public CLI/config behavior contract and workflow governance).

## 5) Test Plan
- Unit tests:
  n/a (docs/ADR-only change)
- Integration checks:
  `bash scripts/check_all.sh`
- Manual verification:
  confirm Slice 12 docs consistently reference dual controls and deterministic terminal reasons.

## 6) Risks and Rollback
- Primary risks:
  stale references to old Slice 12 migration semantics may remain in historical sections.
- Rollback approach:
  revert M27 docs and restore prior planning text.

## 7) Done Definition
- ADR and planning docs are synchronized.
- Two consecutive no-change refinement passes are recorded.
- Fresh context is ready to start `S12-T2`.

## Test plan
- `bash scripts/check_all.sh`

## Progress notes
- Added ADR-0005: `docs/decisions/0005-dual-iteration-controls.md`.
- Updated ADR index in `docs/decisions/README.md`.
- Re-baselined Slice 12 tickets/criteria in `BACKLOG.md` to dual controls and canonical terminal reasons.
- Updated `ROADMAP.md` Slice 12 milestone and near-term order, including README optimization after Slice 12 closure.
- Updated `SESSION_START.md` Slice 12 execution constraints and README closure rule.
- Updated `STATUS.md` next actions and recorded refinement outcomes.
- Refinement pass 1 improvements applied.
- Refinement pass 2 improvements applied: corrected tracking-state consistency in `STATUS.md` (`Current ticket` reset to `none` after ticket closure).
- Refinement pass 3: no additional improvements identified.
- Refinement pass 4: no additional improvements identified (second consecutive no-change pass).

## Blocker (if any)
- blocker: none.
