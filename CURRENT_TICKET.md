# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: SUi-1
- title: Slice 21 — Web UI dashboard (skeleton server + static asset embed)
- status: todo
- classification: feature

## 1) Problem Statement
- What is broken or missing?
  InfraFactory is CLI-only. No way to visually browse scenarios, watch runs in real time, or inspect run history/generated code from a browser.
- Why does it matter now?
  Slices 1-20 complete with 12 passing scenarios. The pipeline is stable enough to build a visual layer on top.

## Prerequisites
- Node.js 18+ and npm 9+ installed (`node --version`, `npm --version`)
- `go test -tags noui ./...` passes before starting

## 2) Scope
- In scope: SUi-1 (skeleton server + SvelteKit embed + `infrafactory ui` command). Full Slice 21 plan covers SUi-1 through SUi-8.
- Out of scope: existing CLI behavior changes. All existing commands and tests must remain unaffected.
- Plan reference: `docs/plans/web-ui-plan.md` — read Quick Reference, SUi-1 section, and Pitfalls #1-4, #11 before coding.

## 3) Acceptance Criteria
1. `make build && ./bin/infrafactory ui` serves a placeholder SvelteKit page at `127.0.0.1:4173`.
2. `go test -tags noui ./...` passes (no breakage to existing CLI tests).
3. `NewRootCmd(opts ...RootOption)` functional options pattern implemented.
4. SPA fallback handler serves `index.html` for unmatched routes when UI assets are embedded; returns 404 JSON with dev-mode message when assets=nil.

## Progress notes

### Planning (complete)
- Full UI plan written: `docs/plans/web-ui-plan.md`.
- 8 sub-slices defined (SUi-1..SUi-8).
- Tech stack: SvelteKit (adapter-static), Go `net/http`, WebSocket (`github.com/coder/websocket`), `go:embed`.
- Docs updated: ROADMAP, STATUS, BACKLOG, CURRENT_TICKET, SESSION_START.

## Operational Caveats
- `ui/build/` does not exist yet. Use `-tags noui` for all Go builds until `npm run build` populates it.
- The Makefile has no existing `build` target — SUi-1 creates it as a new target.

## Blocker (if any)
- blocker: none.
