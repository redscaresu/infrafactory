# STATUS

Last updated: 2026-03-14

## Current phase
- Active milestone: Slices 22-29 — Incremental deployment model + Layer 3 real Scaleway deploy.
- Slices 1-21 complete. All 12 training scenarios pass `infrafactory run` on first iteration.
- All Go test suites green under `noui` gate (`go test -tags noui ./...`, `check_all.sh`).
- ADRs 0009 (incremental deployment) and 0010 (Layer 3, supersedes ADR-0003) accepted. Documentation complete. Implementation next.

## In progress
- No active unblocked implementation tickets.

## Known blockers
- None. `S9-T8` (sandbox/live deploy) is unblocked — ADR-0010 supersedes ADR-0003.

## Next actions
1. Slice 22: Mockway snapshot/restore API (`POST /mock/snapshot`, `POST /mock/restore`). No deps — start here.
2. Slice 23: InfraFactory `--clean`/`--no-destroy` flags + incremental auto-detection. Depends on Slice 22.
3. Slice 24: Incremental E2E (webserver → database → redis 3-stage test). Depends on Slice 23.
4. Slice 25: Incremental UI (toggles, mode badge, baseline panel, plan diff). Depends on Slice 24.
5. Slice 26: Layer 3 dual-apply harness. Can start after Slice 23. Parallel with Slice 24-25.
6. Slices 27-29: Layer 3 probes, incremental E2E, UI. Sequential after Slice 26.

## Update policy
- Update at end of each meaningful coding session.
- Keep concise and factual.
- Move old detail to `docs/status/ARCHIVE.md`.
- Put durable architecture decisions in ADRs and `CONCEPT.md`.
- Keep startup/read-order instructions only in `SESSION_START.md` to avoid duplication.

## Recent updates
- **Run viewer fallback and run-date follow-up**:
  - Diagnosed the blank IaC viewer against the real local runstore: the affected runs had no persisted `generated/` snapshot directories even though `output/<scenario>/` still contained IaC.
  - Added a run-viewer fallback so older runs without stored snapshots show the current scenario output with an explicit warning instead of an empty preview.
  - Added a `Started` column to `/runs`.
  - Added frontend regression coverage for run-date formatting and re-verified the frontend build/tests.
- **Run diff view and full artifact download follow-up**:
  - Added `GET /api/runs/{scenario}/{run_id}/artifacts.zip` to download the full run artifact directory, not only IaC files.
  - Upgraded the run detail page with snapshot-to-snapshot diffing and a second download action for the full archive.
  - Replaced the lightweight IaC tokenizer with a richer stateful highlighter that understands attributes, functions, interpolation, heredocs, and block comments.
  - Added backend regression tests for full-artifact archive contents and frontend helper tests for diff generation, archive URLs, snapshot option selection, and richer highlighting.
  - Re-ran Go tests after one transient WebSocket timeout in `internal/api`; the package passed on immediate rerun and the final full hygiene pass was green.
- **Per-iteration IaC snapshots, highlighting, and bundle download**:
  - Persisted immutable IaC snapshots under `.infrafactory/runs/<scenario>/<run_id>/iterations/<n>/generated/` in addition to the run-final generated set.
  - Added run API endpoints for iteration listing and iteration-scoped generated-file reads plus a zip bundle download endpoint for run IaC history.
  - Upgraded the run detail page with snapshot selection, syntax-highlighted IaC preview, and a download button for the run bundle.
  - Added runstore/API/CLI regression tests for iteration snapshot persistence and bundle generation, plus frontend helper tests for highlighting and bundle URLs.
  - Fixed a Makefile `.PHONY` parse bug so `make ui-build` works again.
  - Built embedded UI assets, launched the app against a synthetic runstore, and verified:
    - `/api/runs/demo-scenario/run-123/iterations` returned the iteration list
    - `/api/runs/demo-scenario/run-123/iterations/2/files/main.tf` returned the iteration snapshot IaC
    - `/api/runs/demo-scenario/run-123/bundle.zip` contained both final and per-iteration IaC paths
    - `/runs/demo-scenario/run-123` served the run detail page with `200`
