# InfraFactory Agent Working Agreement

For AI coding agents. Human contributors should use `CONTRIBUTING.md`.

## Mission
Build `infrafactory`, a Go CLI + SvelteKit UI that generates and validates OpenTofu across **AWS**, **GCP**, and **Scaleway** with deterministic, testable behavior. Each cloud's scenarios validate against a deterministic HTTP-level mock (fakeaws / fakegcp / mockway); S3 routes through a small in-repo shim (`cmd/s3router/`, S80) that fans traffic between SeaweedFS (data plane) and fakeaws (`?publicAccessBlock` subresource SeaweedFS doesn't model). See `CONCEPT.md` § "Third-Party Mock Integration".

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
| `docs/plans/<arc-name>-plan.md` | Goal-named variable-length arc plan (typically 2-4 slices). Tickets, exit criteria, autonomous-execution prompt. | When planning the next arc |
| `docs/NEXT_SESSION.md` | Fresh-context handoff — points at the active arc + open follow-ups | At end of each meaningful coding session |
| `docs/status/ARCHIVE.md` | Per-arc close-out narratives — durable history | At arc close-out |
| `STATUS.md` | Current phase + recent arc summaries | At end of each meaningful coding session |
| `BACKLOG.md` | M-ticket maintenance backlog (cross-arc work that doesn't fit the active arc). Stub today — no active M-tickets. Historical entries in `BACKLOG_ARCHIVE.md`. | When a maintenance need surfaces that doesn't belong in the active arc plan |
| `CONCEPT.md` | Durable architecture, contracts, design decisions | Only for major architecture/design shifts |
| `docs/decisions/*.md` | ADRs for decision-impacting changes | When change crosses ADR trigger threshold (see below) |

## Planning a New Arc

Arcs are **goal-named** and **variable-length** (typically 2-4 slices, sometimes up to 6 — but driven by the goal, not by a slice-count template). Adopted 2026-06-03 after the S89–S93 close-out (`docs/status/ARCHIVE.md` § "2026-06-03 S89–S93" → "Scaffold question").

The shape:

1. **Name the arc by goal**, not by slice numbers — e.g. "39/39 sustain validation", "fakegcp panic audit". Filename: `docs/plans/<arc-name>-plan.md` (kebab-case). The arc still numbers its slices sequentially (S94, S95, …) for cross-reference into commits / ARCHIVE entries.
2. **Write the plan**: Big picture (what + why), Slices table (as many as the goal needs, including any sweep / audit / investigation steps), Standing rules (inherit from prior arcs), per-slice motivation + tickets + exit criteria, autonomous-execution loop prompt, fresh-context checklist.
3. **No padding.** If the goal naturally fits in 2 slices, the plan is 2 slices. The 5-slice template from S54–S93 is retired.
4. **Mandatory close-out**: every arc, regardless of length, ends with a `docs/status/ARCHIVE.md` per-arc section + `docs/NEXT_SESSION.md` update. The close-out can be the last slice or folded into the last substantive PR — but it has to happen. (The ARCHIVE entries are the project's institutional memory and the only thing that makes a fresh session bootable.)
5. **Repoint** `STATUS.md` "Next arc planned" line + `docs/NEXT_SESSION.md` at the new plan.
6. **Get approval** from the user before kicking off the autonomous loop.

ADRs only when crossing the threshold below. Plan files are the slice ticket source — `BACKLOG.md` is only for cross-arc maintenance work (M-tickets).

## Fresh Context

When starting a new conversation, follow this checklist:

### 1) Load minimal context
1. `README.md`
2. `AGENTS.md` (this file)
3. `docs/NEXT_SESSION.md` (open follow-ups from prior session — read FIRST when starting work)
4. `STATUS.md`
5. `docs/plans/slices-*.md` for the active arc — see `docs/NEXT_SESSION.md` for the current arc pointer
6. `BACKLOG.md` (M-ticket maintenance backlog; slice tickets live in plan files)
7. `CONCEPT.md` (if major design context is needed)
8. `docs/decisions/README.md` (+ relevant ADRs)

Slice work is organised as goal-named arcs (variable-length, typically 2-4 slices). Each arc lives in `docs/plans/<arc-name>-plan.md` with an autonomous-execution loop prompt at the bottom. `docs/status/ARCHIVE.md` has per-arc close-out narratives. Historical 5-slice arcs (S54–S93) live under `docs/plans/slices-<a>-<b>-plan.md`; the naming convention shifted at the S94 boundary.

### 2) Preflight
```bash
git status --short
git branch --show-current
git log -1 --oneline
```
- If unexpected local changes appear, stop and ask the user.
- Confirm active arc in `docs/NEXT_SESSION.md` § "Suggested next arc" or "READ FIRST"; blockers in `STATUS.md`.
- Pick next uncompleted slice from the active `docs/plans/slices-*-plan.md`.
- For maintenance work not tied to an arc, see `BACKLOG.md` M-tickets.

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
- Mock rebuild required after any sibling-mock code change: `pkill -f <mock-bin>; cd ../<mock> && go build && ./<bin> --port <port> &`. For containerised runs use `make mocks-down-containers && make mocks-up-containers`.
- Build tag: `-tags noui` required when `ui/build/` doesn't exist. The `!noui` build requires `ui/build/`.
- Playwright e2e tests live in `ui/e2e/` (currently 51 tests, growing). Run with `make test` (Go unit + UI unit + Playwright).
- Visual baselines under `ui/e2e/visual.spec.ts-snapshots/` render live UI state — adding scenario YAMLs OR completing runs (which add rows to the Runs page) drifts them. Pre-commit hook auto-refreshes when `scenarios/training/*.yaml` changes (M56); for other drift, run `make ui-baseline-update` manually.
- `make run` builds everything and starts the UI at `http://127.0.0.1:4173`.
- `make up` is the one-shot bring-up: mockway + fakegcp + fakeaws + SeaweedFS + UI in one command. `make down` tears down the mocks (Ctrl-C stops the UI).
- **Sweep protocol** (see `feedback_sweep_protocol.md` memory): treat failures as either (a) mock-server gaps → fix at source in `fakeaws`/`fakegcp`/`mockway`, never seed `pitfalls/*.yaml`; or (b) LLM-generated HCL mistakes → let auto-learning capture a pitfall. Four CI ratchets enforce pitfall purity: `TestPitfallsNoHumanSeeding` rejects `source: seed`/`static`; `TestPitfallsNoMockActionableSeeds` rejects mock-actionable substrings; `TestPitfallsNoOPADuplication` rejects verbatim OPA-msg duplication; `TestPitfallsSourceEnum` fences the allowed source-field values to `learned` / `learned_from_diff` / `learned_from_diff_avoid` only. After-sweep cleanup: `scripts/sweep_39.sh` runs `bin/pitfall-merge` (S94) to selectively restore — `learned_from_diff_avoid` (N13 deletion-as-fix, grounded in a confirmed successful run) is preserved; `learned` + `learned_from_diff` are discarded as sweep noise.
- **Cross-repo cascade commits**: lifecycle-parity work spans infrafactory + a sibling mock. Commit the mock-side change first (it's the dependency), then update infrafactory's e2e test or call sites that depend on the new mock behavior. All four repos use origin/main; push order matters.
- **Demo recording tooling**: `asciinema` (CLI demo via `./docs/demo/record.sh`) → `.cast` → `agg` → `.gif`; Playwright (UI demo via `make demo-ui`) → `.webm` → `gifski` → `.gif`. Build-time dep only; never advertised in user-facing docs.

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

Three first-party HTTP-level mocks + one third-party backend live alongside infrafactory:

- **mockway** (`../mockway`, github.com/redscaresu/mockway) — Scaleway mock; 280+ tests; runs on `:8080`. Apache-2.0, public.
- **fakegcp** (`../fakegcp`, github.com/redscaresu/fakegcp) — GCP mock; runs on `:8081`. Mockway-level test parity reached 2026-05-23 (881-line repository_test.go, FK violation tests, cascade delete tests). Memorystore + Cloud SQL + GKE + IAM + Storage + DNS + Pub/Sub + Secret Manager + Cloud Run + Cloud KMS (added 2026-05-31 in fakegcp@c7999b5).
- **fakeaws** (`../fakeaws`, github.com/redscaresu/fakeaws) — AWS mock; runs on `:8082`. Ships 10 services across 5 wire formats (IAM, S3, EC2, RDS, DynamoDB, EKS, SQS, Route53, Secrets Manager, KMS); aggregate handler coverage 82.4%; 17 codex review passes archived under `../fakeaws/docs/review-passes/`. EC2/IAM/Route53/DynamoDB substantially broadened 2026-05-30 → 2026-05-31 from a self-learning sweep (see fakeaws@348322d).
- **SeaweedFS** (`chrislusf/seaweedfs` container) — third-party S3 backend for `aws_s3_bucket` reads (`terraform-provider-aws` needs the full management surface; fakeaws's stripped S3 handler isn't enough). Runs on `:9090` via `docker-compose.mocks.yml`. Anonymous-mode `ListAllMyBuckets` returns empty even when buckets exist — use HEAD-by-name as the assertion path. Empirical evaluation log in `CONCEPT.md` § "Third-Party Mock Integration" (rejects Adobe S3Mock + Garage + LocalStack + MinIO).
- **s3router** (`cmd/s3router/`, S80) — in-repo reverse-proxy shim that listens on `:9091` and fans S3 traffic across SeaweedFS and fakeaws. `?publicAccessBlock` → fakeaws (SeaweedFS uniquely 501s on that subresource); everything else → SeaweedFS; `PUT/DELETE /<bucket>` fans out to both. `infrafactory.yaml` `s3.url` points at the shim, not SeaweedFS directly. Add a subresource to `fakeawsSubresources` in `main.go` only when a new SeaweedFS 501 surfaces. ADR-0015 § "S80 — S3 backend router" carries the rationale.

All four sibling repos are independent public OSS repos on origin/main; cross-repo work cascades (see "operational caveats" above). The s3router is part of the infrafactory repo, not a sibling.

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
