# InfraFactory Usage Guide

This guide shows how to run InfraFactory in its current state.

## 1) Prerequisites

- Go `1.24+`
- OpenTofu (`tofu`) installed and available in `PATH`
- Optional for deploy-layer work: Mockway running locally

## 2) Install Dependencies and Verify Build

```bash
go mod tidy
go test ./...
```

Run full local hygiene checks:

```bash
bash scripts/check_all.sh
```

## 3) Validate CLI Wiring

Show available commands:

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

Note: command orchestration is still being integrated, so many command paths still return explicit `not implemented` stubs.

## 4) Use Config and Scenario Contracts

Primary contracts:
- Runtime config: `infrafactory.yaml`
- Scenario schema: `scenario.schema.json`

Example scenario files:
- `scenarios/training/web-app-paris.yaml`
- `scenarios/holdout/web-app-paris-pinned.yaml`

Example config file:
- `infrafactory.yaml`

## 5) Run Package-Level Tests While Developing

```bash
go test ./internal/config
go test ./internal/scenario
go test ./internal/generator
go test ./internal/harness
go test ./internal/feedback
go test ./internal/runstore
```

## 6) Optional Integration Smoke Test (Layer 2)

The integration smoke test is opt-in and skipped by default.

```bash
INFRAFACTORY_ENABLE_INTEGRATION=1 \
INFRAFACTORY_MOCKWAY_URL=http://localhost:8080 \
go test ./internal/harness -run TestLayer2IntegrationSmoke
```

## 7) Where Run Artifacts Are Stored

Run persistence is implemented under:

```text
.infrafactory/runs/<scenario>/<run-id>/
```

The store includes run metadata and iteration artifacts.

## 8) Useful References

- High-level architecture: `docs/architecture.md`
- Full design log: `CONCEPT.md`
- Backlog and execution tracking: `BACKLOG.md`, `CURRENT_TICKET.md`, `STATUS.md`