- **Per-run IaC viewer and regression test follow-up**:
  - Persisted generated IaC under runstore paths (`.infrafactory/runs/<scenario>/<run_id>/generated/...`) instead of relying only on mutable scenario output directories.
  - Added run-scoped IaC API endpoints under `/api/runs/{scenario}/{run_id}/files` and `/api/runs/{scenario}/{run_id}/files/{path...}`.
  - Upgraded the run detail page into a per-run IaC viewer with file list, code preview, and a Live link.
  - Added UI mode/backend version markers in the sidebar and Run History row actions for `IaC` and `Live`.
  - Added frontend logic tests for Live selection, Run History filtering, and failure hint derivation.
  - Added backend regression tests for generated-file persistence/read/list/traversal, diagnostics success paths, and UI starter async lifecycle.
  - Launched a temporary live server against a synthetic runstore and verified:
    - `/api/diagnostics` returned ready Claude runtime status
    - `/api/runs/demo-scenario/run-123/files` returned the stored IaC file list
    - `/api/runs/demo-scenario/run-123/files/main.tf` returned stored IaC content
    - `/runs`, `/runs/demo-scenario/run-123`, and `/diagnostics` served the embedded app shell with `200`
  - Review pass 1 found durable doc gaps in README/BACKLOG/ROADMAP/plan notes and those were updated.
  - Review pass 2 found no further improvements.
  - Review pass 3 found no further improvements.
  - Final doc sync pass updated `SESSION_START.md`, `CONCEPT.md`, and `internal/runstore/doc.go` so fresh sessions and architecture docs match the shipped UI and run-scoped IaC history.
- **Web UI diagnostics and run-ops follow-up**:
  - Added `GET /api/diagnostics` with backend generator readiness checks for `claude-code` and `openrouter`.
  - Added `/diagnostics` UI page plus sidebar access, and linked Live failure hints to diagnostics.
  - Improved `/live` recovery to prefer the latest run for the selected scenario before falling back globally.
  - Added Run History filtering by scenario/run ID/terminal reason and status.
  - Added regression coverage for diagnostics contract, incomplete run directories breaking `/api/runs`, and UI starter context/preflight failures previously observed during manual testing.
- **Web UI run UX follow-up**:
  - Added UI run preflight checks so `POST /api/runs/{scenario}/start` fails immediately with a clear message when `claude` is missing from `PATH` or `OPENROUTER_API_KEY` is unset.
  - Updated the frontend API client to unwrap JSON `{error: ...}` responses into readable UI errors.
  - Reworked the Live page to show run failure cards with stage/check/command/detail instead of only raw iteration JSON.
  - Added focused `uiRunStarter` preflight tests and re-verified with `bash scripts/check_all.sh`.
- **Web UI DX hardening follow-up (post Slice 21)**:
  - Fixed frontend dependency conflict by aligning Vite to v6 (`npm install` now succeeds).
  - Added Docker Compose UI dev services: `infrafactory-api` and `infrafactory-ui` (profile `ui`).
  - Added Make targets: `ui-stack-up`, `ui-stack-logs`, `ui-stack-down`.
  - Fixed embedded asset handoff: `make ui-build` now copies built assets to `cmd/infrafactory/ui/build` for `go:embed`.
  - Updated `scripts/check_all.sh` to gate untagged tests on embedded asset path existence.
  - Updated README Web UI instructions for local and Docker workflows.
- **Slice 21 complete (SUi-1..SUi-8)**:
  - Added backend API package surfaces for scenarios, runs, output, config, and run start.
  - Added runstore capabilities: `ListScenarios`, `ReadIterationArtifact`, `RunMetadata.TerminalReason`.
  - Added WebSocket hub/client/sink and `/api/ws` streaming endpoint.
  - Added UI run starter wiring from `infrafactory ui` to real run execution with single-run guard and conflict handling.
  - Added SvelteKit dashboard routes for scenario browsing/editing, run history/detail, output viewing, and live log stream.
  - Added/expanded API and runstore test coverage for success/error/traversal/concurrency contracts.
  - Added GoReleaser pre-build hook for UI (`make ui-build`) and README Web UI documentation.
  - Added `github.com/coder/websocket` dependency.
