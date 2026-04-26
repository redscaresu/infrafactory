# ROADMAP

This roadmap tracks durable milestones. It avoids date-based status snapshots that become stale quickly.
This file is intentionally high-level and mostly stable; day-to-day execution tracking belongs in `BACKLOG.md` and `STATUS.md`.

## Guiding constraints

- Keep the CLI runnable at all times.
- Build in vertical slices.
- Add focused tests with each behavioral change.
- Prefer deterministic behavior and explicit contracts.
- Slice closure review protocol (optimized prompt):
  After each slice is implemented, run a dedicated review-improve pass over code, tests, docs, and artifacts; apply any improvements that increase correctness, determinism, observability, and operator clarity; repeat until two consecutive passes find no further improvements; record each pass outcome in `STATUS.md` and `CURRENT_TICKET.md`.

## Milestones

1. Slice 1: Bootstrap + parse/validate
- Wire commands: `init`, `generate`, `validate`, `test`, `run`, `mock start`.
- Implement `internal/config` loader + validation.
- Implement `internal/scenario` loader + JSON Schema validation.

2. Slice 2: Generator pipeline
- Define `SeedGenerator` interface.
- Implement 3-phase generation flow and prompt rendering.
- Implement robust `# File:` output parser with tests.

3. Slice 3: Static harness
- Run `tofu init/validate/plan/show -json`.
- Evaluate OPA policies against plan JSON.
- Return structured failure output.

4. Slice 4: Mock deploy harness
- Apply against Mockway.
- Run topology checks and state policy checks.

5. Slice 5: Destroy + run history
- Run `tofu destroy`.
- Verify no orphaned resources.
- Persist run/iteration data in run store.

6. Slice 6: Convergence logic
- Implement feedback loop and stuck detection with signature-level specificity.
- Add criteria-only holdout flow.

7. Slice 7: CLI orchestration
- Wire command adapters end-to-end across generate/validate/test/run/mock start.
- Freeze command/output contracts.
- Add hermetic and opt-in real-tool smoke coverage.

8. Slice 8: Developer experience
- Add make-based local workflow automation for dependencies/tests/smoke/cleanup.
- Keep default paths hermetic and make real-tool smoke opt-in.
- Document canonical developer commands.

9. Slice 9: Criteria-complete orchestration
- Wire default runtime generator behavior for `generate`/`run`.
- Expand scenario runtime model so criteria and holdout routing data are available to CLI orchestration.
- Execute scenario acceptance criteria in `test`/`run` (topology + state policy + holdout flow).
- Define and enforce criteria support/defer matrix for unsupported sandbox-only checks in current slices.
- Honor validation layer enable/disable flags consistently in CLI orchestration.
- Expand mock command lifecycle operations.
- Sandbox/live deploy layer defaults to disabled; Layer 3 real deploy is planned in Slices 26-29 (ADR-0010).

10. Slice 10: Reliability and contract hardening
- Freeze command/output contracts via golden snapshots and schema assertions.
- Normalize CLI error taxonomy and deterministic failure messaging.
- Version run artifact schema with backward-compatible readers.
- Add idempotency/retry safety and performance regression guardrails.
- Finalize permanent sandbox/live deploy block governance docs and ADR.

11. Slice 11: Generator transport integration
- Implement concrete generator transport wiring for `claude -p` and OpenRouter.
- Keep generation deterministic via strict prompt/input/output contracts and parser reuse.
- Add credential-safety guardrails so transport errors/logs do not leak secrets.
- Use lazy provider-schema extraction (`tofu providers schema -json`) to enrich phase 2/3 prompts with authoritative resource attributes when available, without blocking non-generate commands.
- Add hermetic transport-adapter tests and opt-in smoke tests for real transports.
- Preserve existing CLI/output contracts and failure taxonomy while replacing default transport stubs.

12. Slice 12: Feedback-driven regeneration hardening
- Ensure `run` iteration-N generation receives structured failures from iteration N-1.
- Reduce heuristic post-processing in favor of model-corrected regeneration informed by concrete harness failures.
- Strengthen run-loop convergence quality by improving failure payload fidelity and prompt integration.
- Add focused regression tests proving feedback is injected and iteration metadata is preserved.
- Keep one explicit run control:
  - `agent.repair_iterations_max` (+ CLI `--repair-iterations-max`) for failure-triggered retry budget with model feedback.
