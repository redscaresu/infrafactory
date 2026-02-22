# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: M15
- title: Auto-inject missing Scaleway provider wiring during generate
- status: done
- classification: implementation-only

## 1) Problem Statement
- What is broken or missing?
  Claude generation could emit Scaleway resources without provider wiring, and fail-fast blocking in `generate` prevented progressing through the happy path.
- Why does it matter now?
  Users need deterministic successful bundles from `generate` even when model output omits boilerplate provider blocks.

## 2) Scope
- In scope:
  Auto-inject missing Scaleway provider wiring in generated files and keep validation as a postcondition.
- Out of scope:
  Broader Terraform semantic repair.

## 3) Acceptance Criteria
1. `generate` adds missing `required_providers.scaleway` when Scaleway resources are present.
2. `generate` adds missing `provider "scaleway"` block when Scaleway resources are present.
3. Regression test verifies successful generation with injected `providers.tf`.

## 4) Impacted Areas
- Packages/files changed:
  `internal/cli/generate_command.go`,
  `internal/cli/generate_command_test.go`,
  `README.md`,
  `BACKLOG.md`,
  `STATUS.md`,
  `CURRENT_TICKET.md`.
- External contracts affected (CLI/schema/policy):
  no.

## 5) Test Plan
- Unit tests:
  `go test ./internal/cli`
- Integration checks:
  `bash scripts/check_all.sh`
- Manual verification:
  run `generate` on a scenario that emits Scaleway resources and confirm `providers.tf` is written with required provider wiring.

## 6) Risks and Rollback
- Primary risks:
  injected provider defaults may need tuning if provider requirements change.
- Rollback approach:
  remove the auto-injection helper and restore strict fail-fast behavior.

## 7) Done Definition
- Generate-time provider-wiring auto-injection implemented and tested.
- Tracking docs updated.

## Test plan
- `go test ./internal/cli`
- `bash scripts/check_all.sh`

## Progress notes
- Added `ensureScalewayProviderWiring(...)` in generate command path before validation.
- Injection writes missing Scaleway `required_providers` and/or `provider "scaleway"` blocks into `providers.tf`.
- Added `TestGenerateCommandAutoAddsScalewayProviderWiringWhenMissing`.

## Blocker (if any)
- blocker: none.