- **UI follow-up hardening (Live run visibility fix)**:
  - Fixed a real Live page regression where runs launched from the scenario page could remain visually idle because `run.json` was only written at terminal completion.
  - `internal/cli/run_command.go` now persists initial run metadata with `status: running` before generator execution starts, so `/api/runs/{scenario}/{run_id}` becomes visible immediately.
  - `ui/src/routes/live/+page.svelte` now synthesizes console lines from polled run metadata and iteration artifacts when websocket delivery is absent, instead of staying stuck on `No active run.` or an empty waiting state.
  - `ui/src/lib/ws.ts` now bypasses the Vite websocket proxy in dev mode and connects directly to the backend websocket origin (`:4173` by default, override with `VITE_UI_API_ORIGIN`) to avoid repeated proxy `ECONNRESET` failures.
  - `internal/api/server.go` now explicitly allows localhost websocket origins so the cross-origin dev UI (`:5173`) can subscribe to `/api/ws` on the backend (`:4173`).
  - `internal/api/server.go` now gives upgraded websocket clients a connection-scoped lifetime instead of tying read/write pumps to the HTTP request context, which was causing live log streams to die before later run events arrived.
  - `internal/cli/ui_command.go` now resolves `agent.claude.command` to an absolute binary path during UI preflight and injects that resolved path into the async run runtime, avoiding later `PATH` drift for UI-triggered Claude runs.
  - `infrafactory.yaml` now pins the local Claude binary to `/opt/homebrew/bin/claude` on this machine so UI-triggered runs do not depend on shell PATH setup.
  - `internal/runstore/runstore.go` now explicitly skips directories missing `run.json` before metadata decode, preserving `/runs` even when the runstore contains older partial directories.
  - Added `GET /api/runs/{scenario}/{run_id}/log` backed by persisted `app.log`, and `/live` now replays those log lines before appending websocket frames. The page only falls back to synthesized metadata/artifact lines when both replay and websocket data are absent.
  - Fresh-context docs were updated so future agents do not need to rediscover these contracts:
    - `SESSION_START.md`
    - `README.md`
    - `CONCEPT.md`
    - `docs/plans/web-ui-plan.md` (marked as historical and aligned with current gates/runtime notes)
  - Added focused regression tests:
    - `TestRunCommandPersistsRunningMetadataBeforeCompletion`
    - frontend tests for `synthesizeLiveConsoleLines(...)`
    - frontend tests for websocket URL origin selection
    - backend websocket test for `Origin: http://127.0.0.1:5173`
    - HTTP-level start-run + websocket broadcast integration test
    - UI-run preflight test for resolved absolute Claude path
    - run-history regression test for historical incomplete directory names
    - run-log handler test for `app.log` replay
    - frontend replay/live merge helper test
  - Verified with:
    - `go test -timeout=20s -run 'TestRunCommandPersistsRunningMetadataBeforeCompletion' ./internal/cli`
    - `go test -timeout=60s ./internal/api`
    - `cd ui && npm test && npm run build`
    - `bash scripts/check_all.sh`
    - real local runtime reproduction: started backend on `127.0.0.1:4186`, connected a websocket client with `Origin: http://127.0.0.1:5173`, triggered `POST /api/runs/web-app-paris/start`, and observed streamed `run_start`, `iteration_start`, and `stage_start` frames.
    - real local UI-triggered run on `127.0.0.1:4189` with the pinned absolute Claude path reached `generator/claude: phase "plan_architecture" start` without reproducing `exec: "claude": executable file not found in $PATH`.