- Ensure failed iterations emit deterministic, structured failure summaries to app logs for each pass.
- Ensure terminal stop signaling is deterministic and non-duplicative with one canonical reason (`target_reached`, `repair_budget_exhausted`, `stuck`).
- Apply slice-closure review protocol before marking Slice 12 complete.

13. Slice 13: Full application logic logging and observability
- Define a stable application logging contract (fields, levels, redaction, deterministic formatting).
- Define deterministic log destinations for operators and automation (stderr summary + per-run artifact log path).
- Instrument command orchestration paths so generation/validation/test/run decisions are fully traceable.
- Ensure per-pass and per-stage failures are logged with run/iteration context and actionable details.
- Include failure-class context in run-loop observability (IaC-validation vs transport/runtime vs orchestration-control).
- Preserve secret-safety/redaction guarantees while increasing observability depth.
- Add focused tests (and where needed golden fixtures) to freeze logging behavior and prevent regressions.
- Apply slice-closure review protocol before marking Slice 13 complete.

14. Slice 14: High-fidelity run feedback payloads for model-guided fixes
- Define a strict feedback contract so iteration `N+1` receives detailed failure context from iteration `N`, not only coarse command-level errors.
- Reuse structured validate/test failure output in `run` feedback payload generation to preserve stage/check/policy/resource detail.
- Ensure generator transport/parse failure classes are represented distinctly so the model can differentiate IaC defects from transport/runtime defects.
- Enforce deterministic terminal-control signaling so one stop event emits one canonical control reason in feedback/output.
- Add regression tests that fail if feedback payloads regress back to generic markers like `validation failed`.
- Document operator and fresh-context workflows for inspecting feedback payload quality from run artifacts.
- Apply slice-closure review protocol before marking Slice 14 complete.

15. Slice 15: Adaptive retry and transport-resilience policy
- Define retry-governance rules that distinguish model-correctable IaC failures from non-correctable transport/runtime failures.
- Prevent unproductive regeneration loops when failures are dominated by transport issues (timeouts, killed subprocess, dependency outages).
- Add deterministic retry controls per failure class (for example bounded transport retry budget/backoff and explicit stop reasons).
- Persist richer transport diagnostics (phase, timeout, exit signal, stderr summary, duration) in run artifacts for post-mortem and prompt tuning.
- Surface operator-facing remediation guidance (for example timeout tuning vs scenario/code changes) in deterministic output and runbook docs.
- Add focused tests proving transport-dominated runs stop with actionable reasons rather than generic max-iteration churn.
- Apply slice-closure review protocol before marking Slice 15 complete.

16. Slice 16: Issue-backlog remediation and robustness hardening
- Remediate open issues in `ISSUES.md` (context propagation, bounded response reads, env override determinism, schema-loading guarantees, and policy correctness gaps).
- Keep remediation incremental and ticketed so each issue class has clear acceptance tests and deterministic behavior.
- Remove stale/dead code paths and no-op branches that reduce maintainability.
- Align policy intent/messages with actual checks to avoid misleading compliance signaling.
- Ensure fresh-context startup docs and operator guidance reflect post-remediation behavior.
- Apply slice-closure review protocol before marking Slice 16 complete.

