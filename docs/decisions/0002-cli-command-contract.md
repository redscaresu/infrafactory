# ADR-0002: CLI Command Contract Freeze for Slice 7

## Status
Accepted

## Context

Slice 7 wires end-to-end command orchestration across `generate`, `validate`, `test`, and `run`.
Before that wiring, the public CLI contract must be explicit and stable so implementation work and tests target one behavior.

Without an explicit contract, command argument shape, output-mode behavior, and exit-code semantics can drift across tickets and produce inconsistent UX and fragile tests.

## Decision

Freeze the following command contract in `internal/cli`:

- `generate`, `validate`, `test`, and `run` require exactly one positional scenario path argument.
- A global `--output` flag is defined with allowed values `human` and `json`.
- Invalid usage (argument mismatch or invalid output mode) is normalized to a CLI usage error classification.
- Exit codes are standardized:
  - `0` for success
  - `1` for runtime/command failures
  - `2` for usage errors
- Command-level wrappers preserve this classification so contract tests can assert deterministic parse and exit behavior.
- Output contract is standardized before orchestration wiring:
  - human summary shape (`command`, `scenario`, `status`, then ordered `stages` and `failures`)
  - machine payload envelope with schema/version identifier (`infrafactory.output.v1`)
  - deterministic sort order for stages and failures
  - empty collections serialized as arrays (`[]`), not `null`

## Consequences

- Later wiring tickets (`S7-T3`+ ) can focus on orchestration internals without re-defining CLI surface area.
- Tests can reliably distinguish usage failures from runtime failures.
- Additional flags and output schema details must extend this contract rather than silently changing it.
- Downstream command wiring can emit stable human and machine outputs without redefining ordering rules per command.

## Amendment (2026-06-02, S67 — `mock reset` subcommand)

Added `infrafactory mock reset` to the existing `mock` subcommand
group. Same contract as the other `mock` subcommands (status / start
/ stop / logs): scenario-independent, takes no positional args, uses
the global `--output` flag, exit code 0 on success and 1 on
command_failed.

The implementation fans out via a new `cloudMockStateRouter.ResetAll`
helper that hits every configured mock backend (mockway + fakegcp +
fakeaws) and cascades to the s3 backend (SeaweedFS by default) via
the existing `resetS3Backend`. This is the systematic fix for the
S54 SeaweedFS state-leak — bare-curl `/mock/reset` to fakeaws does
NOT cascade to SeaweedFS, so prior session buckets bled into sweeps
as `BucketAlreadyExists`. Routing through `mock reset` (or directly
through `cloudMockStateRouter` in `infrafactory run`'s clean-deploy
path) is the right path.

No change to the existing command contract or exit codes — `mock
reset` slots in alongside the other `mock` subcommands.
