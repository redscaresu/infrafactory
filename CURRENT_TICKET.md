# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: M36
- title: Apply second-pass maintenance fixes from post-slice 22-29 review
- status: done
- classification: implementation-only

## 1) Problem Statement
- What is broken or missing?
  A second review pass found four small hardening issues in WebSocket JSON escaping, state-policy input flattening, iteration artifact path validation, and Mockway error payload handling.
- Why does it matter now?
  These are edge-case correctness and defense-in-depth issues that are cheap to fix now and harder to reason about later.

## 2) Scope
- In scope: small maintenance fixes plus focused regression coverage.
- Out of scope: new feature work.

## 3) Acceptance Criteria
1. UI error broadcasting uses valid JSON escaping for control characters.
2. State-policy input flattening no longer silently shadows a top-level `state` key.
3. Iteration artifact writes reject traversal paths.
4. Mockway state fetch errors truncate large non-2xx payloads.
5. Verification passes after the fixes.

## Progress notes

### Completed implementation
- Switched `escapeJSONString` to JSON-backed escaping so tabs, carriage returns, form feeds, backspaces, and other control characters remain valid in WebSocket payloads.
- Made state-policy envelope construction fail explicitly if the decoded mock state already contains a top-level `state` key.
- Reused `resolveGeneratedPath` in iteration artifact writes for traversal defense-in-depth.
- Truncated large Mockway non-2xx state payloads before embedding them in returned errors.
- Added focused regression tests for each maintenance fix.

### Verification
- Focused `go test ./internal/cli`, `./internal/harness`, and `./internal/runstore` runs passed for the maintenance paths.
- Final full-suite verification recorded in the handoff.

## Operational Caveats
- The full `go test -tags noui ./...` sweep remains slow because `internal/cli` takes roughly 50s in this repo.

## Blocker (if any)
- blocker: none.
