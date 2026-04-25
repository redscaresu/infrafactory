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
1. `S9-T8` (sandbox/live deploy, real Scaleway) is unblocked — ADR-0010 supersedes ADR-0003. Layer 3 real Scaleway deploy is planned in Slices 26-29.
2. Output and logs surface `(real deployment skipped)` when `validation.layers.sandbox_deploy.enabled` is `false` (the default).
3. Slice 16 issue-driven robustness hardening is complete; preserve its guarantees.
4. Slice 17 hardening follow-ups are complete (`S17-T1`, `M31`, `M32`, `M33`, `M34`); preserve opt-in capture defaults, convergence fixes, self-review canonical-only matching, and docs alignment.
5. `run` is criteria-aware and includes criteria-only holdout completion checks; do not regress to coarse stage-only convergence behavior.
6. `dns_resolution` remains auto-pass informational output until Layer 3 real Scaleway deploy is implemented (Slice 27); do not treat it as a hard-fail criterion.
7. Default runtime now uses concrete generator transports; `claude-code` requires `agent.claude.command` in `PATH` and `openrouter` requires `OPENROUTER_API_KEY` plus `agent.openrouter.model`.
8. Slices 13-15 are complete in MVP form; preserve logging contracts, feedback fidelity, and adaptive retry behavior while applying issue fixes.
9. Slice 16 includes schema-validation hardening; scenario loads must not silently skip schema validation when schema paths are unavailable.
10. For ticket-planning closures, record refinement pass outcomes and require at least one improvement pass plus a no-change verification pass in both `STATUS.md` and `CURRENT_TICKET.md`.
11. **Slices 18-19 complete** — mockway covers all targeted Scaleway APIs (K8s, IAM, Container Registry, Redis, Composite). Reliability review (`S19-T1`) finished with all bugs fixed and regression-tested. All 6 scenarios pass on first iteration.
12. **Slice 21 (Web UI) complete** — `infrafactory ui` now serves the SvelteKit frontend (embedded in normal builds, API-only under `-tags noui`). Preserve the current contracts: `/diagnostics` runtime health, `/runs` newest-first history, `/runs/<scenario>/<run-id>` per-run IaC viewer, and immutable run-scoped IaC snapshots under `.infrafactory/runs/<scenario>/<run-id>/generated/`.
13. **Slice 21 testing contract** — `go test -tags noui ./...` must pass at all times (no CLI breakage). `internal/api/` uses `httptest`, WebSocket hub has dedicated unit tests, and frontend regressions currently use lightweight Node-based tests (`cd ui && npm test`) plus `npm run build`.
14. **Live page runtime contract** — a started run must become visible immediately via `run.json` with `status: running`; do not defer first metadata write until terminal completion. The Live page is allowed to fall back to synthesized console lines from polled run metadata/iteration artifacts when websocket delivery is absent.
15. **Dev websocket contract** — in Vite dev mode (`:5173`), the browser connects directly to the backend websocket origin (`ws://127.0.0.1:4173/api/ws` by default, override with `VITE_UI_API_ORIGIN`). Do not reintroduce a dependency on Vite’s websocket proxy for Live logs.
16. **Websocket origin contract** — the backend `/api/ws` handler must explicitly allow local dev origins (`127.0.0.1:*`, `localhost:*`) because the dev UI is cross-origin (`:5173` → `:4173`). Keep a regression test that dials with `Origin: http://127.0.0.1:5173`.
17. **UI-run Claude contract** — when `agent.type=claude-code`, the `ui` command resolves `agent.claude.command` to an absolute binary path during preflight and uses that exact path for the async run runtime. Do not rely on later `PATH` lookups drifting across goroutines/process contexts.
18. **Run history contract** — `/api/runs` and `/api/runs/{scenario}` must skip incomplete historical run directories that lack `run.json` instead of failing the whole page.
19. **Live log contract** — `/live` is not websocket-only. It replays per-run `app.log` via `GET /api/runs/{scenario}/{run_id}/log`, then appends websocket frames if present, and only falls back to synthesized metadata/artifact lines if both are absent.

