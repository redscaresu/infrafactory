# BACKLOG

Single source of ticket status across slices.

Legend: `todo` | `in_progress` | `blocked` | `done`

| id | slice | title | priority | status | deps | owner |
|---|---|---|---|---|---|---|
| S1-T1 | Slice 1 | Wire Cobra root and commands (`init`, `generate`, `validate`, `test`, `run`, `mock start`) | P0 | done | - | codex |
| S1-T2 | Slice 1 | Implement `internal/config` loader with defaults and required-field validation | P0 | done | S1-T1 | codex |
| S1-T3 | Slice 1 | Implement `internal/scenario` loader + JSON Schema validation | P0 | done | S1-T1 | codex |
| S1-T4 | Slice 1 | Add Slice 1 focused tests (config/scenario valid+invalid paths) | P0 | done | S1-T2,S1-T3 | codex |
| S2-T1 | Slice 2 | Define `SeedGenerator` interface and generator contracts | P1 | done | S1-T4 | codex |
| S2-T2 | Slice 2 | Implement prompt rendering helpers and feedback context injection | P1 | done | S2-T1 | codex |
| S2-T3 | Slice 2 | Implement `# File:` parser with code-fence stripping + duplicate handling | P1 | done | S2-T1 | codex |
| S2-T4 | Slice 2 | Add generator/parser focused tests | P1 | done | S2-T2,S2-T3 | codex |
| S3-T1 | Slice 3 | Implement static harness (`tofu init/validate/plan/show -json`) | P2 | done | S2-T4 | codex |
| S3-T2 | Slice 3 | Integrate OPA evaluation against plan JSON | P2 | done | S3-T1 | codex |
| S3-T3 | Slice 3 | Add structured static-layer failure reporting + tests | P2 | done | S3-T1,S3-T2 | codex |
| S4-T1 | Slice 4 | Implement mock deploy orchestration (`tofu apply`, mock reset/state client) | P3 | done | S3-T3 | codex |
| S4-T2 | Slice 4 | Add topology checks and state policy checks in harness | P3 | done | S4-T1 | codex |
| S4-T3 | Slice 4 | Add mock deploy layer tests (opt-in where external deps required) | P3 | done | S4-T2 | codex |
| S5-T1 | Slice 5 | Implement destroy flow + orphan verification | P4 | done | S4-T3 | codex |
| S5-T2 | Slice 5 | Implement run store persistence on disk | P4 | done | S5-T1 | codex |
| S5-T3 | Slice 5 | Add destroy/run-store tests | P4 | done | S5-T2 | codex |
| S6-T1 | Slice 6 | Implement feedback loop + max-iteration control | P5 | done | S5-T3 | codex |
| S6-T2 | Slice 6 | Implement stuck detection using failure-signature subset logic | P5 | done | S6-T1 | codex |
| S6-T3 | Slice 6 | Implement criteria-only holdout flow | P5 | done | S6-T2 | codex |
| S7-T1 | Slice 7 | Wire `init` command scaffold generation + next-step output | P0 | done | S6-T3 | codex |
| S7-T2 | Slice 7 | Add shared CLI runtime/context builder and command error formatter | P0 | done | S7-T1 | codex |
| S7-T12 | Slice 7 | Freeze CLI command contract (args/flags/exit codes/output modes) | P0 | done | S7-T2 | codex |
| S7-T16 | Slice 7 | Freeze CLI output contract (human summary + machine JSON schema) | P0 | done | S7-T12 | codex |
| S7-T13 | Slice 7 | Add shared CLI command test harness utility (workspace/setup/capture helpers) | P1 | done | S7-T12 | codex |
| S7-T3 | Slice 7 | Wire `generate` command to config/scenario/generator pipeline + file writes | P0 | done | S7-T12,S7-T16 | codex |
| S7-T4 | Slice 7 | Wire `validate` command to static layer execution + policy reporting | P0 | done | S7-T12,S7-T16 | codex |
| S7-T5 | Slice 7 | Wire `test` command to mock deploy + destroy verification flow | P0 | done | S7-T12,S7-T16 | codex |
| S7-T6 | Slice 7 | Add early CLI orchestration smoke tests (`generate`/`validate`/`test`) | P0 | done | S7-T3,S7-T4,S7-T5 | codex |
| S7-T7 | Slice 7 | Wire `run` command skeleton (single-iteration orchestration path) | P0 | done | S7-T3,S7-T4,S7-T5 | codex |
| S7-T8 | Slice 7 | Add `run` convergence controls (max-iteration + stuck detection integration) | P0 | done | S7-T7 | codex |
| S7-T9 | Slice 7 | Add `run` persistence/reporting integration with runstore | P0 | done | S7-T8 | codex |
| S7-T10 | Slice 7 | Wire `mock start` command to runtime start path + preflight checks | P1 | done | S7-T12 | codex |
| S7-T11 | Slice 7 | Add hermetic CLI orchestration integration tests and regression fixtures | P0 | done | S7-T6,S7-T7,S7-T8,S7-T9,S7-T10 | codex |
| S7-T14 | Slice 7 | Add opt-in real-tool orchestration smoke tests (`tofu` + optional Mockway) | P1 | done | S7-T11 | codex |
| S7-T15 | Slice 7 | Sync README/usage docs to final CLI behavior and examples | P1 | done | S7-T9,S7-T11 | codex |
| S8-T1 | Slice 8 | Add developer-experience automation (`Makefile` + dependency lifecycle targets) | P0 | done | S7-T15 | codex |
| S8-T2 | Slice 8 | Add DX smoke runner targets for real-tool validation flows | P0 | done | S8-T1 | codex |
| S8-T3 | Slice 8 | Sync README/usage docs for DX workflows (deps/tests/smoke/cleanup) | P1 | done | S8-T2 | codex |
| S9-T1 | Slice 9 | Wire default runtime `SeedGenerator` implementation for `generate`/`run` | P0 | done | S8-T3 | codex |
| S9-T10 | Slice 9 | Expand `internal/scenario.Scenario` model to include criteria/layer-routing fields | P0 | done | S9-T1 | codex |
| S9-T2 | Slice 9 | Parse scenario `acceptance_criteria` into typed executable check specs | P0 | done | S9-T10 | codex |
| S9-T11 | Slice 9 | Define criteria support matrix and deterministic unsupported-criteria behavior | P0 | done | S9-T2 | codex |
| S9-T3 | Slice 9 | Execute criteria-driven topology + state-policy checks in `test` | P0 | done | S9-T11 | codex |
| S9-T4 | Slice 9 | Honor `validation.layers.*.enabled` flags in `validate`/`test`/`run` orchestration | P0 | done | S9-T3 | codex |
| S9-T5 | Slice 9 | Upgrade `run` from skeleton to criteria-aware convergence orchestration | P0 | done | S9-T3,S9-T4 | codex |
| S9-T6 | Slice 9 | Wire criteria-only holdout evaluation into `run` completion path | P1 | done | S9-T5 | codex |
| S9-T7 | Slice 9 | Expand `mock` command lifecycle (`start/stop/status/logs`) with parity tests | P1 | done | S9-T4 | codex |
| S9-T8 | Slice 9 | Sandbox/live deploy layer wiring (real Scaleway) | P2 | blocked | - | codex |
| S9-T9 | Slice 9 | Document sandbox deploy deferment due cost implications | P0 | done | S8-T3 | codex |
| S10-T1 | Slice 10 | Freeze output contract with golden snapshots for all commands/modes | P0 | done | S10-T2,S10-T3,S10-T6 | codex |
| S10-T2 | Slice 10 | Normalize CLI error taxonomy/messages across command paths | P0 | done | S9-T7 | codex |
| S10-T3 | Slice 10 | Version run artifact schema and add backward-compatible readers | P0 | done | S9-T7 | codex |
| S10-T4 | Slice 10 | Add idempotency/retry safety checks for repeated command execution | P1 | done | S10-T1,S10-T2,S10-T3 | codex |
| S10-T5 | Slice 10 | Add performance baseline benchmarks and regression guardrails | P1 | done | S10-T3 | codex |
| S10-T6 | Slice 10 | Add criteria/policy failure explainability summaries | P1 | done | S10-T2,S10-T3 | codex |
| S10-T7 | Slice 10 | Finalize permanent sandbox-block governance docs + ADR | P0 | done | S9-T7,S10-T1 | codex |

