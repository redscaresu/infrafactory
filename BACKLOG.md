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

## Operating notes
- Update `status` and dependencies as work evolves.
- Keep exactly one `in_progress` ticket at a time.
- Use `CURRENT_TICKET.md` for session-level execution details.