17. Slice 18: Expand mockway coverage to remaining Scaleway API surfaces
- Create standalone scenarios for each uncovered Scaleway API surface and run them through infrafactory to discover mockway gaps.
- Fix mockway (add missing handlers/tables) and infrafactory (extend scenario schema, mappings, prompts) iteratively until each scenario passes on first iteration.
- Split work by API surface: K8s standalone (Tier 1), IAM standalone (Tier 1), Container Registry (Tier 2), Redis (Tier 2), Composite (depends on all).
- Tier 1 (`S18-T1`, `S18-T2`) exercises existing mockway handlers and can run in parallel.
- Tier 2 (`S18-T3`, `S18-T4`) requires new mockway services and can run in parallel after Tier 1.
- Composite (`S18-T5`) validates all services together after Tier 1+2 are green.
- Schema extensions required: add `iam`, `registry`, `redis` to `scenario.schema.json` resources (currently `additionalProperties: false`); add corresponding Go struct fields in `scenario.go`; add redis size mappings to `mappings.yaml`.
- Out of scope: Serverless/Containers, S3 (off-the-shelf mock follow-up).
- Execution protocol: write scenario → `infrafactory run` → diagnose failures → fix mockway or infrafactory → write regression test → rebuild Docker → rerun until first-iteration pass. No confirmation prompts — proceed autonomously.
- Apply slice-closure review protocol before marking Slice 18 complete.

18. Slice 19: Reliability review and hardening of Slice 18
- Review all Slice 18 code in both mockway and infrafactory for bugs, edge cases, error handling, and correctness issues.
- Fix all identified issues with regression tests in both codebases.
- Run all Slice 18 scenarios (standalone + composite) repeatedly until every one passes `infrafactory run` on first iteration with no regressions.
- Execute autonomously without human interaction — diagnose, fix, test, rebuild Docker, rerun until green.
- Apply slice-closure review protocol before marking Slice 19 complete.

19. Slice 20: Scenario combination expansion
- Create 6 new training scenarios that exercise untested parameter combinations across all schema parameters and mockway services.
- Coverage targets: mysql engine, medium/large/xlarge sizes, high availability, private LB, multi-backend LB, tcp protocol, K8s/Redis/database overrides, public registry, selective IAM flags.
- Execution protocol: write scenario → `infrafactory run` → diagnose failures → fix mockway or infrafactory → write regression test → rebuild Docker → rerun until first-iteration pass.
- Tier 1 (independent): S20-T1, S20-T2, S20-T3, S20-T5.
- Tier 2 (after any Tier 1): S20-T4, S20-T6.
- Apply slice-closure review protocol before marking Slice 20 complete.

20. Slice 21: Web UI dashboard
- Add a web dashboard (`infrafactory ui`) for browsing scenarios, watching runs in real time, viewing run history, editing scenario YAML, and viewing generated .tf files.
- Tech stack: SvelteKit (adapter-static) frontend embedded in Go binary via `go:embed`; Go `net/http` backend reusing existing internal packages; WebSocket (`github.com/coder/websocket`) for real-time streaming.
- New `internal/api/` package: HTTP server, REST handlers, WebSocket hub, SPA fallback, `WebSocketSink` plugging into existing `AppLogger.sinks`.
- New `ui/` directory: SvelteKit SPA with scenario sidebar, pipeline visualization, iteration timeline, log stream, YAML editor, HCL viewer.
- New `infrafactory ui --addr 127.0.0.1:4173` Cobra command via `NewRootCmd(opts ...RootOption)` functional options pattern.
- Extract `executeRunLoop()` from `runRunCommand` to decouple run execution from Cobra for API reuse.
- Add `ListScenarios()` to `runstore` for run history browsing.
- Build integration: `Makefile` targets (`ui-install`, `ui-build`, `ui-dev`, `ui-clean`), `go:embed` with `noui` build tag fallback, GoReleaser `before.hooks`.
- Implementation sub-slices:
  - SUi-1: Skeleton server + static asset embed.
  - SUi-2: Scenario browser + sidebar.
  - SUi-3: Run history browser.
  - SUi-4: Generated code viewer.
  - SUi-5: WebSocket infrastructure + log streaming.
  - SUi-6: Live run execution + pipeline visualization.
  - SUi-7: Scenario YAML editor.
  - SUi-8: Build pipeline + polish.
- Follow-up hardening completed after Slice 21 closure:
  - backend diagnostics endpoint/page (`/api/diagnostics`, `/diagnostics`)
  - run-scoped immutable IaC history under `.infrafactory/runs/<scenario>/<run-id>/generated/`
  - per-iteration immutable IaC snapshots under `.infrafactory/runs/<scenario>/<run-id>/iterations/<n>/generated/`
  - run detail page acts as the per-run IaC viewer
  - diff view between snapshots, richer syntax-highlighted IaC viewer, IaC-only and full-artifact downloads, and Run History newest-first ordering/filtering/direct `IaC` / `Live` actions
