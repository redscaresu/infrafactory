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