20. **Slices 22-25 complete** — incremental deployment model (ADR-0009). Single evolving scenario YAML, factory regenerates all HCL, OpenTofu handles incremental diffs. Key flags: `--no-destroy` (preserve state between runs), `--clean` (force fresh start). Mockway snapshot/restore for feedback iterations.
21. **Slices 26-29 complete** — Layer 3 real Scaleway deploy (ADR-0010, supersedes ADR-0003). Optional dual-apply: same HCL applied to both mockway and real Scaleway with separate `.tfstate` files. Layer 2 gates Layer 3.
24. **Slice 30 complete** — Layer 3 production hardening. `plan-live.txt` artifact captured per iteration. Auto-destroy on failed runs (billing protection). `scaleway_account_project` validated in generated HCL. `sandbox_project_id` config removed — project lifecycle is fully HCL-managed per ADR-0010.
25. **Slice 31 in progress** — topology derivation from raw mock state (ADR-0011). S31-T1 (docs) is done. Next ticket: S31-T2 (implement `DeriveTopology()` in `internal/harness/topology_derive.go`). Read `docs/plans/topology-derivation-plan.md` for full design including struct mapping, derivation rules, and ASCII diagrams. The topology evaluator currently expects pre-computed `connectivity`/`http_probe` maps that mockway doesn't produce — this slice fixes that.
26. **Playwright e2e tests** — 18 tests in `ui/e2e/`. Run with `make test` (includes Go unit + UI unit + Playwright). Pre-commit hook runs `make test` automatically.
27. **`make run`** — builds everything and starts the UI at `http://127.0.0.1:4173`.
22. **Primary incremental scenario** — webserver → database → Redis. This is the main use case the incremental model must support. See CONCEPT.md *Scenario evolution example* for the 3-stage YAML progression.
23. **Implementation contracts** — CONCEPT.md *Implementation Contracts (Slices 22-29)* contains 22 binding contracts covering: scenario identifier semantics, API request schemas, config vs flag precedence, auto-detection rules, snapshot/restore failure handling, artifact paths, destroy behavior matrix, probe contracts, Layer 3 credential/project/state lifecycle, UI endpoint response schemas, RunMetadata versioning, failure taxonomy, concurrency invariants, and legacy compatibility. **Read this section before modifying any Slice 22-29 code.** These are binding decisions, not suggestions.

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
  `output/<scenario>/` is the latest mutable generated IaC and is overwritten each run; immutable historical IaC snapshots now live under `.infrafactory/runs/<scenario>/<run-id>/generated/`.
- Provider schema extraction is lazy and generate-path-only:
  first `generate`/`run` generation call attempts schema extraction once (cached in runtime); `validate`/`test`/`mock` should not pay that startup cost.
- Schema extraction is best-effort:
  if `tofu init/providers schema` fails, generation proceeds without schema-enriched prompts; inspect logs for `provider_schema` skip details before debugging prompt quality.
- Preserve Slice 13-15 guarantees while fixing Slice 16 issues:
  keep deterministic JSON-line logging fields/redaction behavior, structured run feedback (`failure_class` and detailed failure context), and adaptive transport stop behavior with persisted diagnostics.
- For any future planning refinement over unfinished slices (`todo`/`blocked` backlog work):
  require the same refinement protocol: continue until two consecutive no-change passes, and explicitly record pass outcomes in both `CURRENT_TICKET.md` and `STATUS.md` for fresh-context continuity.
  `S9-T8` is now unblocked by ADR-0010; Layer 3 implementation is planned in Slices 26-29.
- Optimized post-slice review prompt (apply to each unfinished slice after implementation):
  "After completing this slice, run a dedicated review-improve pass over code, tests, docs, and artifacts. Apply any improvements that increase correctness, determinism, observability, and operator clarity. Repeat review-improve passes until two consecutive passes find no further improvements. Record each pass outcome in `STATUS.md` and `CURRENT_TICKET.md`."

### Slices 7-10 default execution constraints (completed)
- All tickets done. Preserve CLI/output contracts, golden snapshots, error taxonomy, and hermetic test defaults.
- `S9-T8` (sandbox/live deploy) is unblocked by ADR-0010 and planned for Slices 26-29.
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
  Preserve and extend the shipped `infrafactory ui` command and SvelteKit frontend without regressing API-only (`noui`) behavior, diagnostics, run history, or per-run IaC viewing.
- Plan:
  `docs/plans/web-ui-plan.md` — read Quick Reference, Prerequisites, your slice section, and Pitfalls before coding.
- Build tag rule:
  Until `ui/build/` exists (populated by `npm run build`), always use `-tags noui` for Go builds and tests. The `!noui` build (`embed.go`) requires `ui/build/` to exist — without it, `go build` fails with `pattern all:ui/build: no matching files found`.