- Security: path traversal validation on PUT, single concurrent run mutex, credential redaction, slow-client message dropping.
- Testing: `httptest` unit tests for REST handlers, WebSocket hub tests, frontend logic/build checks (`cd ui && npm test && npm run build`), and `go test -tags noui ./...` must not break existing CLI.
- Apply slice-closure review protocol before marking Slice 21 complete.

21. Slice 22: Mockway snapshot/restore API
- Add `POST /mock/snapshot` endpoint — copies current SQLite state to a temp file. Returns snapshot ID. Only one active snapshot supported (v1).
- Add `POST /mock/restore` endpoint — restores SQLite state from the most recent snapshot. Fails 404 if no snapshot exists.
- Update `POST /mock/reset` to also clear any active snapshot.
- Implementation: SQLite `VACUUM INTO` for snapshot (atomic copy), file swap + re-open for restore.
- Add integration tests: snapshot → mutate → restore → verify original state; restore without snapshot → 404; reset clears snapshot.
- No changes to existing Mockway handlers or state model.
- Apply slice-closure review protocol before marking Slice 22 complete.

22. Slice 23: InfraFactory incremental run support
- Add `--clean` flag to `infrafactory run` CLI command. Default: auto-detect mode.
- Add `--no-destroy` flag to skip Layer 4 destruction and preserve mockway state + `.tfstate` for the next incremental run. Without this, destruction wipes everything after convergence, making incremental detection impossible after a successful run.
- Add incremental detection logic to run loop: check three conditions — (1) `GET /mock/state` has existing resources, (2) `.tfstate` exists in `output/{scenario}/`, (3) run store has a previous successful run for the scenario. All three present → incremental; any missing → clean.
- Add mockway client methods: `Snapshot()` and `Restore()` calling the new Mockway endpoints (Slice 22).
- Update `MockDeployHarness`:
  - Incremental mode: call `Restore()` instead of `Reset()` between feedback iterations.
  - Clean mode: call `Reset()` (current behavior).
- Update run loop entry:
  - Incremental: call `Snapshot()` at run start to save baseline.
  - Clean: call `Reset()` + delete `.tfstate` at run start (current behavior).
- Update `RunMetadata` with `incremental: bool` and optional `previous_run_id: string` fields.
- Preserve `.tfstate` between runs when not using `--clean` (skip deletion at run start in incremental mode).
- Add unit tests: auto-detection logic (mock state present/absent × `.tfstate` present/absent × previous run present/absent), `--clean` override, snapshot/restore client calls, metadata fields, output dir cleanup preserving `.tfstate`/`.terraform/` in incremental mode.
- Add integration test: run scenario with `--no-destroy` → verify convergence and state persists → run same scenario again (auto-detects incremental) → verify `tofu plan` shows no changes for existing resources.
- Existing training/regression scenarios must continue to pass unchanged.
- Update README: document `--clean` and `--no-destroy` flags in the CLI reference and config reference sections.
- Apply slice-closure review protocol before marking Slice 23 complete.

23. Slice 24: Incremental deployment E2E validation
- Create a single evolving scenario file (`scenarios/training/incremental-project-paris.yaml`) that exercises the incremental path end-to-end.
- Test stages (each stage edits the same file, then re-runs with `--no-destroy`):
  - Stage 1: scenario has compute + VPC + LB only. Run with `--no-destroy` (no prior state → clean auto-detected), verify convergence. State persists after run.
  - Stage 2: edit scenario to add `database:` block + connectivity criteria. Run with `--no-destroy` (auto-detects incremental because mockway has state from Stage 1). Verify:
    - `tofu plan` shows only database resources as new (web server/VPC/LB unchanged in state).
    - Topology evaluator validates cross-resource connectivity (database on same private network as compute).
    - OPA policies pass for the full topology.
  - Stage 3: edit scenario to add `redis:` block. Run with `--no-destroy` (incremental). Same verification pattern.
