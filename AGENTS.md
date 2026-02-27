# InfraFactory Agent Working Agreement

For AI coding agents. Human contributors should use `CONTRIBUTING.md`.

## Mission
Build `infrafactory`, a Go CLI that generates and validates OpenTofu for Scaleway scenarios with deterministic, testable behavior.

Fresh-session checklist lives in `SESSION_START.md`.

## Source of Truth
1. `scenario.schema.json`
2. `infrafactory.yaml`
3. `CONCEPT.md` prose

Additional references:
- ADRs: `docs/decisions/*.md`
- Plans: `docs/plans/*.md`
- Prompts: `prompts/*.md`
- Progress log: `STATUS.md`
- Backlog source of truth: `BACKLOG.md`
- Session execution stub: `CURRENT_TICKET.md`

## Execution Loop (mandatory)
1. Frame task with `docs/process/TICKET_TEMPLATE.md`.
2. Classify change:
- `implementation-only`
- `decision-impacting`
3. If `decision-impacting`, create/update ADR (`docs/decisions/NNNN-title.md`) and update `docs/decisions/README.md`.
4. Implement smallest runnable vertical slice.
5. Add/update focused tests.
6. Run `go test ./...` (or report why not possible).
7. Sync docs:
- Always update `STATUS.md`.
- Update `BACKLOG.md` ticket status.
- Update `CURRENT_TICKET.md` session state.
- Update `CONCEPT.md` for major architecture/durable design shifts.
- Manual end-of-session `CONCEPT.md` sweep by maintainer is additive, not a replacement.
- Update `AGENTS.md` only when workflow changes.
8. Run hygiene check before handoff:
- Local: `bash scripts/check_all.sh`
- CI/PR: `bash scripts/check_doc_hygiene.sh <base-sha> <head-sha>`

## ADR Trigger Threshold (strict)
Create/update ADR when change affects:
- public CLI contract/wiring
- cross-package architecture boundaries
- schema semantics (`scenario.schema.json`, `infrafactory.yaml`)
- external dependency strategy (tofu/mockway/opa integration model)
- durable workflow governance

Usually no ADR needed for prompt wording tweaks or internal refactors without contract change.

## Engineering Rules
- Keep command handlers thin; put logic in `internal/*` packages.
- Keep packages cohesive:
  - `internal/cli`, `internal/config`, `internal/scenario`, `internal/generator`, `internal/harness`, `internal/feedback`, `internal/runstore`, `internal/api`
  - `ui/` — SvelteKit frontend (adapter-static, embedded via `go:embed`). Build tag `noui` excludes embed and `ui` command — use `go test -tags noui ./...` when `ui/build/` does not exist.
- Use explicit structs and typed errors.
- Keep behavior deterministic and tests hermetic where possible.
- Keep CLI runnable at all times.

## Quality Bar
- `go test ./...` passes for completed slices.
- Stubs must return explicit "not implemented" errors.
- No hidden side effects outside project paths.

Roadmap for slices lives in `ROADMAP.md`.

## Safety
- Never revert/delete unrelated user changes.
- Never use destructive git commands without explicit request.
- If unexpected external changes appear, stop and ask the user.
