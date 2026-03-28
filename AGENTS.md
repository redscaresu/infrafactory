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

## Project File Ecosystem

| File | Purpose | When to update |
|---|---|---|
| `SESSION_START.md` | Fresh-context checklist and briefing facts for new conversations | When new durable contracts or operational guardrails are established |
| `ROADMAP.md` | Stable milestones and sequencing (high-level) | When a new slice is planned or completed |
| `BACKLOG.md` | Single source of ticket status across all slices | When tickets are created, started, or completed |
| `CURRENT_TICKET.md` | Per-session execution stub (one active ticket) | At session start and as work progresses |
| `STATUS.md` | Progress log with recent updates | At end of each meaningful coding session |
| `CONCEPT.md` | Durable architecture, contracts, design decisions | Only for major architecture/design shifts |
| `docs/decisions/*.md` | ADRs for decision-impacting changes | When change crosses ADR trigger threshold (see below) |
| `docs/plans/*.md` | Design reference for planned slices | When planning a new slice |
| `docs/process/TICKET_TEMPLATE.md` | Template for framing tickets | Rarely (process changes only) |
| `docs/process/EXECUTION_PROMPT.md` | Reusable autonomous execution prompt | Rarely (process changes only) |

## Planning a New Slice

When the user asks to plan new work (a new slice, feature, or initiative):

1. **Research** — explore the codebase and read relevant docs to understand what exists and what's missing.
2. **Write plan** — create `docs/plans/<slice-name>-plan.md` with: context, quick reference, gap analysis, tickets with acceptance criteria and impacted files, execution order, verification steps, and out-of-scope items. Follow the format of existing plans (e.g., `docs/plans/web-ui-plan.md`).
3. **Add milestone** — append the slice to `ROADMAP.md` milestones with a summary and execution order.
4. **Add tickets** — add tickets to `BACKLOG.md` with id, slice, title, priority (`P1`/`P2`), status (`todo`), deps, and owner. Insert new tickets at the top of the table (newest first).
5. **Get approval** — present the plan to the user before implementation begins.

Do NOT start implementing until the plan is approved. The plan file is the design reference; `BACKLOG.md` is the execution tracker.

## Session Bootstrap (Fresh Context)

Follow `SESSION_START.md` exactly:

1. Load the minimal context files listed there (README, AGENTS, STATUS, BACKLOG, CURRENT_TICKET, etc.).
2. Run the preflight commands (`git status`, `git branch`, `git log`).
3. Confirm active milestone in ROADMAP, blockers in STATUS, and next ticket in BACKLOG.
4. Fill `CURRENT_TICKET.md` using `docs/process/TICKET_TEMPLATE.md`.
5. Keep exactly one `in_progress` ticket in BACKLOG during execution.

If unexpected local changes appear, stop and ask the user.

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

## Scaleway Bootstrap (Layer 3 Prerequisites)

Layer 3 (real Scaleway deploy) uses a self-managed project lifecycle per ADR-0010. The generated HCL always includes a `scaleway_account_project` resource — infrafactory creates and destroys its own project as part of the IaC lifecycle. No pre-existing sandbox project is required.

**What the user must provide:**

1. **Create org-level API keys** — via the [Scaleway console](https://console.scaleway.com/) under IAM → API Keys. The keys must have **organization-level permissions** (not project-scoped) so that `scaleway_account_project` can be created/destroyed by terraform.
2. **Set environment variables**:
   - `SCW_ACCESS_KEY` — the API key ID
   - `SCW_SECRET_KEY` — the API secret key
3. **Enable Layer 3 in config** — set `validation.layers.sandbox_deploy.enabled: true` in `infrafactory.yaml`.

**Project lifecycle:**
- `tofu apply` creates the project and all resources inside it.
- `tofu destroy` destroys the project and everything inside it.
- With `--no-destroy`, the project persists in `terraform-live.tfstate` and is reused on the next incremental run.
- No `sandbox_project_id` config is needed — the project ID comes from terraform state.

Credential validation happens at run start — missing keys produce a hard failure with a clear error message, not silent degradation to mock-only.

## Safety
- Never revert/delete unrelated user changes.
- Never use destructive git commands without explicit request.
- If unexpected external changes appear, stop and ask the user.
