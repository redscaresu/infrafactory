# InfraFactory

Scenario-driven infrastructure generation and validation for Scaleway with OpenTofu.

## Overview

InfraFactory follows a software-factory model for infrastructure:
1. Parse and validate scenario and config contracts.
2. Generate OpenTofu files from scenario intent.
3. Run deterministic static and deploy-layer validation.
4. Persist run artifacts and iterate with structured feedback.

## Current State

Core internal slices are implemented and tested:
- Config loading and validation (`internal/config`)
- Scenario parsing and JSON Schema validation (`internal/scenario`)
- Generator contracts, prompt rendering, and `# File:` parsing (`internal/generator`)
- Static, mock-deploy, and destroy harness primitives (`internal/harness`)
- Feedback loop and stuck detection helpers (`internal/feedback`)
- Filesystem run store (`internal/runstore`)

CLI commands are wired (`init`, `generate`, `validate`, `test`, `run`, `mock start`), while end-to-end command orchestration is still being integrated.

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
- OpenTofu (`tofu`) available in `PATH`
- Optional for deploy-layer integration: Mockway running locally

## Quick Start

```bash
go mod tidy
go test ./...
go run ./cmd/infrafactory --help
```

## Usage

### Validate CLI wiring

```bash
go run ./cmd/infrafactory --help
```

Current command tree:
- `init`
- `generate`
- `validate`
- `test`
- `run`
- `mock start`

### Contracts and examples

- Runtime config contract: `infrafactory.yaml`
- Scenario schema contract: `scenario.schema.json`
- Example training scenario: `scenarios/training/web-app-paris.yaml`
- Example holdout scenario: `scenarios/holdout/web-app-paris-pinned.yaml`

### Package-level test commands

```bash
go test ./internal/config
go test ./internal/scenario
go test ./internal/generator
go test ./internal/harness
go test ./internal/feedback
go test ./internal/runstore
```

### Optional layer-2 integration smoke test

```bash
INFRAFACTORY_ENABLE_INTEGRATION=1 \
INFRAFACTORY_MOCKWAY_URL=http://localhost:8080 \
go test ./internal/harness -run TestLayer2IntegrationSmoke
```

### Run artifacts location

```text
.infrafactory/runs/<scenario>/<run-id>/
```

## Local Quality Checks

```bash
bash scripts/check_all.sh
```

## Documentation Index

- Architecture: `docs/architecture.md`
- Full concept log: `CONCEPT.md`
- Decisions (ADRs): `docs/decisions/`
- Contributor guide: `CONTRIBUTING.md`
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
