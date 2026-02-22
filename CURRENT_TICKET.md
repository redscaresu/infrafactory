# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: M1
- title: Harden runtime/policy regressions captured in `ISSUES.md` + add regression tests
- status: done
- classification: implementation-only

## 1) Problem Statement
- What is broken or missing?
  `ISSUES.md` captured concrete runtime and policy mismatches causing false passes/failures and fragile stage reporting.
- Why does it matter now?
  These regressions reduce trust in validation/test outcomes and can silently mask policy violations.

## 2) Scope
- In scope:
  Fix OPA plan/state input wiring gaps, policy path resolution, mock env parity, Mockway timeout, and policy rule mismatches; add regression tests and ignore local issues scratch file.
- Out of scope:
  architectural redesign items from `ISSUES.md` that are explicitly low-priority design choices.

## 3) Acceptance Criteria
1. Critical runtime/policy mismatches from `ISSUES.md` are fixed.
2. Regression tests are added for each fixed behavior.
3. `ISSUES.md` is ignored in git.

## 4) Impacted Areas
- Packages/files changed:
  `internal/harness/opa.go`,
  `internal/harness/state_policy.go`,
  `internal/cli/validate_command.go`,
  `internal/cli/test_command.go`,
  `internal/cli/mockway_client.go`,
  `policies/scaleway/no_public_database.rego`,
  `policies/scaleway/no_public_endpoints.rego`,
  `policies/scaleway/vpc_required.rego`,
  `internal/harness/opa_test.go`,
  `internal/harness/state_policy_test.go`,
  `internal/cli/validate_command_test.go`,
  `internal/cli/test_command_test.go`,
  `internal/cli/mockway_client_test.go`,
  `.gitignore`,
  `STATUS.md`,
  `BACKLOG.md`,
  `CURRENT_TICKET.md`.
- External contracts affected (CLI/schema/policy):
  yes (policy evaluation wiring and policy behavior).

## 5) Test Plan
- Unit tests:
  `go test ./...`
- Integration checks:
  `bash scripts/check_all.sh`
- Manual verification:
  n/a

## 6) Risks and Rollback
- Primary risks:
  policy behavior drift if provider plan schema evolves.
- Rollback approach:
  revert this maintenance patch set and re-run policy tests.

## 7) Done Definition
- Runtime/policy regressions fixed with tests.
- Required docs updated (`STATUS.md`, `BACKLOG.md`, `CURRENT_TICKET.md`).
- Remaining follow-up captured explicitly.

## Test plan
- `go test ./...`
- `bash scripts/check_all.sh`

## Progress notes
- Added `EvaluatePlanPoliciesWithConstraints` and wired validate path to pass scenario constraints into OPA.
- Added `EvaluateStatePoliciesWithInput`, policy target propagation, and robust constraint-policy path resolution.
- Fixed `validate` static env parity with test command (`SCW_ACCESS_KEY`, `SCW_SECRET_KEY`, `SCW_DEFAULT_PROJECT_ID`).
- Hardened stage appenders to avoid conflicting pass/fail duplicates when future harness contracts evolve.
- Set Mockway state client timeout to `30s`.
- Fixed Scaleway policy mismatches:
  - `no_public_database`: top-level `input.rdb` for deployed state.
  - `no_public_endpoints`: support `server` attribute (plus `server_id` fallback).
  - `vpc_required`: detect server linkage via configuration `server_id.references`.
- Added focused regression tests in `internal/harness` and `internal/cli` for all above behavior.
- Added `ISSUES.md`/`issues.md` to `.gitignore`.
- Fixed criteria-only holdout execution path to reuse the converged training scenario output directory (`runCriteriaOnlyHoldouts`), with regression coverage to ensure holdout mock deploy runs against training output.
- Removed unused `internal/feedback` loop implementation/tests (`RunLoop`) to eliminate dead code drift.
- Simplified `runIteration` step model to remove the unused `runner` field for the `test` step while preserving current behavior.
- Reworked orphan counting to tolerate new resource collections without code changes and added regression coverage for unknown collection keys.

## Blocker (if any)
- blocker: none.
