# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: UI-FOLLOWUP
- title: Web UI post-slice hardening and per-run IaC history
- status: done
- classification: implementation-only

## 1) Problem Statement
- What is broken or missing?
  The initial Slice 21 UI lacked durable run diagnostics, clear Live failure handling, and immutable per-run IaC history in the UI and API contracts.
- Why does it matter now?
  The user needed the UI to be operationally trustworthy: visible runtime health, readable failure states, stable run history, and run-specific IaC snapshots.

## 2) Scope
- In scope: UI starter/runtime hardening, diagnostics, run history improvements, per-run IaC storage/viewing, and durable doc/test sync.
- Out of scope: changing generator transport strategy. Note: ADR-0003 has been superseded by ADR-0010.

## 3) Acceptance Criteria
1. Starting a run returns a clear API/UI error if the configured generator runtime is unavailable.
2. The Live page shows readable failure details without requiring users to inspect raw JSON.
3. Each run stores its own immutable generated IaC set and the UI can display it.
4. Focused tests and `bash scripts/check_all.sh` pass.
5. Docs reflect the durable Slice 21 follow-up state.

## Progress notes

### Completed implementation
- Added `uiRunStarter` preflight checks in `internal/cli/ui_command.go`:
  - `claude-code` now verifies the configured CLI command exists in `PATH`.
  - `openrouter` now verifies `OPENROUTER_API_KEY` is set before accepting the run request.
- Added focused Go tests for both preflight failure modes.
- Updated `ui/src/lib/api.ts` to unwrap JSON API error payloads into readable `Error` messages.
- Reworked `ui/src/routes/live/+page.svelte` to render failure cards with iteration, stage, check, command, and detail fields while preserving live event streaming.
- Added `GET /api/diagnostics` and a `/diagnostics` UI page for backend generator readiness checks.
- Added Run History filtering and improved `/live` fallback behavior for scenario-scoped latest runs.
- Added regression tests for incomplete run directories so `/api/runs` remains available even with partial run artifacts on disk.
- Added per-run generated IaC persistence in the runstore and run-scoped read/list API endpoints.
- Upgraded the run detail route into a per-run IaC viewer and added Run History actions for `IaC` and `Live`.
- Extended runstore persistence to keep per-iteration IaC snapshots under `iterations/<n>/generated/`.
- Added iteration-list/iteration-files API endpoints and a run bundle download endpoint for historical IaC retrieval.
- Upgraded the run detail page with snapshot selection, syntax-highlighted IaC preview, and a download button.
- Added regression tests for iteration snapshot persistence and the run bundle contract, plus frontend highlighting helper tests.
- Added a full run artifact archive endpoint and wired a second download action in the UI.
- Added snapshot-to-snapshot diffing in the run detail page.
- Replaced the initial lightweight HCL tokenizer with a richer stateful highlighter covering functions, interpolation, heredocs, attributes, and block comments.
- Fixed the Makefile `.PHONY` parse bug so `make ui-build` works again.
- Diagnosed a real local regression where older runs had no persisted `generated/` snapshot directories; updated the run detail page to fall back to `output/<scenario>/` with an explicit warning in that case.
- Added a `Started` column to the Run History page.
- Added frontend logic tests for Run History/Live state selection and filtering.
- Fixed Live run visibility during active execution by persisting initial `running` metadata before generator completion, so the UI can poll `/api/runs/{scenario}/{run_id}` immediately after a run starts.
- Added synthesized Live console output when websocket messages are absent, using polled run metadata and iteration artifacts as the fallback source of truth.
- Removed the Vite websocket proxy dependency for the browser Live stream path; dev UI now connects directly to the backend websocket origin and supports `VITE_UI_API_ORIGIN` override.
- Updated backend websocket accept policy to allow local dev browser origins (`127.0.0.1:*`, `localhost:*`) and added a regression test for `Origin: http://127.0.0.1:5173`.
- Fixed websocket connection lifetime so upgraded clients are no longer bound to the short-lived HTTP request context.
- Added an HTTP-level regression test that opens `/api/ws`, calls `POST /api/runs/{scenario}/start`, and verifies a websocket log frame is delivered.
- Resolved `claude` to an absolute binary path during UI preflight and reused that path in the async run runtime.
- Pinned `agent.claude.command` in `infrafactory.yaml` to `/opt/homebrew/bin/claude` for this local machine so the config itself is explicit.
- Hardened run history enumeration so incomplete historical run directories without `run.json` are skipped deterministically.
- Added persisted run-log replay endpoint and updated `/live` to render `app.log` history plus websocket frames instead of relying on websocket delivery alone.
- Updated fresh-context and durable docs so future agents inherit the actual Slice 21 runtime/testing contracts instead of the original plan assumptions.
- Verified the live server against a synthetic runstore on `127.0.0.1:4182`.
- Synced README/ROADMAP/BACKLOG/web-ui plan notes to the run-scoped IaC and diagnostics behavior.
- Synced `SESSION_START.md`, `CONCEPT.md`, and `internal/runstore/doc.go` so fresh-session guidance and architecture notes reflect the shipped UI and immutable per-run IaC history.
- Closure review protocol:
  - Review pass 1: found doc drift and updated durable docs.
  - Review pass 2: no further improvements found.
  - Review pass 3: no further improvements found.

### Verification
- `go test -timeout=60s -run 'TestUIRunStarterPreflight|TestRuns' ./internal/cli ./internal/api` passed.
- `go test ./internal/api ./internal/runstore ./internal/cli` passed.
- `go test -timeout=20s -run 'TestRunCommandPersistsRunningMetadataBeforeCompletion' ./internal/cli` passed.
- `go test -timeout=60s ./internal/api` passed.
- `cd ui && npm test && npm run build` passed.
- `bash scripts/check_all.sh` passed.

## Operational Caveats
- Containerized runs still need a valid generator runtime inside the backend container; the new preflight makes that failure explicit instead of deferring it into run artifacts.

## Blocker (if any)
- blocker: none for unblocked tickets.
