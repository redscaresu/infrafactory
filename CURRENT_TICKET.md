# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: M34
- title: End-to-end pipeline stabilization and self-review contract tightening
- status: done
- classification: implementation + docs

## 1) Problem Statement
- What is broken or missing?
  1. Mockway missing Block Storage API routes (`/block/v1alpha1/`) caused `tofu destroy` failures.
  2. Mockway missing RDB ACL DELETE endpoint caused destroy-phase 501 errors.
  3. Stale Docker container ran old code without server terminate fix.
  4. `SelfReviewIndicatesNoChanges` used overly broad substring matching, risking false "no changes" classification.
  5. Prompt pitfall for `private_ips` referenced wrong resource type.
  6. Tests codified weakened self-review parse behavior.
- Why does it matter now?
  These issues caused intermittent destroy/orphan failures and could suppress legitimate self-review corrections.

## 2) Scope
- In scope:
  Mockway endpoint fixes (block volumes, RDB ACL DELETE), prompt pitfall updates, self-review contract tightening, adapter test alignment, doc sync.
- Out of scope:
  New features, schema/CLI contract changes, transport redesign.

## 3) Acceptance Criteria
1. `tofu destroy` completes cleanly (no 501 errors from missing mockway endpoints).
2. `SelfReviewIndicatesNoChanges` matches only exact canonical `NO ISSUES FOUND`.
3. Adapter tests reflect strict self-review contract.
4. Pipeline passes 5/5 first-iteration runs consistently.
5. All docs synchronized.

## 4) Impacted Areas
- Files changed:
  `internal/generator/parser.go`, `internal/generator/parser_test.go`,
  `internal/generator/claude_adapter_test.go`,
  `prompts/phase2_generate_hcl.md`,
  `README.md`, `SESSION_START.md`, `ROADMAP.md`, `BACKLOG.md`, `STATUS.md`, `CURRENT_TICKET.md`.
- Mockway files changed (separate repo):
  `handlers/handlers.go` (block volume routes, RDB ACL DELETE route),
  `handlers/rdb.go` (DeleteRDBACLs handler),
  `handlers/handlers_test.go` (TestBlockVolumeEndpoints).
- External contracts affected (CLI/schema/policy): no.

## 5) Test Plan
- `go test ./internal/generator/` — all pass
- 6 consecutive `run` executions — all first-iteration passes
- `bash scripts/check_all.sh`

## 6) Risks and Rollback
- Primary risks: None; tighter self-review check is a strict subset of prior behavior.
- Rollback approach: Revert parser/test edits.

## 7) Done Definition
- Pipeline 5/5+ first-iteration pass rate.
- Self-review contract strict and tested.
- Docs synchronized.
- Local checks pass.

## Progress notes
- Fixed mockway: added block volume API routes, RDB ACL DELETE handler, server terminate deployment.
- Updated `private_ips` prompt pitfall to reference NIC resource.
- Tightened `SelfReviewIndicatesNoChanges` to exact canonical phrase only.
- Updated adapter tests: renamed and aligned to strict/fallback semantics.
- Pipeline results: 6/6 first-iteration passes (including post-tightening).
- Review pass 1: docs sync applied (STATUS, ROADMAP, BACKLOG, README, SESSION_START, CURRENT_TICKET).

## Blocker (if any)
- blocker: none.
