# STATUS

Last updated: 2026-02-22

## Current phase
- Active milestone: Slice 7 CLI orchestration
- Next gate: end-to-end command wiring across generate/validate/test/run
- Current ticket: S7-T1
- Next ticket: S7-T2

## In progress
- `init` command scaffold generation + next-step output (`S7-T1`)

## Known blockers
- None currently.

## Next actions
1. Complete `S7-T1` (`init` scaffold + deterministic next-step output).
2. Complete `S7-T2` plus `S7-T12`/`S7-T16`/`S7-T13` (command+output contract freeze + shared test harness) before wiring core commands.
3. Wire `S7-T3`/`S7-T4`/`S7-T5`, run early smoke checks (`S7-T6`), then complete `run` split tickets (`S7-T7`..`S7-T9`), hermetic integration (`S7-T11`), doc sync (`S7-T15`), and optional real-tool smoke (`S7-T14`).

## Update policy
- Update at end of each meaningful coding session.
- Keep concise and factual.
- Move old detail to `docs/status/ARCHIVE.md`.
- Put durable architecture decisions in ADRs and `CONCEPT.md`.
- Keep startup/read-order instructions only in `SESSION_START.md` to avoid duplication.

## Recent updates
- Optimized Slice 7 dependency graph: removed `P1` test-harness ticket from `P0` critical-path smoke/integration dependencies.
- Simplified Slice 7 with transitive-dependency cleanup (removed redundant direct deps where upstream gates already enforce order).
- Optimized Slice 7 execution path: decoupled `mock start` from output-contract gating and decoupled docs sync from optional real-tool smoke.
- Rebalanced Slice 7 priorities to keep critical command wiring as `P0` and supporting utilities/docs as `P1`.
- Optimized Slice 7 again: added explicit CLI output contract ticket (`S7-T16`) to lock human/machine output schema before command wiring.
- Optimized Slice 7 again: added CLI contract-freeze ticket (`S7-T12`), shared command-test harness ticket (`S7-T13`), hermetic vs real-tool integration split (`S7-T11`/`S7-T14`), and final doc-sync gate (`S7-T15`).
- Optimized Slice 7 further: split `run` into skeleton/convergence/persistence tickets and added early orchestration smoke tests.
- Optimized Slice 7 plan: added shared runtime ticket (`S7-T2`) and reduced unnecessary serial dependencies.
- Reordered Slice 7 so `generate`/`validate`/`test` can proceed after shared runtime and converge at `run`.
- Planned Slice 7 orchestration backlog (`S7-T1`..`S7-T16`) for full CLI command wiring.
- Set active work to `S7-T1` as the first unblocked orchestration ticket.
- Completed `S6-T3`: added criteria-only holdout discovery by training-scenario reference and feedback-blocking behavior.
- Completed Slice 6 milestone (`S6-T1` through `S6-T3`) with passing local checks.
- Completed `S6-T2`: added deterministic failure-signature extraction and subset-based stuck detection.
- Added focused tests for subset/equal/non-subset stuck detection behavior.
- Verified local checks: `go test ./...` and `bash scripts/check_all.sh` both pass.
- Completed `S6-T1`: added feedback loop with max-iteration control and per-iteration artifact persistence.
- Added focused tests for early convergence and max-iteration stop behavior.
- Verified local checks: `go test ./...` and `bash scripts/check_all.sh` both pass.
- Completed `S5-T3`: added destroy/run-store integration persistence logic and focused success/failure tests.
- Completed Slice 5 milestone (`S5-T1` through `S5-T3`) with passing local checks.
- Completed `S5-T2`: added filesystem run-store implementation under `.infrafactory/runs/` with deterministic metadata/artifact paths.
- Added focused run-store tests for write/read/list and iteration artifact persistence.
- Verified local checks: `go test ./...` and `bash scripts/check_all.sh` both pass.
- Completed `S5-T1`: added destroy orchestration with post-destroy orphan verification and typed stage errors.
- Added tests for destroy success/failure and orphan detection paths.
- Verified local checks: `go test ./...` and `bash scripts/check_all.sh` both pass.
- Completed `S4-T3`: finalized layer-2 tests and added opt-in integration smoke guard (`INFRAFACTORY_ENABLE_INTEGRATION`).
- Completed Slice 4 milestone (`S4-T1` through `S4-T3`) with passing local checks.
- Completed `S4-T2`: added topology evaluator and OPA `deny_state` evaluation for mock-state checks.
- Added focused tests for topology and state-policy pass/fail coverage.
- Verified local checks: `go test ./...` and `bash scripts/check_all.sh` both pass.
- Completed `S4-T1`: added mock deploy orchestration (`reset -> tofu apply -> state snapshot`) with typed stage errors.
- Added fake client/runner tests covering success and reset/apply/state failure paths.
- Verified local checks: `go test ./...` and `bash scripts/check_all.sh` both pass.
- Completed `S3-T3`: added machine-readable static-layer failure conversion with stage/command/stdout/stderr detail.
- Added focused tests for validate failure, plan/show failure shapes, and OPA failure shape consistency.
- Completed Slice 3 milestone (`S3-T1` through `S3-T3`) with passing local checks.
- Completed `S3-T2`: integrated OPA/Rego evaluation against plan JSON with structured policy failures via `internal/feedback`.
- Added OPA fixture tests for policy pass/fail behavior in the static layer.
- Verified local checks: `go test ./...` and `bash scripts/check_all.sh` both pass.
- Completed `S3-T1`: added `internal/harness` static workflow with deterministic tofu stage order and structured stage results.
- Added fake-runner tests for success and stage-specific failures, including invalid plan JSON.
- Verified local checks: `go test ./...` and `bash scripts/check_all.sh` both pass.
- Completed `S2-T4`: expanded generator/parser tests with fixture-based malformed output coverage.
- Completed Slice 2 milestone (`S2-T1` through `S2-T4`) with passing local checks.
- Completed `S2-T3`: added deterministic `# File:` parser with fence stripping and last-write-wins duplicate handling.
- Added parser tests for single/multi-file, fenced content, duplicate resolution, and malformed output.
- Completed `S2-T2`: added prompt rendering helpers with strict template execution and feedback JSON injection.
- Added focused tests for prompt rendering with and without feedback context.
- Verified local checks: `go test ./...` and `bash scripts/check_all.sh` both pass.
- Completed `S2-T1`: added `internal/generator` contracts (`SeedGenerator`, request/response metadata, typed error wrappers, output validation helpers).
- Added focused generator contract tests covering interface execution, output validation, and `errors.Is`/`errors.As` behavior.
- Verified local checks: `go test ./...` and `bash scripts/check_all.sh` both pass.
- Completed `S1-T4`: added focused table-driven tests covering config+scenario success chain and isolated failure paths.
- Completed Slice 1 milestone (`S1-T1` through `S1-T4`) with passing `go test ./...` and `bash scripts/check_all.sh`.
- Completed `S1-T3`: added `internal/scenario` loader with YAML parsing, JSON Schema validation, and typed path-aware validation errors.
- Added focused scenario fixtures/tests for valid, schema-invalid, and malformed YAML inputs.
- Verified local checks: `go test ./...` and `bash scripts/check_all.sh` both pass.
- Completed `S1-T2`: added `internal/config` loader with defaults, deterministic parsing, and typed validation errors for required fields.
- Wired non-`init` CLI commands to load `--config` before returning stub behavior.
- Added focused config tests and fixtures for valid/defaulted config and missing required-field errors.
- Completed `S1-T1`: wired Cobra root command with `init`, `generate`, `validate`, `test`, `run`, and `mock start`.
- Added `internal/cli` focused tests for command tree discovery and explicit stub error behavior.
- Ran `go mod tidy`; generated `go.sum` and cleared prior dependency-checksum blocker.
- Verified local checks: `go test ./...` and `bash scripts/check_all.sh` both pass.
- Added reusable prompt at `docs/process/EXECUTION_PROMPT.md`.
- Linked reusable prompt from `README.md` and `SESSION_START.md`.
- Added README kickoff instruction for fresh sessions.
- Added `BACKLOG.md` as single ticket status source.
- Added `CURRENT_TICKET.md` as session execution stub.
- Added `scripts/check_all.sh` to run tests + doc hygiene in one command.
- Updated startup/contributor flow to use backlog + current-ticket files.
- Set `BACKLOG.md` active work state: `S1-T1` is `in_progress` (single active ticket).
- Added Apache-2.0 `LICENSE` and linked license section in `README.md`.
