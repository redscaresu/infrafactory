# ADR-0008: UI Command and `noui` API-Only Mode

## Status
Accepted

## Context
Slice 21 introduces a browser UI served by the CLI binary. The existing command contract did not include a `ui` subcommand, and the repository must remain testable before frontend assets are built.

The web UI uses static frontend assets generated under `ui/build/`, but these assets are not always present in fresh Go-only environments. The CLI still needs to expose API endpoints for two-terminal development (`go run -tags noui ...` + `npm run dev`).

## Decision
1. Add a new top-level CLI subcommand: `infrafactory ui`.
2. Register `ui` unconditionally in `NewRootCmd`, regardless of build tag.
3. Use build-tag split embed wiring:
   - `!noui`: embed `ui/build` with `go:embed`.
   - `noui`: provide nil assets.
4. Define `ui` runtime behavior:
   - With embedded assets: serve SPA + API.
   - Without embedded assets (`assets == nil`): serve API endpoints only and return deterministic 404 JSON for non-API paths with actionable dev/build guidance.
5. Keep Go test/build workflow green without frontend artifacts by using `go test -tags noui ./...` when `ui/build/` is absent.

## Consequences
- CLI contract expands with a stable `ui` command.
- Local development supports API-only mode without requiring `npm run build`.
- Build/test scripts must account for `noui` mode until frontend assets are produced.
- SPA serving and config redaction behavior are now part of the backend contract and covered by tests.