- Final destruction test: run Stage 3 scenario WITHOUT `--no-destroy`. Verify Layer 4 destroys all resources (compute + db + redis + VPC + LB). Next run auto-detects as clean.
- Add a `--clean` regression test: run Stage 2 scenario content with `--clean`, verify it still converges (full regeneration from empty state).
- Document the incremental workflow in README (operator guide section).
- Apply slice-closure review protocol before marking Slice 24 complete.

24. Slice 25: Incremental deployment UI
- Add run configuration controls to the scenario page:
  - `--no-destroy` toggle (default off). When on, `POST /api/runs/{scenario}/start` passes `no_destroy: true` to the run loop.
  - `--clean` toggle (default off). When on, forces clean mode regardless of auto-detection.
  - Show auto-detected run mode indicator before run starts: "Incremental (prior state detected)" or "Clean (no prior state)".
- Add `GET /api/scenarios/{scenario}/run-mode` backend endpoint that performs the 3-check detection server-side (mockway state + `.tfstate` + previous successful run) and returns `{ "mode": "incremental" | "clean", "reason": "..." }`. Frontend queries this on scenario page load and after each run completes.
- Add `GET /api/runs/{scenario}/{run_id}/plan` backend endpoint returning raw `tofu plan` output for the latest iteration.
  - Persist `tofu plan` stdout as `plan.txt` in iteration artifacts during the run loop.
  - Return plain text, frontend renders in a monospace pre-formatted block.
- Add `GET /api/runs/{scenario}/{run_id}/baseline` backend endpoint returning mockway state snapshot taken at run start.
  - Persist baseline snapshot as `baseline_state.json` in run artifacts when an incremental run starts.
  - Return the resource list grouped by type (same format as `GET /mock/state`).
- Update the Live page:
  - Add a run mode badge: "INCREMENTAL" or "CLEAN" prominently displayed near the run status.
  - Add a collapsible "Baseline State" panel showing resources that existed before this run. Hidden when run mode is clean (no baseline). Panel shows resource type, name/ID, and count per type.
  - Add a "Plan Diff" tab/panel showing the raw `tofu plan` output for the current/latest iteration. Updated after each iteration's Layer 1 completes.
- Update `POST /api/runs/{scenario}/start` to accept optional `no_destroy` and `clean` boolean fields in the request body, passed through to the run loop.
- Add frontend tests: toggle state management, mode indicator logic, baseline panel show/hide, plan text rendering.
- Add backend tests: plan artifact persistence and retrieval, baseline snapshot persistence, start-run with flags.
- Update README Web UI section: document incremental run controls (toggles, mode indicator, baseline panel, plan diff).
- Apply slice-closure review protocol before marking Slice 25 complete.

25. Slice 26: Layer 3 real Scaleway deploy harness
- Implement ADR-0010 (already accepted, supersedes ADR-0003): wire `validation.layers.sandbox_deploy.enabled` to the dual-apply harness.
- Implement dual-apply architecture in the run loop:
  - After Layer 2 (mockway) passes, run `tofu apply` against real Scaleway using the same `.tf` files.
  - Manage two `.tfstate` files: `terraform.tfstate` (mock, existing) and `terraform-live.tfstate` (real).
  - Pass real Scaleway API endpoint (no `SCW_API_URL` override) for Layer 3 apply.
  - Layer 3 failures included in structured failure JSON, fed back to next iteration.
- Implement Layer 3 destroy behavior:
  - `--no-destroy`: keep real resources regardless of outcome.
  - No `--no-destroy` + converges: `tofu destroy` against real Scaleway after verification.
  - No `--no-destroy` + fails: auto-destroy real resources.
- Update config: `validation.layers.sandbox_deploy.enabled` defaults to `false`. When `true`, requires `SCW_ACCESS_KEY`/`SCW_SECRET_KEY` env vars with real permissions. Fail with clear error if enabled but credentials missing.
- Update prompts: when Layer 3 enabled, include `scaleway_account_project` resource in generation guidance so the factory bootstraps a fresh project.
- Add unit tests: dual-apply state file management, Layer 3 gating (only runs if Layers 1+2 pass), destroy behavior matrix (`--no-destroy` × converge/fail), credential validation.
- Add integration test (requires real credentials, opt-in): run a minimal scenario (single compute instance) with Layer 3 enabled, verify real resource creation and destruction.
- Update README: document Layer 3 configuration (`validation.layers.sandbox_deploy.enabled`, required env vars), destroy behavior, and project bootstrap in the config reference and operator guide sections.
- Apply slice-closure review protocol before marking Slice 26 complete.

