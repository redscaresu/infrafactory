# Contributing

## Before you start
Read:
- `README.md`
- `SESSION_START.md`
- `docs/process/TICKET_TEMPLATE.md`
- `docs/architecture.md`
- `docs/decisions/README.md`
- `STATUS.md`
- `BACKLOG.md`
- `CURRENT_TICKET.md`

## Workflow
1. Pick a focused change.
2. Add/update tests with behavior changes.
3. Run `go test ./...`.
4. Keep errors explicit and actionable.
5. If decision-impacting, add/update ADR.
6. If major architecture changed, update `CONCEPT.md`.
7. Update `STATUS.md`.
8. Update `BACKLOG.md` and `CURRENT_TICKET.md`.
9. Before commit run: `bash scripts/check_all.sh`.

## ADR policy
Expected for schema/CLI/architecture-boundary changes.
Usually not required for prompt-only or internal refactor-only changes.

## Source-of-truth precedence
1. `scenario.schema.json`
2. `infrafactory.yaml`
3. `CONCEPT.md` prose

## AI-assisted work
AI contributors must follow `AGENTS.md`.
