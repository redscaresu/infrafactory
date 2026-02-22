# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: S6-T3
- title: Implement criteria-only holdout flow
- status: done

## Goal
- Discover and execute criteria-only holdouts while blocking feedback injection.

## Scope
- In scope: criteria-only holdout discovery and feedback-blocking execution contract.
- Out of scope: full CLI orchestration.

## Acceptance criteria
1. Criteria-only holdouts are auto-discovered by `references` against training scenario.
2. Criteria-only holdout failures are surfaced without feedback payload exposure.
3. Focused tests cover discovery and feedback-blocking behavior.

## Test plan
- `go test ./...`
- `bash scripts/check_all.sh`

## Progress notes
- Completed Slice 1 (`S1-T1` through `S1-T4`):
- Added `internal/config` loader with defaults and typed validation errors.
- Added `internal/scenario` loader with JSON Schema validation and typed path-aware errors.
- Added focused table-driven tests covering success/failure path isolation across config+scenario chain.
- Completed `S2-T1`:
- Added `internal/generator` contracts and typed error wrappers.
- Added focused generator contract tests with passing required checks.
- Completed `S2-T2`:
- Added prompt rendering helpers and feedback injection tests.
- Completed `S2-T3`:
- Added parser with fence stripping and duplicate resolution.
- Added parser tests for multi-path success/failure coverage.
- Completed `S2-T4`:
- Added fixture-based malformed parser-output coverage.
- Completed Slice 2 milestone with passing required checks.
- Completed `S3-T1`:
- Added static tofu workflow and fake-runner tests.
- Completed `S3-T2`:
- Added OPA plan-policy evaluator and structured feedback failures.
- Added OPA pass/fail fixture tests.
- Completed `S3-T3`:
- Added structured static failure conversion and focused shape tests.
- Completed `S4-T1`:
- Added deploy orchestration with reset/apply/state snapshot and typed failures.
- Completed `S4-T2`:
- Added topology and state-policy evaluators with focused pass/fail tests.
- Completed `S4-T3`:
- Added opt-in layer-2 integration smoke test and finalized unit coverage.
- Completed `S5-T1`:
- Added destroy flow with orphan verification and focused tests.
- Completed `S5-T2`:
- Added filesystem run-store and focused persistence tests.
- Completed `S5-T3`:
- Added destroy/run-store persistence integration tests.
- Completed `S6-T1`:
- Added max-iteration feedback loop with persistence tests.
- Completed `S6-T2`:
- Added subset-based stuck detection and focused tests.
- Completed `S6-T3`:
- Added criteria-only holdout discovery for matching training references.
- Added holdout feedback-blocking behavior and focused tests.
- All current `BACKLOG.md` tickets are now complete.

## Blocker (if any)
- blocker: none
- attempts: n/a
- required input: none