26. Slice 27: Layer 3 real probes and acceptance criteria
- Implement real network probes for Layer 3:
  - `connectivity`: real `nc`/`curl` probe between resources (replaces graph query from Layer 2).
  - `http_probe`: real HTTP request to endpoint (replaces graph query from Layer 2).
  - `dns_resolution`: real `dig`/`nslookup` against configured DNS (previously deferred as sandbox-only).
- Update acceptance criteria evaluation to run real probes when Layer 3 is enabled, graph queries when Layer 2 only.
- Add probe timeout and retry configuration to `infrafactory.yaml` (real probes may need time for DNS propagation, service startup).
- Add tests: probe execution against real endpoints (opt-in), fallback to graph queries when Layer 3 disabled.
- Update README: document probe timeout/retry configuration and updated criteria support status (connectivity, http_probe, dns_resolution now functional with Layer 3).
- Apply slice-closure review protocol before marking Slice 27 complete.

27. Slice 28: Layer 3 incremental E2E validation
- Run the incremental deployment workflow (Slice 24 pattern) against real Scaleway:
  - Stage 1: compute + VPC + LB with `--no-destroy` + Layer 3 enabled. Verify real resources created.
  - Stage 2: add database. Verify `tofu plan` shows only database as new against real Scaleway state.
  - Stage 3: add redis. Same pattern.
  - Final: run without `--no-destroy`. Verify real resources destroyed including the bootstrapped project.
- Validate real probes: `connectivity` (compute → database on port 5432), `http_probe` (LB on port 80).
- This is an opt-in E2E test requiring real credentials and incurring real Scaleway costs.
- Apply slice-closure review protocol before marking Slice 28 complete.

28. Slice 29: Layer 3 UI integration
- Update scenario page:
  - Add "Layer 3 (Real Scaleway)" toggle. When enabled, show credential status (env vars detected or missing).
  - Mode indicator shows "Mock + Real" when Layer 3 enabled.
- Update Live page:
  - Show Layer 3 status alongside Layer 2 in iteration progress (Layer 2: pass → Layer 3: applying...).
  - Show real resource creation/destruction progress.
  - Show real probe results for connectivity/http_probe/dns_resolution criteria.
- Update `POST /api/runs/{scenario}/start` to accept `layer3_enabled` boolean.
- Add `GET /api/scenarios/{scenario}/layer3-status` endpoint: checks `sandbox_deploy.enabled` config + credential env vars, returns readiness status.
- Add frontend + backend tests.
- Update README Web UI section: document Layer 3 toggle, credential status indicator, and real probe results display.
- Apply slice-closure review protocol before marking Slice 29 complete.

