# Session Start (Fresh Agent)

## 1) Load minimal context
1. `README.md`
2. `AGENTS.md`
3. `STATUS.md`
4. `BACKLOG.md`
5. `CURRENT_TICKET.md`
6. `docs/process/TICKET_TEMPLATE.md`
7. `docs/process/EXECUTION_PROMPT.md`
8. `docs/decisions/README.md` (+ relevant ADRs)
9. `docs/mockway-contract.md`
10. `CONCEPT.md` only if major design context is needed

## 2) Define target
- Confirm active milestone in `ROADMAP.md`.
- Confirm blockers/next actions in `STATUS.md`.
- Pick next uncompleted ticket from `BACKLOG.md`.
- Fill `CURRENT_TICKET.md` for session execution.

Quick repo-state preflight before selecting a ticket:
```bash
git status --short
git branch --show-current
git log -1 --oneline
```
- If you see unexpected local changes you did not make, stop and ask how to proceed.
- Keep exactly one `in_progress` ticket in `BACKLOG.md` during execution.

## Fresh Context Briefing (Current)

Before writing code, confirm these facts are still true in `STATUS.md`/`BACKLOG.md`:
1. `S9-T8` (sandbox/live deploy, real Scaleway) remains permanently blocked by governance policy (ADR-0003).
2. Output and logs must explicitly surface: `(real deployment skipped for cost reasons for now)` for sandbox/live-blocked behavior.
3. Slice 16 issue-driven robustness hardening is complete; preserve its guarantees while keeping the remaining blocked lane (`S9-T8`) unchanged unless ADR-0003 is superseded.
4. Slice 17 hardening follow-ups are complete (`S17-T1`, `M31`, `M32`, `M33`, `M34`); preserve opt-in capture defaults, convergence fixes, self-review canonical-only matching, and docs alignment.
5. `run` is criteria-aware and includes criteria-only holdout completion checks; do not regress to coarse stage-only convergence behavior.
6. `dns_resolution` remains auto-pass informational output while sandbox/live deploy is blocked; do not treat it as a hard-fail criterion.
7. Default runtime now uses concrete generator transports; `claude-code` requires `agent.claude.command` in `PATH` and `openrouter` requires `OPENROUTER_API_KEY` plus `agent.openrouter.model`.
8. Slices 13-15 are complete in MVP form; preserve logging contracts, feedback fidelity, and adaptive retry behavior while applying issue fixes.
9. Slice 16 includes schema-validation hardening; scenario loads must not silently skip schema validation when schema paths are unavailable.
10. For ticket-planning closures, record refinement pass outcomes and require at least one improvement pass plus a no-change verification pass in both `STATUS.md` and `CURRENT_TICKET.md`.
11. **Slices 18-19 complete** — mockway covers all targeted Scaleway APIs (K8s, IAM, Container Registry, Redis, Composite). Reliability review (`S19-T1`) finished with all bugs fixed and regression-tested. All 6 scenarios pass on first iteration.
12. **Slice 21 (Web UI) planned** — `infrafactory ui` command serving SvelteKit frontend embedded via `go:embed`. 8 sub-slices (SUi-1..SUi-8). Full plan at `docs/plans/web-ui-plan.md`. Key integration: `WebSocketSink` plugs into existing `AppLogger.sinks` for real-time streaming. `executeRunLoop` extraction from `runRunCommand` enables API reuse.
13. **Slice 21 testing contract** — `go test -tags noui ./...` must pass at all times (no CLI breakage). New `internal/api/` tests use `httptest.NewServer`. WebSocket hub has dedicated unit tests. Frontend uses Playwright e2e for critical flows.

Minimal startup verification commands:
```bash
go test -tags noui ./...    # Use -tags noui until ui/build/ exists
bash scripts/check_all.sh
node --version              # Must show v18+ (only for Slice 21 frontend work)
```

If either command fails, restore the repo to a green baseline before starting a new ticket.

### Fresh Context Addenda (Operational)
- Prefer `run` over manual `generate` + `test` when diagnosing/repairing generation failures; only `run` feeds prior iteration failures into LLM generation (`FeedbackJSON`).
- Avoid introducing new provider-specific string normalization rules in `generate`; prioritize improving feedback quality so the model corrects output itself.
- Mockway startup failures are commonly local port collisions (`0.0.0.0:8080` already allocated); resolve the conflicting container/process before retrying `mock start`.
- Use `http://127.0.0.1:8080` for local Mockway checks in this repo context (more reliable than `localhost`).
- Debug iterative behavior from run artifacts:
  `.infrafactory/runs/<scenario>/<run-id>/iterations/<n>/iteration.json` records stage/failure snapshots per iteration.
