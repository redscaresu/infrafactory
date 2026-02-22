# InfraFactory

Scenario-driven infrastructure generation and validation for Scaleway with OpenTofu.

## Overview

InfraFactory follows a software-factory model for infrastructure:
1. Parse and validate a scenario contract.
2. Generate OpenTofu files.
3. Run deterministic static and deploy-layer validation.
4. Persist run artifacts and iterate with structured feedback.

## Current State

Core internal slices are implemented and tested:
- Config loading/validation (`internal/config`)
- Scenario loading + JSON Schema validation (`internal/scenario`)
- Generator contracts, prompt rendering, and `# File:` parsing (`internal/generator`)
- Static + mock deploy + destroy harness primitives (`internal/harness`)
- Feedback loop + stuck detection helpers (`internal/feedback`)
- Filesystem run store (`internal/runstore`)

CLI command wiring exists (`init`, `generate`, `validate`, `test`, `run`, `mock start`), but end-to-end command orchestration is still being integrated.

## Repository Layout

- `cmd/infrafactory/`: CLI entrypoint
- `internal/cli`: command tree and command-level wiring
- `internal/config`: runtime config model and loader (`infrafactory.yaml`)
- `internal/scenario`: scenario parsing and schema validation
- `internal/generator`: generator contracts, prompt rendering, output parser
- `internal/harness`: static/deploy/destroy orchestration primitives
- `internal/feedback`: failure models, loop control, stuck detection
- `internal/runstore`: `.infrafactory/runs` persistence implementation
- `scenario.schema.json`: scenario contract
- `infrafactory.yaml`: runtime config contract
- `policies/`: OPA policy files
- `scenarios/`: training/holdout/regression fixtures

## Requirements

- Go `1.24+`
- OpenTofu (`tofu`) for harness execution flows
- Optional: Mockway for deploy-layer integration paths

## Quick Start

```bash
go mod tidy
go test ./...
go run ./cmd/infrafactory --help
```

## Local Quality Checks

Run the standard local hygiene check before handoff/PR:

```bash
bash scripts/check_all.sh
```

## Documentation Index

- Usage guide: `docs/USAGE.md`
- Architecture: `docs/architecture.md`
- Full concept log: `CONCEPT.md`
- Decisions (ADRs): `docs/decisions/`
- Contributor flow: `CONTRIBUTING.md`
- Agent workflow: `AGENTS.md`
- Session bootstrap: `SESSION_START.md`
- Ticket backlog: `BACKLOG.md`
- Current execution stub: `CURRENT_TICKET.md`
- Rolling status: `STATUS.md`
- Execution prompt: `docs/process/EXECUTION_PROMPT.md`

## Agent Kickoff

For autonomous ticket execution in a fresh agent session, use:

```text
Use docs/process/EXECUTION_PROMPT.md exactly. Start now.
```

## License

Apache License 2.0. See `LICENSE`.
