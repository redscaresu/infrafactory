# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: S9-T8
- title: Sandbox/live deploy layer wiring (real Scaleway)
- status: blocked
- classification: decision-impacting

## 1) Problem Statement
- What is broken or missing?
  Sandbox/live deploy layer wiring remains intentionally deferred and blocked due cost/credentials policy constraints.
- Why does it matter now?
  This is the final Slice 9 ticket, but it cannot proceed without explicit approval on cost/credentials governance.

## 2) Scope
- In scope:
  real Scaleway sandbox/live deploy orchestration model (`S9-T8`) once policy is approved.
- Out of scope:
  any implementation while approval remains absent.

## 3) Acceptance Criteria
1. Explicit cost/credentials policy approval is provided.
2. ADR/contract updates for sandbox deploy governance are accepted.
3. Implementation and tests proceed only after (1) and (2).

## 4) Impacted Areas
- Packages/files expected to change:
  blocked pending approval.
- External contracts affected (CLI/schema/policy):
  yes (deploy governance and runtime behavior).

## 5) Test Plan
- Unit tests:
  blocked pending approval.
- Integration checks:
  blocked pending approval.
- Manual verification:
  blocked pending approval.

## 6) Risks and Rollback
- Primary risks:
  cost leakage, credential handling mistakes, and policy non-compliance.
- Rollback approach:
  n/a while blocked.

## 7) Done Definition
- Blocker state documented and synchronized.
- Implementation deferred until approval.

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
- Completed `S9-T7`: expanded mock lifecycle command parity with focused tests for `start`/`stop`/`status`/`logs`.
- Updated blocked `S9-T8` stub behavior to emit explicit output/log messaging: `(real deployment skipped for cost reasons for now)` for sandbox-deploy blocked paths and deferred criteria signaling.
- Completed `S9-T6`: wired holdout completion checks into run with deterministic block behavior and explicit no-feedback-loop coupling.
- Completed `S9-T5`: upgraded run convergence to use criteria-level failure propagation from test execution while preserving deterministic stop semantics.
- Completed `S9-T4`: wired layer-enable orchestration behavior and blocked sandbox signaling for `validate`/`test`/`run`.
- Completed `S9-T3`: wired criteria-driven topology/state-policy execution in `test` with deterministic pass/fail output behavior.
- Completed `S9-T11`: added deterministic unsupported-criteria behavior contract with explicit support-matrix skip/failure output for deferred checks such as `dns_resolution`.
- Completed `S9-T2`: added typed executable criteria mapping and focused parse-error coverage for malformed criteria specs.
- Completed `S9-T10`: expanded typed scenario model decode for criteria/layer routing fields and added focused backward-compatibility + field-mapping loader tests.
- Completed `S9-T1`: default runtime now injects a concrete `SeedGenerator`; `generate`/`run` no longer fail on missing generator dependency by default; added focused runtime/generator/command tests.
- Planned Slice 9 (`S9-T1`..`S9-T9`) and halted at `S9-T8` due explicit policy blocker.
- Refined Slice 9 with two additional prerequisite tickets: `S9-T10` (typed scenario model expansion) and `S9-T11` (criteria support/deferment behavior contract).
- Updated `SESSION_START.md` with Slice 9 startup guardrails so fresh contexts see the exact execution order, blocked sandbox scope, and criteria deferment matrix before implementation.
- Expanded `SESSION_START.md` with runtime caveats (`127.0.0.1` Mockway), real-tool smoke preconditions, canonical scenario path, and blocker-output protocol for execution-prompt compliance.
- Drafted and refined Slice 10 backlog tickets (`S10-T1`..`S10-T7`) for reliability/contract hardening; resolved dependency sequencing gaps and completed two consecutive no-change refinement passes.
- Documented fresh-context startup requirements in `SESSION_START.md`, including current blocked/unblocked lanes, startup verification commands, Slice 10 order constraints, and corrected runtime-state notes for criteria-aware `run`.
- Completed refinement loop for fresh-context docs with two consecutive no-change passes and no further suggested additions.

## Blocker (if any)
- blocker: `S9-T8` sandbox/live deploy wiring remains blocked by explicit cost/credentials policy requirement.
- attempts: all unblocked Slice 9 tickets (`S9-T1`, `S9-T10`, `S9-T2`, `S9-T11`, `S9-T3`, `S9-T4`, `S9-T5`, `S9-T6`, `S9-T7`) completed with passing checks; blocked-path stub messaging has been wired and verified.
- required input: explicit approval and governance direction for real (non-stub) sandbox/live deploy implementation scope.
