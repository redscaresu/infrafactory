# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: S9-T10
- title: Expand `internal/scenario.Scenario` model to include criteria/layer-routing fields
- status: in_progress
- classification: implementation-only

## 1) Problem Statement
- What is broken or missing?
  The runtime scenario model currently exposes only limited fields, which blocks criteria and layer-routing orchestration work in Slice 9.
- Why does it matter now?
  `S9-T2+` depends on typed access to criteria and routing metadata from loaded scenarios.

## 2) Scope
- In scope:
  typed scenario model expansion and loader tests proving criteria/layer fields are available.
- Out of scope:
  criteria execution semantics, support/defer matrix behavior, and sandbox/live deploy wiring.

## 3) Acceptance Criteria
1. `internal/scenario.Scenario` includes typed fields needed by orchestration (`type`, `references`, constraints, acceptance criteria).
2. Scenario loading preserves existing valid fixture compatibility while populating typed fields.
3. Focused tests assert typed decode behavior and backward compatibility.

## 4) Impacted Areas
- Packages/files expected to change:
  `internal/scenario/scenario.go` and `internal/scenario/scenario_test.go`.
- External contracts affected (CLI/schema/policy):
  no.

## 5) Test Plan
- Unit tests:
  scenario loader typed-model tests in `internal/scenario`.
- Integration checks:
  none.
- Manual verification:
  `go test ./...` and `bash scripts/check_all.sh`.

## 6) Risks and Rollback
- Primary risks:
  breaking existing scenario decode paths or introducing nil/zero-value ambiguity for optional fields.
- Rollback approach:
  revert scenario model additions and restore prior loader/tests.

## 7) Done Definition
- Code and focused tests completed for typed scenario model expansion.
- `go test ./...` and `bash scripts/check_all.sh` pass.
- `STATUS.md`, `BACKLOG.md`, and `CURRENT_TICKET.md` synchronized.

## Slice 9 Defaults (Fresh Context)
- Focus:
  close remaining orchestration gaps for criteria-complete command behavior.
- Test policy:
  hermetic tests remain default; real-tool smoke remains opt-in.
- Sandbox policy:
  live/sandbox deploy remains deferred and blocked due cost implications until explicit approval.

## Test plan
- `go test ./...`
- `bash scripts/check_all.sh`

## Progress notes
- Completed slices 1 through 6 internal primitives and focused tests.
- Created and optimized Slice 7 orchestration backlog with shared runtime ticketing, reduced serial deps, and explicit output-contract freeze (`S7-T16`).
- Completed `S7-T1`: `init` now writes a schema-valid scaffold with hints and deterministic next-step output; added focused CLI tests.
- Completed `S7-T2`: added shared CLI runtime/context builder, dependency injection points, and standardized CLI error formatting with focused tests.
- Completed `S7-T12`: froze CLI argument/flag/exit-code contract and added ADR-0002 plus focused contract tests.
- Completed `S7-T16`: added deterministic CLI output contract helpers for human summaries and machine JSON schema output (with stable ordering).
- Completed `S7-T13`: added shared CLI command test harness utility with deterministic fixture setup/output capture and adopted it in command contract tests.
- Completed `S7-T3`: wired `generate` through runtime + scenario + generator pipeline with deterministic file writes and output-mode rendering.
- Completed `S7-T4`: wired `validate` to static harness + OPA plan policy evaluation with deterministic success/failure output mapping.
- Completed `S7-T5`: wired `test` to mock deploy + destroy flow with deterministic success/failure output mapping.
- Completed `S7-T6`: added early smoke suites for `generate`/`validate`/`test` covering representative success/failure command paths.
- Completed `S7-T7`: wired `run` single-iteration skeleton across generate/validate/test with deterministic stage aggregation.
- Completed `S7-T8`: integrated `run` max-iteration and stuck-detection convergence controls with focused stop-condition tests.
- Completed `S7-T9`: integrated `run` persistence/reporting with runstore metadata + per-iteration artifacts and focused persistence tests.
- Completed `S7-T10`: wired `mock start` runtime start path with preflight checks and deterministic success/failure outputs.
- Completed `S7-T11`: added hermetic CLI orchestration integration tests + regression fixture coverage across wired commands.
- Completed `S7-T15`: synchronized README usage/docs with wired command signatures, flags, output modes, and run artifact behavior.
- Completed `S7-T14`: added env-guarded real-tool smoke tests for `validate` (tofu) and optional `test` (tofu + external Mockway URL).
- Slice 7 backlog complete.
- Completed `S8-T1`: added root `Makefile` for dependency lifecycle + test automation and updated `docker-compose.yml` dependency restart behavior.
- Completed `S8-T2`: added make-based opt-in smoke wrappers with Mockway readiness check and configurable URL.
- Completed `S8-T3`: synced README developer workflow to canonical `make` commands.
- Slice 8 backlog complete.
- Completed `S9-T1`: default runtime now injects a concrete `SeedGenerator`; `generate`/`run` no longer fail on missing generator dependency by default; added focused runtime/generator/command tests.
- Planned Slice 9 (`S9-T1`..`S9-T9`) and started `S9-T10`.
- Refined Slice 9 with two additional prerequisite tickets: `S9-T10` (typed scenario model expansion) and `S9-T11` (criteria support/deferment behavior contract).
- Updated `SESSION_START.md` with Slice 9 startup guardrails so fresh contexts see the exact execution order, blocked sandbox scope, and criteria deferment matrix before implementation.
- Expanded `SESSION_START.md` with runtime caveats (`127.0.0.1` Mockway), real-tool smoke preconditions, canonical scenario path, and blocker-output protocol for execution-prompt compliance.

## Blocker (if any)
- blocker: none
- attempts: n/a
- required input: none