- **SUi-1 complete (Slice 21A)**:
  - Added `infrafactory ui` command with default bind `127.0.0.1:4173`.
  - Added root options pattern: `NewRootCmd(opts ...RootOption)` and `WithUIAssets(fs.FS)`.
  - Added build-tag embed split:
    - `cmd/infrafactory/embed.go` (`!noui`) embeds `ui/build`.
    - `cmd/infrafactory/embed_dev.go` (`noui`) provides nil assets.
  - Added `internal/api` skeleton:
    - `GET /api/config` allowlisted response only.
    - `GET /api/*` fallback returns `501 not implemented`.
    - SPA handler serves static files and `index.html` fallback when assets are embedded.
    - Non-API requests return deterministic 404 JSON message in API-only (`noui`) mode.
  - Added placeholder SvelteKit scaffold in `ui/`.
  - Added tests for server/spa/config handlers and root command wiring.
  - Updated `scripts/check_all.sh` to run `go test -tags noui ./...` when `ui/build/` is absent.
  - Added ADR-0008 documenting `ui` command contract and `noui` API-only behavior.
- **Slice 21 execution plan retained**: Full implementation plan remains in `docs/plans/web-ui-plan.md` as the historical/design reference for the shipped Web UI slice.
- **Slice 20 complete (S20-T1..S20-T6)**: 6 new scenarios exercising untested parameter combos:
  - `mysql-ha-paris`: mysql engine, medium DB, HA=true, private networking.
  - `compute-lb-multi-paris`: large compute (count=3), multi-backend LB (80/http + 443/tcp).
  - `k8s-medium-override-paris`: medium K8s with node_type/node_count overrides.
  - `private-lb-db-paris`: private LB, large PostgreSQL with node_type/engine_version overrides.
  - `public-registry-iam-paris`: is_public=true registry, IAM with policy=false.
  - `redis-xlarge-session-paris`: xlarge Redis with node_type override, xlarge compute.
  - Prompt fixes: LB backend/frontend zone pitfall, compute type mapping enforcement, phase1 exact-mapping enforcement.
  - Mockway fix: expanded server type catalog (GP1-L, GP1-XL, DEV1-L).
  - All 12 scenarios (6 existing + 6 new) pass on first iteration.
- **S19-T1 complete (round 4)**: Referential integrity and validation strictness:
  - **Delete cascades removed** — `DeleteLB`, `DeleteCluster`, `DeleteRDBInstance` now return 409 Conflict when dependents exist (per AGENTS.md contract). Exception: `lb_private_networks` cascade since the Scaleway provider doesn't detach them before LB delete.
  - **init_endpoints strict validation** — `BuildRDBEndpointsFromInit` rejects `private_network` with missing ID instead of silently falling back to public endpoint.
  - All 6 scenarios still pass on first iteration.
- **S19-T1 complete (round 2)**: Additional reliability fixes from extended review:
  - **IAM defaults not applied** — JSON Schema declared `default: true` for application/api_key/policy but Go's json.Unmarshal doesn't apply schema defaults. Fixed with `applyIAMDefaults()` that checks the raw YAML for omitted fields.
  - **LB/Frontend/Backend updates didn't persist** — same pattern as RDB update bug. Added `repo.UpdateLB()`, `repo.UpdateFrontend()`, `repo.UpdateBackend()` methods.
  - **LB list routes leaked cross-LB data** — `ListFrontends`/`ListBackends` ignored `lb_id` URL param. Added `ListFrontendsByLB`/`ListBackendsByLB` repo methods.
  - **RDB certificate endpoint didn't check instance existence** — returned 200 for nonexistent IDs. Now 404s.
  - **K8s companion test coverage** — added `scaleway_k8s_pool` auto-include test.
  - **Scenario test coverage** — added fixtures + tests for `iam`, `registry`, `redis`, `kubernetes` resource types and IAM default behavior.
  - All 6 scenarios still pass on first iteration.
- **S19-T1 complete (round 1)**: 3 bugs fixed (RDB update persistence, Redis missing fields).
- Completed Slice 18 (`S18-T1`..`S18-T5`): all 5 new scenarios pass on first iteration.
