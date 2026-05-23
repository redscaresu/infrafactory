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
## Planning a New Slice

1. **Add tickets** to `BACKLOG.md` with id, slice, title, priority, status (`todo`), deps, and owner.
2. **Add milestone** to `ROADMAP.md`.
3. **Get approval** from the user before implementation begins.

That's it. ADRs only when crossing the threshold below. No plan files needed — the tickets are the plan.

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

## Sibling Mock Repos

Three HTTP-level mocks live alongside infrafactory and back its training scenarios:

- **mockway** (`../mockway`) — Scaleway mock; 280+ tests; runs on `:8080`. Drives the Scaleway training scenarios used in Slices 1-32.
- **fakegcp** (`../fakegcp`) — GCP mock; partial test coverage (Slice 41 is the parity ticket; no git init yet).
- **fakeaws** (`../fakeaws`) — AWS mock; **complete as of Slice 48** (commits on `main`, no remote). Ships 9 services across 5 wire formats (IAM, S3, EC2, RDS, DynamoDB, EKS, SQS, Route53, Secrets Manager); aggregate handler coverage 82.4%; 17 codex review passes archived under `../fakeaws/docs/review-passes/`. AWS-side commits land on `main`; the cross-repo mirror lives on the `fakeaws-build` branch here.

When extending a sibling mock, mirror the per-bundle PR rule in `../fakeaws/concepts.md` — handler + tests + examples + scenario anchors + coverage_matrix.yaml + `LandedServices` flip all in one slice. The `TestFullCoverageAudit` + `TestRegressionSeedAuditManifestMatchesHandlers` audits in each mock repo enforce this.

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

## Secrets
- Never commit `.env`, credentials, API keys, or private keys.
- `.gitignore` blocks common secret files (`.env`, `credentials.json`, `*.pem`, `*.key`).
- Pre-commit hook scans staged diffs for secret patterns (`SCW_ACCESS_KEY=`, `OPENROUTER_API_KEY=`, `BEGIN PRIVATE KEY`, etc.).
- If the hook blocks your commit, remove the secret from the file and use environment variables instead.
- Same protections apply to mockway and fakegcp repos.

## Safety
- Never revert/delete unrelated user changes.
- Never use destructive git commands without explicit request.
- If unexpected external changes appear, stop and ask the user.