- Keep output semantics in mind:
  `output/<scenario>/` is latest generated IaC and is overwritten each run; historical evidence lives under `.infrafactory/runs/`.
- Provider schema extraction is lazy and generate-path-only:
  first `generate`/`run` generation call attempts schema extraction once (cached in runtime); `validate`/`test`/`mock` should not pay that startup cost.
- Schema extraction is best-effort:
  if `tofu init/providers schema` fails, generation proceeds without schema-enriched prompts; inspect logs for `provider_schema` skip details before debugging prompt quality.
- Preserve Slice 13-15 guarantees while fixing Slice 16 issues:
  keep deterministic JSON-line logging fields/redaction behavior, structured run feedback (`failure_class` and detailed failure context), and adaptive transport stop behavior with persisted diagnostics.
- For any future planning refinement over unfinished slices (`todo`/`blocked` backlog work):
  require the same refinement protocol: continue until two consecutive no-change passes, and explicitly record pass outcomes in both `CURRENT_TICKET.md` and `STATUS.md` for fresh-context continuity.
  when refining blocked slices (for example `S9-T8`), constrain work to governance/docs/risk messaging only unless the blocking ADR/policy is explicitly superseded.
- Optimized post-slice review prompt (apply to each unfinished slice after implementation):
  "After completing this slice, run a dedicated review-improve pass over code, tests, docs, and artifacts. Apply any improvements that increase correctness, determinism, observability, and operator clarity. Repeat review-improve passes until two consecutive passes find no further improvements. Record each pass outcome in `STATUS.md` and `CURRENT_TICKET.md`."

### Slices 7-10 default execution constraints (completed)
- All tickets done. Preserve CLI/output contracts, golden snapshots, error taxonomy, and hermetic test defaults.
- `S9-T8` (sandbox/live deploy) remains permanently blocked by ADR-0003.
- Smoke/runtime caveats: prefer `http://127.0.0.1:8080` over `localhost` for Mockway checks; `tofu` must exist in `PATH` for real-tool smoke.

### Slice 11 default execution constraints (completed)
- All tickets done (`S11-T1`..`S11-T7`). Preserve transport contracts and credential-redaction behavior.

### Slices 12-16 default execution constraints (completed)
- All tickets done. Key contracts to preserve:
  - Single run control: `repair_iterations_max` only; stop on first success (`target_reached`).
  - One canonical terminal stop reason per stop event.
  - Deterministic JSON-line logging with secret redaction.
  - Structured failure payloads with `failure_class` tagging in `FeedbackJSON`.
  - Adaptive retry: transport-dominated runs stop early with actionable reasons.
  - Context propagation: `cmd.Context()` everywhere, no `context.Background()`.
  - Bounded response reads, deterministic env override injection.

### Slice 18 default execution constraints
- Goal:
  Expand mockway coverage to remaining Scaleway API surfaces via standalone scenarios that drive iterative mockway/infrafactory fixes.
- Canonical order:
  `S18-T1 || S18-T2` (Tier 1, parallel) → `S18-T3 || S18-T4` (Tier 2, parallel) → `S18-T5` (composite).
- Schema extension rule:
  `scenario.schema.json` resources has `additionalProperties: false`; must add `iam`, `registry`, `redis` properties before those scenarios can validate. Corresponding Go struct fields must be added to `internal/scenario/scenario.go` `Resources`.
- Mappings rule:
  `mappings.yaml` must include redis size mappings (RED1-MICRO, RED1-S, RED1-M, RED1-L) before Redis scenario.
- Iteration protocol:
  write scenario → `infrafactory run <scenario>` → diagnose failures → fix mockway or infrafactory → write regression test → `docker compose up --build -d mockway` → rerun until first-iteration pass.
- **Autonomous execution rule**:
  Do NOT ask the human for confirmation at any point during Slice 18 (`S18-T1`..`S18-T5`). This includes writing scenarios, fixing bugs in mockway or infrafactory, extending schemas, adding mockway handlers/tables/tests, updating prompts, and rebuilding Docker. Proceed autonomously through all fixes.
- Gap detection signal:
  **501 status codes** in `tofu apply`/`tofu destroy` stderr indicate mockway is missing a handler for that API endpoint. This is the primary signal to add a new route.
- Docker rebuild rule:
  **Always** run `docker compose up --build -d mockway` after any mockway code change. Forgetting this causes debugging against stale code.
- Existing mockway handlers (as of M34):
  compute (`instance.go`), networking/VPC (`vpc.go`), RDB (`rdb.go`), K8s (`k8s.go`), IAM (`iam.go`), Container Registry (`registry.go`), Redis (`redis.go`), block storage (basic).
