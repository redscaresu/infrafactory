# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: M22
- title: Refine unfinished slices (`S12`..`S14`) for higher-signal model-guided repair and convergence clarity
- status: done
- classification: decision-impacting

## 1) Problem Statement
- What is broken or missing?
  Unfinished slice definitions still left ambiguity around retry-signal quality (especially coarse validate failures), failure-type classification, and terminal stop signaling semantics.
- Why does it matter now?
  These slices drive upcoming implementation work; unclear contracts risk low-signal feedback to the model and noisy/duplicative run-stop behavior.

## 2) Scope
- In scope:
  Refine all unfinished slice definitions (`S12`..`S14`) to tighten acceptance criteria, sequencing, and tests for high-fidelity retry feedback.
  Record iterative refinement outcomes until two consecutive no-change passes are reached.
  Add fresh-context protocol note for future planning refinements.
- Out of scope:
  Runtime code implementation of these slice behaviors.

## 3) Acceptance Criteria
1. Unfinished slice entries in `BACKLOG.md` are refined with stronger contracts where needed (failure-class tagging, terminal-stop de-dup semantics, and CLI precedence/warning clarity).
2. `ROADMAP.md`, `STATUS.md`, and `SESSION_START.md` reflect the refined direction and fresh-context continuity requirements.
3. Refinement log records one improvement pass and two consecutive no-change passes.

## 4) Impacted Areas
- Packages/files changed:
  `BACKLOG.md`,
  `ROADMAP.md`,
  `STATUS.md`,
  `SESSION_START.md`,
  `CURRENT_TICKET.md`.
- External contracts affected (CLI/schema/policy):
  yes (planned run feedback, logging observability semantics, and iteration-contract migration behavior; implementation pending).

## 5) Test Plan
- Unit tests:
  n/a (planning/docs-only change)
- Integration checks:
  `bash scripts/check_all.sh`
- Manual verification:
  review unfinished-slice entries and ensure refinement protocol is explicitly documented for fresh contexts.

## 6) Risks and Rollback
- Primary risks:
  overlap between Slice 13 logging semantics and Slice 14 feedback semantics if boundaries are not explicit.
- Rollback approach:
  revert planning/doc refinements and restore prior unfinished-slice definitions.

## 7) Done Definition
- Unfinished-slice planning definitions are refined and documented.
- Two consecutive no-change refinement passes recorded.
- Fresh-context note added for future refinement loops.

## Test plan
- `bash scripts/check_all.sh`

## Progress notes
- Updated `BACKLOG.md` unfinished slice entries (`S12`..`S14`) with stricter acceptance criteria around:
  - failure-class tagging (`iac_validation`, `transport_runtime`, `orchestration_control`),
  - deterministic non-duplicative terminal stop signaling,
  - explicit `--iterations`/`--max-iterations` precedence and warning semantics.
- Updated `ROADMAP.md` Slice 12/13/14 milestone text to include non-duplicative terminal-stop and failure-class clarity.
- Updated `STATUS.md` next-actions and recent-updates log for this refinement cycle.
- Updated `SESSION_START.md` with a global fresh-context rule requiring two consecutive no-change passes for planning refinement over unfinished slices.
- Refinement pass 1 improvements applied: no further structural changes required after contract tightening above.
- Refinement pass 2: no additional improvements identified.
- Refinement pass 3: no additional improvements identified (second consecutive no-change pass).

## Blocker (if any)
- blocker: none.
