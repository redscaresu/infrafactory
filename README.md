# InfraFactory

Scenario-driven infrastructure generation and validation for Scaleway using OpenTofu.

## Why this exists

InfraFactory treats infrastructure like a software factory:
- You describe behavior in a scenario file.
- A generator produces OpenTofu.
- A deterministic harness validates behavior and policy.

The design intent and tradeoffs are documented, not hidden:
- Architecture: `docs/architecture.md`
- Mockway dependency contract: `docs/mockway-contract.md`
- Decision records: `docs/decisions/`
- Detailed design log: `CONCEPT.md`
- Fresh-agent start guide: `SESSION_START.md`
- Ticket execution template: `docs/process/TICKET_TEMPLATE.md`
- Reusable execution prompt: `docs/process/EXECUTION_PROMPT.md`
- Rolling execution status: `STATUS.md`
- Ticket backlog: `BACKLOG.md`
- Session execution stub: `CURRENT_TICKET.md`
- Contributor workflow: `CONTRIBUTING.md`
- AI-agent workflow (optional): `AGENTS.md`
- Implementation roadmap: `ROADMAP.md`

PRs run a doc-hygiene guardrail (`.github/workflows/doc-hygiene.yml`) to enforce `STATUS.md`/ADR synchronization.
Local pre-PR check:
`bash scripts/check_all.sh`

## Current status

Early implementation. Core planning docs and initial CLI bootstrap are in place; full slice implementation is in progress.

## Repo structure

- `cmd/infrafactory/`: CLI entrypoint
- `internal/`: core packages (CLI, config, scenario, generator, harness, feedback, runstore)
- `prompts/`: generator prompt templates
- `scenarios/`: scenario fixtures
- `policies/`: OPA policies
- `scenario.schema.json`: scenario contract

## Quick start

```bash
go mod tidy
go test ./...
go run ./cmd/infrafactory --help
```

## How To Start Agent Execution

In a fresh session, send this exact message:

```text
Use docs/process/EXECUTION_PROMPT.md exactly. Start now.
```

## Open collaboration model

This project is intentionally open about architecture and decision history so contributors can understand *why* things are designed this way, not only *what* code exists today.

## License

Apache License 2.0. See `LICENSE`.
