# InfraFactory Agent Working Agreement

For AI coding agents. Human contributors should use `CONTRIBUTING.md`.

## Mission
Build `infrafactory`, a Go CLI that generates and validates OpenTofu for Scaleway scenarios with deterministic, testable behavior.

## Source of Truth
1. `scenario.schema.json`
2. `infrafactory.yaml`
3. `CONCEPT.md` prose

Additional references:
- ADRs: `docs/decisions/*.md`
- Prompts: `prompts/*.md`
- Pitfalls: `pitfalls/{cloud}.yaml` — provider-specific rules loaded at runtime by scenario `cloud` field
- Progress log: `STATUS.md`
- Backlog source of truth: `BACKLOG.md`

## Project File Ecosystem

| File | Purpose | When to update |
|---|---|---|
| `ROADMAP.md` | Stable milestones and sequencing (high-level) | When a new slice is planned or completed |
| `BACKLOG.md` | Single source of ticket status across all slices | When tickets are created, started, or completed |
| `STATUS.md` | Progress log with recent updates | At end of each meaningful coding session |
| `CONCEPT.md` | Durable architecture, contracts, design decisions | Only for major architecture/design shifts |
| `docs/decisions/*.md` | ADRs for decision-impacting changes | When change crosses ADR trigger threshold (see below) |
| `docs/plans/*.md` | Design reference for complex slices needing design discussion (optional) | When planning a complex slice |
| `docs/process/TICKET_TEMPLATE.md` | Template for framing tickets | Rarely (process changes only) |
| `docs/process/EXECUTION_PROMPT.md` | Reusable autonomous execution prompt | Rarely (process changes only) |

## Planning a New Slice

1. **Research** — explore the codebase and read relevant docs.
2. **Add tickets** — add tickets to `BACKLOG.md` with id, slice, title, priority, status (`todo`), deps, and owner.
3. **Optionally write plan** — for complex slices, create `docs/plans/<slice-name>-plan.md`. Follow format of existing plans.
4. **Add milestone** — append the slice to `ROADMAP.md`.
5. **Get approval** — present the plan to the user before implementation begins.

## Fresh Context

When starting a new conversation, follow this checklist:

### 1) Load minimal context
1. `README.md`
2. `AGENTS.md` (this file)
3. `STATUS.md`
4. `BACKLOG.md`
5. `CONCEPT.md` (if major design context is needed)
6. `docs/decisions/README.md` (+ relevant ADRs)

### 2) Preflight
```bash
git status --short
git branch --show-current
git log -1 --oneline
```
- If unexpected local changes appear, stop and ask the user.
- Confirm active milestone in `ROADMAP.md`, blockers in `STATUS.md`.
- Pick next uncompleted ticket from `BACKLOG.md` (status: `todo` or `in_progress`).
- Keep exactly one `in_progress` ticket in `BACKLOG.md` during execution.

### 3) Startup verification
```bash
go test -tags noui ./...    # Use -tags noui until ui/build/ exists
bash scripts/check_all.sh
```
If either fails, restore the repo to a green baseline before starting a new ticket.

### 4) Operational caveats
- Prefer `run` over manual `generate` + `test` — only `run` feeds prior iteration failures into LLM generation.
- Use `http://127.0.0.1:8080` for local Mockway checks (more reliable than `localhost`).
- Port 8080 conflicts are common — check for stale containers before `mock start`.
- Debug iterative behavior from `.infrafactory/runs/<scenario>/<run-id>/iterations/<n>/iteration.json`.
- `output/<scenario>/` is mutable (overwritten each run); immutable snapshots live under `.infrafactory/runs/<scenario>/<run-id>/generated/`.
- `CLAUDECODE` env var blocks nested claude — `unset CLAUDECODE` before `go run ./cmd/infrafactory run`.
- Docker rebuild required after any mockway code change: `docker compose up --build -d mockway`.
- Build tag: `-tags noui` required when `ui/build/` doesn't exist. The `!noui` build requires `ui/build/`.
- Playwright e2e tests: 18 tests in `ui/e2e/`. Run with `make test` (Go unit + UI unit + Playwright).
- `make run` builds everything and starts the UI at `http://127.0.0.1:4173`.

## Execution Loop (mandatory)
1. Frame task with `docs/process/TICKET_TEMPLATE.md`.
2. Classify change: `implementation-only` or `decision-impacting`.
3. If `decision-impacting`, create/update ADR (`docs/decisions/NNNN-title.md`).
4. Implement smallest runnable vertical slice.
5. Add/update focused tests.
6. Run `go test ./...` (or report why not possible).
7. Sync docs: update `STATUS.md`, `BACKLOG.md` ticket status. Update `CONCEPT.md` for major shifts. Update `AGENTS.md` only when workflow changes.
8. Run hygiene check: `bash scripts/check_all.sh`.

## ADR Trigger Threshold
Create/update ADR when change affects:
- public CLI contract/wiring
- cross-package architecture boundaries
- schema semantics (`scenario.schema.json`, `infrafactory.yaml`)
- external dependency strategy (tofu/mockway/opa integration model)
- durable workflow governance

## Engineering Rules
- Keep command handlers thin; put logic in `internal/*` packages.
- Keep packages cohesive: `internal/cli`, `internal/config`, `internal/scenario`, `internal/generator`, `internal/harness`, `internal/feedback`, `internal/runstore`, `internal/api`.
- `ui/` — SvelteKit frontend (adapter-static, embedded via `go:embed`).
- Use explicit structs and typed errors.
- Keep behavior deterministic and tests hermetic where possible.

## Quality Bar
- `go test ./...` passes for completed slices.
- Stubs must return explicit "not implemented" errors.
- No hidden side effects outside project paths.

## Scaleway Bootstrap (Layer 3 Prerequisites)

Layer 3 uses self-managed project lifecycle per ADR-0010. Generated HCL includes `scaleway_account_project` — infrafactory creates/destroys its own project. No pre-existing sandbox required.

**User must provide:**
1. Org-level API keys (IAM -> API Keys, organization-level permissions).
2. Env vars: `SCW_ACCESS_KEY`, `SCW_SECRET_KEY`.
3. Enable Layer 3: `validation.layers.sandbox_deploy.enabled: true` in `infrafactory.yaml`.

## Safety
- Never revert/delete unrelated user changes.
- Never use destructive git commands without explicit request.
- If unexpected external changes appear, stop and ask the user.