## Ticket details

| id | in-scope files/packages | out-of-scope | acceptance criteria | required tests |
|---|---|---|---|---|
| S1-T1 | `cmd/infrafactory`, `internal/cli` command tree wiring only | config/scenario parsing, harness behavior | root command exposes all required Slice 1 commands; leaf commands return explicit `not implemented` errors | command discovery tests; stub error tests |
| S1-T2 | `internal/config` load/validate/defaults; command wiring to call config loader only | scenario schema logic, generator/harness flows | config file loads deterministically; missing required fields return typed validation errors | valid config fixture; invalid/missing required-field fixtures |
| S1-T3 | `internal/scenario` parse + JSON schema compile/validate | generator prompt logic, harness execution | scenario loader validates against `scenario.schema.json`; errors include actionable path/context | valid scenario fixture; invalid schema fixture; malformed YAML fixture |
| S1-T4 | Slice 1 integration tests across config+scenario loaders | Slice 2+ features | successful path covers config+scenario parse chain; failure paths isolate config vs scenario errors | table-driven valid/invalid path tests in `internal/config` and `internal/scenario` |
| S2-T1 | `internal/generator` contracts (`SeedGenerator`, request/response types) | provider-specific prompt text changes | interface returns file map and metadata needed by harness; typed errors defined for generator failures | interface behavior tests; typed error wrapping tests |
| S2-T2 | prompt rendering helpers and feedback context injection in `internal/generator` | parser behavior, harness orchestration | prompt templates render deterministic output with scenario+feedback context | template rendering tests for with/without feedback |
| S2-T3 | `internal/generator` `# File:` parser with code-fence stripping + duplicate resolution | LLM call transport | parser emits deterministic map of files, strips fences, handles duplicates by documented rule | parser fixtures: single file, multi-file, fenced blocks, duplicate markers |
| S2-T4 | focused generator/parser suite | Slice 3 harness flows | generator package has hermetic tests covering render and parse success/failure | package-level table-driven tests with malformed output fixtures |
| S3-T1 | `internal/harness` static workflow (`tofu init/validate/plan/show`) | mock deploy and destroy layers | static layer executes commands in fixed order and captures structured outputs | command runner unit tests using fake exec; failure-path tests per stage |
| S3-T2 | OPA policy evaluation against plan JSON (`internal/harness`, `internal/feedback`) | mock state policy checks | plan JSON is passed to OPA; deny results returned as structured failures | OPA fixture tests for pass/fail policy sets |
| S3-T3 | structured static-layer failure reporting | layer 2+ orchestration | failures include stage, command, stderr/stdout summary and machine-readable fields | error-shape tests for validate/plan/OPA failures |
| S4-T1 | mock deploy orchestration (`tofu apply`, mock reset/state client) | sandbox deploy and destruction | harness resets mock state, applies generated code, captures state snapshot deterministically | mock client tests; apply orchestration tests with fake command runner |
| S4-T2 | topology checks + state policy checks in layer 2 | layer 1 plan checks | connectivity and policy checks evaluate mock state graph correctly | topology evaluator tests; state-policy OPA tests |
| S4-T3 | layer 2 test coverage including opt-in integration tests | Slice 5 destroy/run-store logic | deploy layer test suite runs hermetic unit tests and optional integration path | unit tests with fixtures; integration smoke test guarded by env flag |
| S5-T1 | destroy flow and orphan verification (`internal/harness`) | feedback loop logic | `tofu destroy` runs and post-destroy state confirms no orphans | destroy success/failure tests; orphan-detected test |
| S5-T2 | filesystem run-store implementation in `internal/runstore` | DB/CXDB backends | run metadata and iteration artifacts persist under `.infrafactory/runs/` with deterministic paths | runstore write/read/list tests with temp dirs |
| S5-T3 | destroy/run-store combined tests | Slice 6 convergence logic | destruction results and run-store persistence integrate without hidden side effects | end-to-end unit tests for run record creation on success/failure |
| S6-T1 | feedback loop orchestration + max iteration control in `internal/feedback`/`internal/harness` | holdout-specific logic | loop stops on success or max-iteration threshold; iteration outputs persisted | convergence success test; max-iteration stop test |
| S6-T2 | stuck detection using failure signature subset comparison | holdout execution | loop aborts when failures are unchanged subset according to contract | failure-signature comparator tests with subset/non-subset cases |
| S6-T3 | criteria-only holdout discovery and execution flow | full holdout contract changes | criteria-only holdouts auto-discovered by reference and block without feedback injection | holdout discovery tests; block-without-feedback tests |
| S7-T1 | `internal/cli`, `internal/scenario`, templates/scaffold helpers | generator/harness orchestration | `infrafactory init` writes minimal valid scaffold and prints deterministic next steps | scaffold file content tests; command output tests |
| S7-T2 | `internal/cli` shared runtime/context builder + error formatter + dependency injection points | command-specific orchestration logic | shared command setup loads config/scenario/output context once, provides injectable command dependencies (generator/harness/mock clients), and returns standardized CLI-facing errors | runtime builder tests; dependency-injection tests; error-format tests |
| S7-T12 | `internal/cli` command contract spec/helpers | command implementation internals | command args/flags/exit-code and output-mode contract is explicitly defined and stable before wiring commands | contract tests for parse/exit-code/output mode behavior |
| S7-T16 | `internal/cli` output contract spec/helpers | command orchestration wiring internals | CLI output contract defines stable human summaries and machine JSON schema with deterministic ordering of stages/failures | output-schema tests; golden snapshot tests for human and machine output |
| S7-T13 | `internal/cli` command test helper utilities | command business behavior | shared helper sets up temp workspace/dependencies and captures stdout/stderr deterministically for command tests | helper tests; adoption in at least one command test suite |
| S7-T3 | `internal/cli`, `internal/generator`, output writer helpers | harness layer execution | `infrafactory generate <scenario>` loads config/scenario and writes generated files to output dir | fake generator unit tests; output file write tests |
| S7-T4 | `internal/cli`, `internal/harness` static layer adapters | mock deploy/destroy execution | `infrafactory validate <scenario>` executes static checks and returns structured policy/stage failures | static success/failure command tests |
| S7-T5 | `internal/cli`, `internal/harness` deploy+destroy adapters | feedback loop/orchestration | `infrafactory test <scenario>` executes mock deploy checks and destroy/orphan verification | deploy/destroy command tests with fake clients |
| S7-T6 | `internal/cli` command-level smoke tests for `generate`/`validate`/`test` | full `run` loop and regression matrix | command orchestration smoke coverage catches major wiring regressions early | smoke tests for success and one representative failure path per command |
| S7-T7 | `internal/cli`, `internal/feedback`, `internal/harness` run adapter | convergence controls and persistence | `infrafactory run <scenario>` executes a single-iteration skeleton path with deterministic stage aggregation | run skeleton tests with fake dependencies |
| S7-T8 | `internal/cli`, `internal/feedback` convergence integration | runstore reporting and regression matrix | `run` integrates max-iteration and stuck-detection stop semantics | convergence-control tests for stop-on-success/max/stuck |
| S7-T9 | `internal/cli`, `internal/runstore` run persistence/reporting integration | command preflight/runtime management | `run` persists run metadata and iteration artifacts and prints stable run summary | runstore integration tests for success/failure run outputs |
| S7-T10 | `internal/cli` mock command adapter + process preflight checks | full harness loop | `infrafactory mock start` performs deterministic start/preflight behavior with actionable errors | mock start command tests (success + missing dependency) |
| S7-T11 | cross-package hermetic CLI integration tests + fixtures | external tool/runtime behavior | orchestration command suite has hermetic regression coverage for happy and failure paths | integration tests for `generate/validate/test/run/mock start` flows with fake deps |
| S7-T14 | opt-in orchestration smoke tests using real tools (`tofu`, optional Mockway) | hermetic CI default path | opt-in smoke validates command wiring with real external binaries/services | env-guarded smoke tests for `validate/test/run` critical paths |
| S7-T15 | README/usage documentation synchronization to final CLI behavior | architecture or schema contract changes | user-facing docs and examples reflect actual command behavior/flags/output after wiring completes | doc assertion checklist + command example verification |
| S8-T1 | `Makefile`, `docker-compose.yml` | CLI/runtime orchestration logic | make targets exist for dependency up/down/recreate/clean and hermetic test entrypoints | smoke-free make target dry-runs + command invocation checks |
| S8-T2 | `Makefile`, real-tool smoke wiring | mandatory default hermetic path | make targets run opt-in real-tool smoke with explicit env guards and mockway readiness wait | env-guarded smoke target tests/manual verification |
| S8-T3 | `README.md`, `STATUS.md` | architecture/schema contract changes | docs include canonical DX command set for tests/smoke/dependency lifecycle | doc assertion checklist |
| S9-T1 | `internal/cli/runtime.go`, `internal/generator` | provider prompt/template redesign | runtime provides a non-nil default `SeedGenerator`; `generate`/`run` no longer fail with missing generator dependency when runtime defaults are used | `internal/cli/runtime_test.go` + command-path tests for default runtime behavior |
| S9-T10 | `internal/scenario/scenario.go`, `internal/scenario/*_test.go` | schema file redesign | loaded scenario includes typed fields required by orchestration (`type`, `references`, constraints, acceptance criteria), not just scenario name | scenario loader tests asserting typed decode and backward compatibility of existing valid fixtures |
| S9-T2 | `internal/scenario`, `internal/harness`, `internal/cli` adapters | policy authoring semantics | scenario `acceptance_criteria` entries are mapped deterministically into typed executable check specs (connectivity/http_probe/policy/destruction where supported) | parser/mapping tests for each supported criteria type |
| S9-T11 | `internal/cli`, `internal/harness`, docs | schema semantic changes | unsupported criteria for current layers (notably `dns_resolution` and sandbox-only checks) produce deterministic skip/fail behavior with explicit messaging; behavior is documented | command tests for unsupported criteria behavior in human/json output + doc assertions |
| S9-T3 | `internal/cli/test_command.go`, `internal/harness/topology.go`, `internal/harness/state_policy.go` | run-loop convergence internals | `test` executes topology/state-policy evaluators using criteria-derived inputs and reports failures through output contract | command tests for criteria pass/fail and evaluator invocation coverage |
| S9-T4 | `internal/cli/validate_command.go`, `internal/cli/test_command.go`, `internal/cli/run_command.go` | schema contract changes | CLI orchestration consistently honors `validation.layers.static.enabled`, `validation.layers.mock_deploy.enabled`, and `validation.layers.destruction.enabled`; `sandbox_deploy.enabled` remains explicitly blocked with deterministic surfaced status | tests for enabled/disabled combinations and sandbox-enabled blocked-status output |
| S9-T5 | `internal/cli/run_command.go`, `internal/feedback`, `internal/runstore` | runstore backend redesign | `run` performs criteria-aware iteration decisions (not just stage pass-through) and preserves convergence semantics (success/max/stuck) | run command tests for criteria-driven success/failure, max-iteration stop, and stuck-detection stop |
| S9-T6 | `internal/cli/run_command.go`, `internal/scenario/holdout.go` | holdout schema redesign | criteria-only holdouts auto-run after training convergence and block without feeding holdout failures into generator context | holdout discovery/execution tests for pass/block/no-feedback behavior |
| S9-T7 | `internal/cli/root.go`, `internal/cli/mock_*`, `Makefile` (if needed) | external image publishing strategy | `mock` command supports `start`, `stop`, `status`, and `logs` with deterministic output and errors | command tests for stop/status/logs happy-path + missing-runtime/dependency failures |
| S9-T8 | `internal/harness`, `internal/cli`, docs | mock deploy layer and hermetic defaults | sandbox/live deploy stays permanently blocked and out-of-scope under governance policy (ADR-0003) | n/a (permanently blocked governance path) |
| S9-T9 | `README.md`, `STATUS.md`, `ROADMAP.md` | runtime behavior changes | docs explicitly state sandbox/live deploy deferment due cost implications | doc assertion checklist |
| S10-T1 | `internal/cli/output_contract*`, command tests, fixtures/goldens | command business logic changes | final human/json outputs for all commands are frozen via golden snapshots with deterministic ordering and schema assertions after Slice 10 contract-shaping tickets land | golden tests for `init/generate/validate/test/run/mock *` in human+json modes and deterministic fixture update guardrails |
| S10-T2 | `internal/cli/runtime.go`, command adapters, error helpers | schema/provider behavior changes | CLI errors use consistent codes, operation names, and actionable detail shapes across config/scenario/runtime/dependency failures | table-driven error-shape tests for representative failure classes per command |
| S10-T3 | `internal/runstore`, `internal/cli/run_command.go`, docs | new storage backend | run artifacts include explicit schema version; readers remain backward-compatible for pre-versioned artifacts | runstore read/write compatibility tests and run artifact schema assertions |
| S10-T4 | `internal/cli/*_command.go`, `internal/cli/integration_*` | external orchestration redesign | repeated command execution in same workspace remains deterministic and safe under partial prior failures | idempotency integration tests for repeated `generate/validate/test/run/mock` flows |
| S10-T5 | `internal/*` benchmarks, CI scripts/docs | feature behavior changes | baseline benchmark suite exists for key flows with documented thresholds and regression checks without requiring network/external services | benchmark tests + CI guard script assertions with env-guarded execution in default hermetic path |
| S10-T6 | `internal/cli/output_contract.go`, failure mapping adapters, docs | policy authoring semantics | failure output includes concise explainability summaries linking criteria/policy checks to actionable context | output contract tests for explainability fields in human/json outputs |
| S10-T7 | `docs/decisions/*`, `docs/decisions/README.md`, `README.md`, `ROADMAP.md`, `STATUS.md` | runtime deployment implementation | permanent sandbox/live block governance is codified in ADR and synchronized docs with unambiguous policy language | doc hygiene + ADR index/checklist assertions |

## Operating notes
- Update `status` and dependencies as work evolves.
- Keep exactly one `in_progress` ticket at a time.
- Use `CURRENT_TICKET.md` for session-level execution details.
