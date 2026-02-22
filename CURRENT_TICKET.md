# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: S10-T7
- title: Finalize permanent sandbox-block governance docs + ADR
- status: done
- classification: decision-impacting

## 1) Problem Statement
- What is broken or missing?
  Sandbox/live deploy governance required explicit durable codification as permanent policy.
- Why does it matter now?
  This removes ambiguity from backlog/docs and closes Slice 10 governance hardening.

## 2) Scope
- In scope:
  ADR + docs synchronization for permanent sandbox/live block policy.
- Out of scope:
  sandbox/live deploy implementation.

## 3) Acceptance Criteria
1. ADR records permanent sandbox/live deploy block decision and rationale.
2. Decision index references the ADR.
3. Governance docs consistently state permanent block policy and non-goals.

## 4) Impacted Areas
- Packages/files changed:
  `docs/decisions/0003-permanent-sandbox-live-deploy-block.md`,
  `docs/decisions/README.md`,
  `README.md`,
  `ROADMAP.md`,
  `STATUS.md`,
  `SESSION_START.md`,
  `BACKLOG.md`.
- External contracts affected (CLI/schema/policy):
  yes (durable workflow governance).

## 5) Test Plan
- Unit tests:
  n/a (docs/ADR ticket).
- Integration checks:
  `go test ./...`
- Manual verification:
  `bash scripts/check_all.sh`

## 6) Risks and Rollback
- Primary risks:
  inconsistent governance language across files.
- Rollback approach:
  revert ADR/doc synchronization changes.

## 7) Done Definition
- ADR and docs synchronized.
- Required docs updated (`STATUS.md`, `BACKLOG.md`, `CURRENT_TICKET.md`).
- Remaining follow-up captured explicitly.

## Test plan
- `go test ./...`
- `bash scripts/check_all.sh`

## Progress notes
- Completed `S10-T1`: command/output golden snapshots for all command paths/modes.
- Completed `S10-T2`: normalized CLI error taxonomy/messages.
- Completed `S10-T3`: versioned run artifacts + backward-compatible run metadata readers.
- Completed `S10-T4`: idempotency/retry safety checks for repeated command execution.
- Completed `S10-T5`: benchmark baselines + env-gated benchmark regression guard script/target.
- Completed `S10-T6`: criteria/policy explainability summaries in output contract.
- Completed `S10-T7`: accepted ADR-0003 and synchronized docs to codify permanent sandbox/live deploy block governance.
- User policy input applied: `S9-T8` is permanently blocked.

## Blocker (if any)
- blocker: none.