- Dev workflow:
  Two terminals: (1) `go run -tags noui ./cmd/infrafactory ui --addr 127.0.0.1:4173` (API-only, non-API paths return 404 since assets=nil), (2) `cd ui && npm run dev` (Vite on :5173, proxies HTTP `/api` → `127.0.0.1:4173`). Websocket logs do not use the Vite proxy in dev; the browser connects directly to backend WS origin (`:4173` by default).
- Prerequisites:
  Node.js 18+ and npm 9+ required for frontend work. Not needed for `go test -tags noui ./...`.
- Test invariant:
  `go test -tags noui ./...` must pass after every commit — this is the CI gate.
- Frontend regression coverage:
  Current frontend safety net is `cd ui && npm test && npm run build`. If you add browser-only behavior that these tests cannot cover, expand the test harness rather than assuming manual testing is enough.
- Live-run debugging rule:
  If the Live page looks idle, check both sources of truth separately:
  `GET /api/runs/{scenario}/{run_id}` must return a `running` record soon after run start, and `/api/ws` may still be absent/noisy in dev. The UI should degrade to synthesized console lines from polled metadata/artifacts rather than blank idle output.
- Single new Go dependency:
  `github.com/coder/websocket` only. No external HTTP routers or logging frameworks.
- Path safety:
  All handlers serving files from disk must reject `..` segments. Test path traversal in every handler that reads from `cfg.Paths.*`.

### Slices 22-25: Incremental deployment execution constraints
- **Dependency chain**: Slice 22 → 23 → 24 → 25. Must be implemented sequentially.
- **Slice 22 (Mockway snapshot/restore)**:
  - Code lives in the separate mockway repo at `../mockway/`. Changes require `docker compose up --build -d mockway` to take effect.
  - Implementation: SQLite `VACUUM INTO` for snapshot (atomic file copy), file swap + DB re-open for restore. Reference existing handler patterns in `../mockway/handlers/`.
  - `repository.Repository` stores `*sql.DB`, `path` (DB file path for `VACUUM INTO`), and `snapshotPath` (active snapshot location). `Reset()` clears all tables and removes any active snapshot.
  - Routes are registered in `handlers/handlers.go` lines 23-25 (existing `/mock/reset`, `/mock/state` patterns). `Application` holds `*repository.Repository`.
  - Test pattern: use `testutil.NewTestServer(t)` from mockway test helpers.
  - Endpoints: `POST /mock/snapshot`, `POST /mock/restore`. Update `POST /mock/reset` to clear any active snapshot.