30. Slice 30: Layer 3 production readiness
- Capture `plan-live.txt` artifact during sandbox deploy (`tofu plan` before `tofu apply`).
- Auto-destroy real resources on failed runs (billing protection, Contract #14 destroy matrix).
- Validate generated HCL includes `scaleway_account_project` when Layer 3 enabled (Contract #12).
- Verify holdout checks execute Layer 3 dual-apply pattern (Contract #10).
- Close `S9-T8` governance ticket (superseded by Slices 26-30).
- Design reference: `docs/plans/layer3-production-plan.md`.
- Apply slice-closure review protocol before marking Slice 30 complete.

31. Slice 31: Topology derivation from raw mock state (ADR-0011)
- Derive `connectivity` and `http_probe` maps from raw mockway/fakegcp resource state.
- Auto-detect raw state in `EvaluateTopology` and call `DeriveTopology` transparently.
- Rules: LB frontend+backend+IP chain for http_probe; shared private network for connectivity.
- Unit tests with fixture data covering all derivation rules and edge cases.
- Playwright e2e test verifying topology results visible on Live page.
- Integration verification: all 12 training scenarios pass with derived topology.
- Design reference: `docs/plans/topology-derivation-plan.md`.
- Apply slice-closure review protocol before marking Slice 31 complete.

32. Slice 32: Dynamic pitfalls by cloud provider
- Externalize provider pitfalls from hardcoded prompt templates into `pitfalls/{cloud}.yaml` files.
- Load pitfalls at runtime based on scenario's `cloud` field and inject via `{{.Pitfalls}}` template variable.
- Organized by provider: scaleway.yaml, gcp.yaml, aws.yaml, common.yaml.
- Each pitfall has `source: static` (manual) or `source: learned` (auto-discovered from feedback).
- Design doc for future auto-learning from run feedback patterns.
- Design reference: `docs/plans/dynamic-pitfalls-plan.md`.

33. Slice 33: Cross-repo E2E test
- Go integration test that starts mockway from source and runs `infrafactory run` against scenarios end-to-end.
- Catches cross-repo bugs (field naming, missing response fields, port conventions).
- Tests for web-app-paris (simple) and full-stack-paris (all resource types).
- Gated by env var so normal `go test` is unaffected.
- Design reference: `docs/plans/slices-33-39-plan.md`, ADR-0013.

34. Slice 34: Pitfall learning from failed runs
- Detect oscillation patterns (same failure signature alternating across iterations).
- Learn pitfalls from oscillation failures, not just successful self-correction.
- Extends the dynamic pitfalls auto-learning (Slice 32) to cover more learning opportunities.

35. Slice 35: Better http_probe feedback
- Enrich topology derivation with diagnostic detail when http_probe is false.
- Failure messages include the specific missing link (no frontend, no backend, no IP).
- Helps the LLM fix the right thing on retry.

36. Slice 36: GCP support
- Add `cloud: gcp` scenario type using fakegcp mock server.
- Per-cloud prompt templates (`prompts/gcp/`), GCP topology derivation, GCP pitfalls.
- Training scenarios: basic VM + network, GKE cluster, Cloud SQL.
- Validates the cloud-agnostic architecture with a real second provider.
- Design reference: `docs/plans/slices-33-39-plan.md`, ADR-0013.

37. Slice 37: Pitfalls UI
- `/pitfalls` page showing all pitfalls with source badges (static/learned), resource, and provider.
- Edit form for curating learned pitfalls without editing YAML files.
- GET/PUT API endpoints for pitfalls by provider.

38. Slice 38: Run comparison
- UI page that diffs two runs side-by-side using existing per-run IaC snapshots.
- Dual-pane diff viewer with syntax highlighting and file list sidebar.
- Compare endpoint returns file-level unified diffs.

39. Slice 39: Real-time scenario validation in UI
- Validate scenario YAML as user types (debounced 500ms).
- POST /api/scenarios/validate endpoint (validates without saving).
- Inline error display with line numbers below textarea.

40. Slice 40: Visual UI regression testing
- Playwright screenshot baselines for all pages (home, scenario, live, runs, diagnostics).
- Visual regression tests with pixel diff threshold (fail on unexpected layout changes).
- Functional spot-checks: verify data rendering, YAML content, run metadata, iteration timeline.
- Error state coverage: empty states, 404 pages, API errors, missing scenarios.

## Near-term execution order

1. Keep completed slices (1-32) stable and regression-green.
2. Slices 33-39 are all independent and can be executed in any order.
3. Slice 33 (cross-repo e2e) and Slice 35 (http_probe feedback) are quick wins with high impact on developer productivity.
4. Slice 36 (GCP) is the largest slice (13 tickets T0-T12). T0 first in fakegcp; T1/T2/T4/T6/T7/T8 parallel; T3/T5/T9 after deps; T10 after T2+T3+T6+T7; T11/T12 last.
5. Pipeline consistently achieves first-iteration pass (12/12 training scenarios); monitor for regressions.

## Live progress tracking

Use `BACKLOG.md` and `STATUS.md` for day-to-day progress; keep this file focused on stable milestones and sequencing.
