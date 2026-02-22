# InfraFactory

Scenario-driven infrastructure generation and validation for Scaleway using OpenTofu.

## What It Does

InfraFactory is building an infrastructure factory loop:
1. Validate scenario + config contracts.
2. Generate OpenTofu from scenario intent.
3. Validate through static and mock-deploy layers.
4. Persist run artifacts and iterate from structured feedback.

## Status

Core internal packages are implemented and tested (`internal/config`, `internal/scenario`, `internal/generator`, `internal/harness`, `internal/feedback`, `internal/runstore`).

CLI commands are wired (`init`, `generate`, `validate`, `test`, `run`, `mock start`), with end-to-end command orchestration still in progress.

## Quick Start

```bash
go mod tidy
go test ./...
go run ./cmd/infrafactory --help
```

Run local quality checks:

```bash
bash scripts/check_all.sh
```

## Docs

- Architecture: `docs/architecture.md`
- Decisions (ADRs): `docs/decisions/`
- Concept log: `CONCEPT.md`
- Backlog: `BACKLOG.md`
- Status: `STATUS.md`
- Contributor guide: `CONTRIBUTING.md`

Detailed developer README: `README.dev.md`.

## License

Apache License 2.0. See `LICENSE`.