- **Slice 23 (InfraFactory incremental run support)**:
  - Run loop code: `internal/cli/run_command.go` — `runRunCommand` (line 36, main entry point), iteration loop at line 112, `runIteration` helper (line 336).
  - Harness code: `internal/harness/mock_deploy.go` — `MockStateClient` interface (line 11) has `Reset`, `Snapshot`, `Restore`, `State`. `MockDeployHarness.Run()` calls `h.mock.Restore(ctx)` (line 71, incremental) or `h.mock.Reset(ctx)` (line 79, clean) based on `MockDeployMode`.
  - Destruction runs inside `test_command.go` line 285 (`runtime.Deps.Destroy.Run()`). `--no-destroy` skips this via `SkipDestroy` flag passed to `executeTest`.
  - `mockwayStateClient` in `internal/cli/mockway_client.go` has `Reset()`, `Snapshot()`, `Restore()`, and `State()` methods.
  - Auto-detection logic: 3 checks — (1) `GET /mock/state` has resources, (2) `.tfstate` exists in `output/{scenario}/`, (3) run store has previous successful run. All three → incremental; any missing → clean.
  - **Critical**: `--no-destroy` must skip Layer 4 destruction AND preserve mockway state + `.tfstate`. Without this, the entire incremental model is non-functional because destruction wipes everything.
  - Output dir handling: in incremental mode, delete only `*.tf` files before writing new ones; preserve `.tfstate` and `.terraform/`. In clean mode, wipe entire `output/{scenario}/`.
  - `RunMetadata` in `internal/runstore/` has `Incremental`, `PreviousRunID`, and `Layer3Enabled` fields (optional, `omitempty`, no schema version bump — see CONCEPT.md contract #19).
  - Existing training/regression scenarios must continue passing unchanged (they auto-detect as clean).
  - Update README with `--clean` and `--no-destroy` documentation.
- **Slice 24 (Incremental E2E validation)**:
  - Create `scenarios/training/incremental-project-paris.yaml` — the primary incremental test scenario.
  - Three-stage test: Stage 1 (compute+VPC+LB, `--no-destroy`), Stage 2 (add database, `--no-destroy`), Stage 3 (add redis, `--no-destroy`). Verify incremental apply at each stage.
  - Final destruction test: run Stage 3 without `--no-destroy`, verify all resources destroyed.
  - `--clean` regression test: run with `--clean` to verify clean mode still works.
  - Update README with incremental workflow operator guide.
- **Slice 25 (Incremental UI)**:
  - Extend `POST /api/runs/{scenario}/start` to accept `no_destroy` and `clean` booleans.
  - Add `GET /api/scenarios/{scenario}/run-mode` endpoint (server-side 3-check detection).
  - Add `GET /api/runs/{scenario}/{run_id}/plan` and `/baseline` endpoints.
  - Persist `plan.txt` and `baseline_state.json` in run iteration artifacts.
  - Frontend: toggles on scenario page, mode badge + baseline panel + plan diff on Live page.
  - Update README Web UI section.
  - Preserve all existing Slice 21 contracts (websocket, live log, run history, diagnostics).

### Slices 26-29: Layer 3 real Scaleway deploy execution constraints
- **Dependency chain**: Slice 26 → 27 → 28 → 29. Must be implemented sequentially. Slice 26 can start after Slice 23 (needs `--no-destroy`). Slice 28 depends on both Slice 24 and 27.
- **ADR-0010 already exists** — do not create a new ADR. Implement the decisions documented in `docs/decisions/0010-layer3-real-scaleway-deploy.md`.
- **Slice 26 (Layer 3 harness)**:
  - **Key code location**: `run_command.go` — `detectRunMode` (line 56) handles incremental vs clean detection including Layer 3 credential validation. Layer 3 dual-apply logic is in `test_command.go` (line 285+ for mock destroy, line 299+ for sandbox destroy).
  - Dual-apply: same `.tf` files applied to both mockway and real Scaleway. Two `.tfstate` files: `terraform.tfstate` (mock, existing) and `terraform-live.tfstate` (real).
  - Layer 3 only runs if `validation.layers.sandbox_deploy.enabled: true` AND Layers 1+2 pass.
  - Real Scaleway apply: run `tofu apply` without `SCW_API_URL` override (lets provider use real Scaleway API). Mock apply continues using `SCW_API_URL=http://127.0.0.1:8080`.
  - Credential validation: require `SCW_ACCESS_KEY`/`SCW_SECRET_KEY` env vars when Layer 3 enabled. Fail with clear error if missing.
  - Destroy behavior matrix: `--no-destroy` keeps everything; no `--no-destroy` + converges → destroy both; no `--no-destroy` + fails → auto-destroy real.
  - Prompts: when Layer 3 enabled, guide the LLM to include `scaleway_account_project` resource so the factory bootstraps its own project.
  - Layer 3 failures feed back into the structured failure JSON for the next iteration.
  - Integration tests are opt-in (require real credentials).
  - Update README with Layer 3 config and operator docs.
- **Slice 27 (Real probes)**:
  - Replace graph-query acceptance criteria with real network probes when Layer 3 is enabled: `connectivity` (real `nc`/`curl`), `http_probe` (real HTTP), `dns_resolution` (real `dig`/`nslookup`).
  - Fall back to Layer 2 graph queries when Layer 3 is disabled.
  - Add probe timeout/retry config to `infrafactory.yaml` — real probes need time for DNS propagation, service startup.
  - Update README with probe config and criteria support status.
- **Slice 28 (Layer 3 incremental E2E)**:
  - Run the Slice 24 pattern (3-stage webserver → database → redis) against real Scaleway with Layer 3 enabled.
  - Opt-in test requiring real Scaleway credentials and incurring real costs.
- **Slice 29 (Layer 3 UI)**:
  - Layer 3 toggle on scenario page with credential status indicator.
  - Layer 3 progress on Live page alongside Layer 2.
  - `GET /api/scenarios/{scenario}/layer3-status` endpoint for readiness checks.
  - `POST /api/runs/{scenario}/start` accepts `layer3_enabled` boolean.
  - Update README Web UI section.

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