- Companion resource map:
  `internal/generator/schema_filter.go` `companionResourceTypes` maps parent→sub-resources for provider schema filtering. Update this if new resource types have sub-resources (e.g., registry namespace→image).
- Mockway patterns:
  SQLite-backed repository (`../mockway/repository/repository.go`), chi router, UUID IDs, RFC3339 timestamps, zone/region scoping. Use `handlers/rdb.go` and `handlers/k8s.go` as reference for new services.
- Test rule:
  all new mockway handlers must have CRUD tests using `testutil.NewTestServer(t)` pattern. All infrafactory tests must pass: `go test ./...`.
- Out of scope:
  Serverless/Containers, S3 (follow-up slice with off-the-shelf mock).

### Slice 19 default execution constraints
- Depends on:
  Slice 18 must be fully complete (`S18-T1`..`S18-T5` all done) before starting Slice 19.
- Goal:
  Review all Slice 18 code in both mockway and infrafactory. Identify bugs, edge cases, error handling gaps, and correctness issues. Fix them with regression tests. Rerun all scenarios until green.
- Iteration protocol:
  review code → identify issue → fix in mockway or infrafactory → write regression test → `docker compose up --build -d mockway` → run all scenarios (`k8s-cluster-paris`, `iam-policies-paris`, `registry-paris`, `redis-paris`, `full-stack-paris`) → repeat until all pass on first iteration.
- **Autonomous execution rule**:
  Do NOT ask the human for confirmation at any point during `S19-T1`. Diagnose, fix, test, rebuild, and rerun autonomously until all issues are resolved.
- Review scope:
  all Slice 18 files — scenarios, mockway handlers/tables/tests, `scenario.schema.json`, `scenario.go`, `mappings.yaml`, `schema_filter.go`, prompt updates. Look for: missing error handling, incorrect API response shapes, missing cascade deletes, wrong status codes, schema validation gaps, edge cases in CRUD operations.

### Slice 21 (Web UI) execution constraints
- Goal:
  Add `infrafactory ui` command serving SvelteKit frontend embedded via `go:embed`. 8 sub-slices (SUi-1..SUi-8).
- Plan:
  `docs/plans/web-ui-plan.md` — read Quick Reference, Prerequisites, your slice section, and Pitfalls before coding.
- Build tag rule:
  Until `ui/build/` exists (populated by `npm run build`), always use `-tags noui` for Go builds and tests. The `!noui` build (`embed.go`) requires `ui/build/` to exist — without it, `go build` fails with `pattern all:ui/build: no matching files found`.
- Dev workflow:
  Two terminals: (1) `go run -tags noui ./cmd/infrafactory ui --addr 127.0.0.1:4173` (API-only, non-API paths return 404 since assets=nil), (2) `cd ui && npm run dev` (Vite on :5173, proxies `/api` → `127.0.0.1:4173`).
- Prerequisites:
  Node.js 18+ and npm 9+ required for frontend work. Not needed for `go test -tags noui ./...`.
- Test invariant:
  `go test -tags noui ./...` must pass after every commit — this is the CI gate.
- Single new Go dependency:
  `github.com/coder/websocket` only. No external HTTP routers or logging frameworks.
- Path safety:
  All handlers serving files from disk must reject `..` segments. Test path traversal in every handler that reads from `cfg.Paths.*`.

### S17-T1 implementation reference (completed)
- Activation contract:
  capture is env-gated (`INFRAFACTORY_CAPTURE_LLM_RAW=1`); default behavior persists no LLM prompt/raw artifacts.
- Artifact contract:
  capture files are written under run artifacts (`.infrafactory/runs/<scenario>/<run-id>/iterations/<n>/`) with deterministic phase naming and stable metadata envelopes:
  `llm_raw_<phase>.json` and `llm_prompt_<phase>.json`.
- Safety contract:
  deterministic secret-like redaction and hard byte caps with explicit truncation markers are applied before persistence.
- Feedback-debugging guardrail:
  use paired prompt/response artifacts from the same iteration to verify whether failure feedback reached model input before changing prompt wording.
- Compatibility guardrail:
  preserve default output contract, terminal reasons, and existing run artifact readers when capture is disabled.

## 3) Execute
- Implement + test.
- Run `go test ./...` (or explain why not).

## 4) Mandatory sync before handoff
- Update `STATUS.md`.
- Update `BACKLOG.md` ticket status.
- Update `CURRENT_TICKET.md` (final state + notes).
- If decision-impacting: update ADR + ADR index.
- If major architecture changed: update `CONCEPT.md`.
- If workflow changed: update `AGENTS.md`.
- Run `bash scripts/check_all.sh`.

## 5) Handoff format
- What changed
- What was verified
- Open blockers/risks
- Exact next step
