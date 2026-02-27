# Plan: InfraFactory Web UI (Slice 21)

## Quick Reference

| Key | Value |
|---|---|
| Ticket IDs | SUi-1 through SUi-8 |
| Frontend | SvelteKit (adapter-static) + Tailwind CSS + CodeMirror 6 |
| Backend | Go `net/http.ServeMux` (Go 1.22+ routing) |
| WebSocket | `github.com/coder/websocket` (nhooyr fork) |
| Embed | `go:embed` with `noui` build tag fallback |
| Auth | None (localhost-only by default) |
| Default addr | `127.0.0.1:4173` |
| Plan status | Approved, not started |

## Prerequisites

- **Go 1.22+** (required for `http.ServeMux` path-pattern routing). Project uses Go 1.24.6.
- **Node.js 18+** and **npm 9+** (for SvelteKit build tooling). NOT required for `go test -tags noui ./...`.

```bash
go version        # Must show 1.22+
node --version    # Must show v18+
npm --version     # Must show 9+
```

## Reading Guide

Agents implementing a specific slice: read **Quick Reference**, **Canonical Contracts**, **Architecture**, **your slice section**, and **Pitfalls**. Skip other slices.

| Your slice | Also read |
|---|---|
| SUi-1 | Pitfalls #1-4, #11 |
| SUi-2 | API Endpoints (scenarios), Pitfall #9 |
| SUi-3 | API Endpoints (runs), Pitfall #8 |
| SUi-4 | API Endpoints (output) |
| SUi-5 | API Endpoints (WS messages), Pitfalls #5, #10 |
| SUi-6 | Architecture (executeRunLoop), Pitfalls #2, #10 |
| SUi-7 | API Endpoints (PUT scenarios) |
| SUi-8 | All build sections, Pitfall #3 |

---

## Canonical Contracts

These contracts are fixed across all slices. Do not deviate.

### 1. Bind address

Default: `127.0.0.1:4173` (localhost only). Override with `--addr`. Never default to `0.0.0.0`.

### 2. Scenario identifiers

Two identifiers exist. Each API endpoint uses exactly one:

| Identifier | Example | Used by | Source |
|---|---|---|---|
| **Scenario path** (relative to scenarios dir) | `training/web-app-paris` | `/api/scenarios/{path...}` | Filesystem walk of `cfg.Paths.Scenarios` |
| **Scenario name** (leaf name, no directory) | `web-app-paris` | `/api/runs/{scenario}`, `/api/output/{scenario}`, `POST /api/runs/{scenario}/start` | YAML `scenario` field (`Scenario.Name` in Go, JSON tag `"scenario"`); also runstore dir name and output dir name |

**Why two?** The scenario path includes the group directory (`training/`, `holdout/`) needed to locate the YAML file. The scenario name is what the runstore and output dir use as keys (no slashes, no ambiguity with nested URL segments like `/{runID}`).

**Frontend mapping**: The scenario list response (`GET /api/scenarios`) returns both `path` and `name` per scenario. The frontend uses `path` for scenario navigation and `name` for run/output lookups.

### 3. `ui` command registration

The `ui` command is **always registered** regardless of build tag. When `uiAssets` is nil (noui build), the server starts in API-only mode — `/api/*` endpoints work, but non-API requests return 404 with a message: `"UI assets not embedded. Run Vite dev server on :5173 or build with: make build"`. This enables the two-terminal dev workflow without requiring `npm run build` first.

### 4. Config redaction (allowlist)

`GET /api/config` returns only these fields:

```json
{
  "version": "...",
  "agent": {
    "type": "...",
    "repair_iterations_max": 5,
    "phase_delay_seconds": 0,
    "phases": ["plan_architecture", "generate_hcl", "self_review"],
    "openrouter": { "model": "...", "base_url": "..." }
  },
  "paths": {
    "scenarios": "./scenarios",
    "output": "./output"
  },
  "validation": { "layers": { ... } }
}
```

**Omitted**: `mockway` (internal infra), `scaleway` (credentials context), `constraint_policies` (file paths), `claude.command` (local binary path), `openrouter.timeout_seconds`, `openrouter.max_retries`, `paths.mappings`, `paths.policies`, `paths.prompts`.

### 5. Test gates (staged)

| Gate | When it must pass | Requires |
|---|---|---|
| `go test -tags noui ./...` | Every commit (CI gate) | Go only |
| `go test ./...` | After `npm run build` populates `ui/build/` | Go + Node.js |
| `cd ui && npm run build` | After any frontend change | Node.js |
| `cd ui && npx playwright test` | After Playwright-tested slices | Node.js + browsers |

---

## Design Decisions

1. **Run location**: Clicking "Run" on a scenario auto-navigates to `/live`. Sidebar stays visible.
2. **Dashboard**: Scenario overview grid with last-run status cards (like a CI dashboard). Each card has a small "Run" icon for quick access.
3. **Styling**: Tailwind CSS (utility-first, no component library).
4. **Auth**: No auth. Bind to `127.0.0.1` by default. Optional `--addr` flag for network binding.
5. **Editor**: CodeMirror 6 for YAML editing (lightweight, YAML syntax highlighting, line numbers).
6. **Code viewer**: Read-only with syntax highlighting (highlight.js HCL grammar).
7. **Multi-tab**: All tabs receive WebSocket events independently. Hub broadcasts to all.
8. **Run result display**: Green/red/yellow banner with terminal reason + expandable failure detail cards.
9. **Live page scope**: Auto-shows the active run (only one at a time via mutex). If no run active, shows "No active run" with prompt.
10. **Run list detail**: Shows terminal_reason column (target_reached / repair_budget_exhausted / stuck) alongside status badge.

---

## Architecture

```
infrafactory ui --addr 127.0.0.1:4173

┌─────────────────────────────────────────────────────┐
│ Go HTTP Server (net/http.ServeMux)                  │
│                                                     │
│ /api/scenarios      → handlers_scenarios.go         │
│ /api/runs           → handlers_runs.go              │
│ /api/output         → handlers_output.go            │
│ /api/config         → handlers_config.go            │
│ /api/runs/*/start   → handlers_run_executor.go      │
│ /api/ws             → hub.go + client.go            │
│ /*                  → spa.go (embedded SvelteKit)   │
│                                                     │
│ WebSocketSink → AppLogger.sinks → all browsers      │
│                                                     │
│ Reuses: scenario, runstore, generator, harness,     │
│         config, feedback, logger packages           │
└─────────────────────────────────────────────────────┘
```

**Key integration**: `AppLogger` (`internal/cli/logger.go:30`) already writes JSON `LogEntry` to `[]io.Writer` sinks. A `WebSocketSink` implementing `io.Writer` plugs in at runtime. Zero changes to existing logging call sites.

**Key refactor**: Extract `executeRunLoop()` from `runRunCommand` (`internal/cli/run_command.go:18-258`). The function currently reads flags from `*cobra.Command` and writes output via `writeCommandOutput(cmd, ...)`. Extract lines 59-240 (setup, iteration loop, holdout checks, result construction) into a standalone function that takes `context.Context` + `*CommandRuntime` + `runControls` and returns `OutputResult`. The API handler calls this directly. Note: `runIteration` (line 308) already takes `context.Context` as first parameter; the `cmd *cobra.Command` parameter at line 314 is completely unused — simply remove it.

---

## File Inventory

### New files (Go)

| File | Purpose |
|---|---|
| `internal/api/server.go` | `NewServer(cfg ServerConfig) *Server` — sets up `http.ServeMux`, registers all API routes; conditionally adds SPA handler when `cfg.Assets != nil`, otherwise non-API requests return 404 with dev-mode message |
| `internal/api/hub.go` | WebSocket connection hub: `Register(c)`, `Unregister(c)`, `Broadcast(msg)`, `Run(ctx)` goroutine |
| `internal/api/client.go` | Per-connection WebSocket client: read pump (pings), write pump (buffered channel, drop on slow) |
| `internal/api/ws_sink.go` | `WebSocketSink{hub *Hub}` implements `io.Writer` — plugs into `AppLogger.sinks` |
| `internal/api/spa.go` | `SPAHandler(assets fs.FS)` — serves static files, falls back to `index.html` for SPA routes |
| `internal/api/handlers_scenarios.go` | `GET /api/scenarios`, `GET /api/scenarios/{path...}`, `PUT /api/scenarios/{path...}` |
| `internal/api/handlers_runs.go` | `GET /api/runs`, `GET /api/runs/{scenario}`, `GET /api/runs/{scenario}/{runID}`, `GET /api/runs/{scenario}/{runID}/iterations/{n}` |
| `internal/api/handlers_output.go` | `GET /api/output/{scenario}`, `GET /api/output/{scenario}/{file...}` |
| `internal/api/handlers_config.go` | `GET /api/config` — returns config with credentials redacted |
| `internal/api/handlers_run_executor.go` | `POST /api/runs/{scenario}/start` — starts async run via `executeRunLoop`, streams events to WebSocket |
| `internal/cli/ui_command.go` | `newUICmd(assets fs.FS) *cobra.Command` — `infrafactory ui --addr 127.0.0.1:4173` |
| `cmd/infrafactory/embed.go` | `//go:build !noui` — `//go:embed all:ui/build` |
| `cmd/infrafactory/embed_dev.go` | `//go:build noui` — nil `fs.FS` (no embedded assets) |

### New test files (Go)

| File | Tests |
|---|---|
| `internal/api/server_test.go` | Server starts, serves SPA fallback, `/api/*` routes registered |
| `internal/api/spa_test.go` | Known file returns content; unknown path returns `index.html`; `/api/*` not caught by SPA |
| `internal/api/hub_test.go` | Register/unregister; broadcast delivers to all; slow client dropped after buffer full |
| `internal/api/ws_sink_test.go` | `Write()` calls `hub.Broadcast()`; nil hub is safe |
| `internal/api/handlers_scenarios_test.go` | GET list (grouped); GET detail (YAML + metadata); GET 404; PUT valid; PUT 422 (schema); PUT 400 (bad YAML); PUT path traversal rejected |
| `internal/api/handlers_runs_test.go` | GET all runs; GET per-scenario; GET run detail; GET iteration; 404s for nonexistent |
| `internal/api/handlers_output_test.go` | GET file list (excludes `.terraform/`, state); GET file content; path traversal rejected |
| `internal/api/handlers_config_test.go` | GET config; credentials redacted in response |
| `internal/api/handlers_run_executor_test.go` | POST returns 202; concurrent POST returns 409; mock run sends WebSocket events |

### Modified files (Go)

| File | Change | Exact location |
|---|---|---|
| `internal/cli/root.go` | Add `RootOption` type, `WithUIAssets(fs)`, change `NewRootCmd()` to `NewRootCmd(opts ...RootOption)`. Always add `ui` command (API-only when assets=nil). | Lines 17-41 |
| `cmd/infrafactory/main.go` | Pass `cli.WithUIAssets(uiAssets)` to `NewRootCmd` | Line 11 |
| `internal/cli/run_command.go` | Extract `executeRunLoop(ctx, runtime, scenarioPath, sc, controls, emitter) (OutputResult, error)` from lines 59-240. Remove unused `cmd *cobra.Command` parameter from `runIteration` (line 314 — already takes `ctx` at line 309). Add `EventEmitter` callback type. Persist `terminal_reason` in `WriteRunMetadata`. | Lines 59-240 → new function |
| `internal/runstore/runstore.go` | Add `ListScenarios() ([]string, error)`, `ReadIterationArtifact(scenario, runID string, iteration int) ([]byte, error)`. Extend `RunMetadata` with `TerminalReason string` field. | After line 131 |
| `go.mod` | Add `github.com/coder/websocket` | — |
| `Makefile` | Add `ui-install`, `ui-build`, `ui-dev`, `ui-clean`, update `build` target | After line 92 |
| `goreleaser.yml` | Add `before.hooks: [{cmd: make ui-build}]` | Before `builds:` |
| `.gitignore` | Add `ui/node_modules/`, `ui/build/`, `ui/.svelte-kit/` | End of file |

### New files (Frontend — `ui/`)

```
ui/
├── package.json                    # svelte, tailwindcss, codemirror, highlight.js, playwright
├── svelte.config.js                # adapter-static, fallback: index.html
├── vite.config.ts                  # proxy /api → Go server in dev
├── tailwind.config.js
├── postcss.config.js
├── tsconfig.json
├── playwright.config.ts
├── src/
│   ├── app.html
│   ├── app.css                     # @tailwind base/components/utilities
│   ├── routes/
│   │   ├── +layout.svelte          # Two-column: sidebar + <slot/>
│   │   ├── +layout.ts              # Load scenario list for sidebar
│   │   ├── +page.svelte            # Dashboard: scenario grid with last-run status
│   │   ├── scenarios/
│   │   │   └── [...path]/
│   │   │       ├── +page.svelte    # Scenario detail: metadata + YAML editor + Run button
│   │   │       └── +page.ts        # Fetch scenario detail
│   │   ├── runs/
│   │   │   ├── +page.svelte        # All runs list
│   │   │   └── [scenario]/
│   │   │       └── [runID]/
│   │   │           ├── +page.svelte # Run detail: iteration timeline + pipeline
│   │   │           └── +page.ts
│   │   ├── output/
│   │   │   └── [scenario]/
│   │   │       ├── +page.svelte    # Generated .tf file list + viewer
│   │   │       └── +page.ts
│   │   └── live/
│   │       └── +page.svelte        # Active run: real-time pipeline + log stream
│   └── lib/
│       ├── api.ts                  # REST fetch wrapper (base URL from env or window.location)
│       ├── ws.ts                   # WebSocket client (auto-reconnect, typed message parsing)
│       ├── types.ts                # TypeScript interfaces matching Go response shapes
│       ├── stores/
│       │   ├── scenarios.ts        # Writable store: scenario tree
│       │   ├── runs.ts             # Writable store: run history
│       │   └── liveRun.ts          # Writable store: live run state (driven by WS messages)
│       └── components/
│           ├── Sidebar.svelte      # Scenario tree navigator (grouped by training/holdout)
│           ├── ScenarioCard.svelte  # Dashboard grid card: scenario name + last run badge
│           ├── PipelineView.svelte  # Horizontal: [Generate] → [Validate] → [Test]
│           ├── StageNode.svelte     # Single stage: pending|running|pass|fail|skip
│           ├── IterationTimeline.svelte  # Vertical iteration stack
│           ├── RunStatusBadge.svelte     # Colored badge: success/failed/stuck/running
│           ├── RunResultBanner.svelte    # Large banner: green/red/yellow + terminal reason
│           ├── FailureCard.svelte        # Expandable failure detail card
│           ├── YamlEditor.svelte         # CodeMirror 6 with YAML + validation feedback
│           ├── TfViewer.svelte           # highlight.js with HCL grammar (read-only)
│           └── LogStream.svelte          # Terminal-style scrolling log viewer
└── tests/
    ├── scenarios.spec.ts            # Playwright: sidebar, scenario detail
    ├── runs.spec.ts                 # Playwright: run list, iteration timeline
    ├── output.spec.ts               # Playwright: .tf file viewer
    ├── live.spec.ts                 # Playwright: start run, pipeline animation
    ├── editor.spec.ts               # Playwright: YAML edit, save, validation
    └── ws.spec.ts                   # Playwright: WebSocket connect/reconnect
```

---

## API Endpoints

| Method | Path | Response | Status codes |
|---|---|---|---|
| GET | `/api/scenarios` | `{groups: [{name: "training", scenarios: [{name, path, description, last_run: {run_id, status, terminal_reason} | null}]}]}` | 200 |
| GET | `/api/scenarios/{path...}` | `{name, path, description, raw_yaml, resources, constraints, criteria}` | 200, 404 |
| PUT | `/api/scenarios/{path...}` | `{ok: true}` or `{errors: [...]}` | 200, 400, 422 |
| GET | `/api/runs` | `{runs: [{scenario, run_id, status, terminal_reason, started_at}]}` | 200 |
| GET | `/api/runs/{scenario}` | `{runs: [{run_id, status, terminal_reason, started_at}]}` | 200 |
| GET | `/api/runs/{scenario}/{runID}` | `RunMetadata` | 200, 404 |
| GET | `/api/runs/{scenario}/{runID}/iterations/{n}` | Iteration JSON (stages, failures) | 200, 404 |
| GET | `/api/output/{scenario}` | `{files: ["main.tf", "providers.tf", ...]}` | 200, 404 |
| GET | `/api/output/{scenario}/{file...}` | Raw file content (`text/plain`) | 200, 404 |
| GET | `/api/config` | Config JSON (credentials redacted) | 200 |
| POST | `/api/runs/{scenario}/start` | `{run_id: "..."}` | 202, 409 (already running) |
| GET | `/api/ws` | WebSocket upgrade | 101 |

### WebSocket Message Types

```typescript
type WSMessage =
  | { type: "log";                data: LogEntry }
  | { type: "stage_start";        data: { iteration: number; stage: string } }
  | { type: "stage_complete";     data: { iteration: number; stage: string; status: "pass"|"fail" } }
  | { type: "iteration_complete"; data: { iteration: number; stages: StageSummary[]; failures: FailureSummary[] } }
  | { type: "run_complete";       data: { run_id: string; status: "success"|"failed"; terminal_reason: string } }
  | { type: "run_error";          data: { error: string } }
```

---

## Implementation Slices

### SUi-1: Skeleton server + static asset embed

**Goal**: `infrafactory ui` serves a placeholder SvelteKit page at `http://127.0.0.1:4173`.

**Steps**:

1. Create `ui/` SvelteKit scaffold:
   ```bash
   cd ui && npx sv create . --template minimal --types ts
   npm install -D @sveltejs/adapter-static tailwindcss @tailwindcss/vite
   ```
   Configure `svelte.config.js` with adapter-static (fallback: `index.html`).
   Configure `vite.config.ts` to proxy `/api` → `http://127.0.0.1:4173`.
   Add Tailwind to `app.css`.
   Set `+page.svelte` to show "InfraFactory Dashboard" placeholder.

2. Create `cmd/infrafactory/embed.go`:
   ```go
   //go:build !noui
   package main
   import "embed"
   //go:embed all:ui/build
   var uiAssets embed.FS
   ```

3. Create `cmd/infrafactory/embed_dev.go`:
   ```go
   //go:build noui
   package main
   import "io/fs"
   var uiAssets fs.FS
   ```
   **Compile check**: Run `go build -tags noui ./cmd/infrafactory` immediately after creating both embed files. If `ui/build/` does not exist yet, the `!noui` build will fail with `pattern all:ui/build: no matching files found`. Always use `-tags noui` until after `npm run build` has populated `ui/build/`.

4. Create `internal/api/server.go`:
   - `type ServerConfig struct { Addr string; Assets fs.FS; Config config.Config; Hub *Hub; ... }`
   - `func NewServer(cfg ServerConfig) *http.Server` — registers routes on `http.ServeMux`
   - `/api/config` route → `handlers_config.go` (returns allowlisted config fields)
   - Other `/api/*` routes (placeholder 501 for now)
   - If `cfg.Assets != nil`: `/*` routes → SPA handler (embedded static files)
   - If `cfg.Assets == nil`: `/*` routes → 404 with JSON `{"error": "UI assets not embedded. Run Vite dev server on :5173 or build with: make build"}`

4b. Create `internal/api/handlers_config.go`:
   - `GET /api/config` — builds allowlisted response struct from `ServerConfig.Config` (see Canonical Contracts §4 for exact fields), returns JSON

5. Create `internal/api/spa.go`:
   - `func SPAHandler(assets fs.FS) http.Handler`
   - Strips `ui/build/` prefix from embedded FS
   - Tries to serve static file; if not found, serves `index.html`
   - Does NOT catch `/api/*` paths

6. Create `internal/cli/ui_command.go`:
   - `func newUICmd(assets fs.FS) *cobra.Command`
   - Flag: `--addr` (default `127.0.0.1:4173`)
   - Loads config, creates `NewServer(...)`, calls `ListenAndServe`
   - Graceful shutdown on context cancellation

7. Modify `internal/cli/root.go`:
   ```go
   type RootOption func(*rootConfig)
   type rootConfig struct { uiAssets fs.FS }
   func WithUIAssets(assets fs.FS) RootOption { return func(c *rootConfig) { c.uiAssets = assets } }
   func NewRootCmd(opts ...RootOption) *cobra.Command {
       cfg := &rootConfig{}
       for _, opt := range opts { opt(cfg) }
       // ... existing code ...
       cmd.AddCommand(newUICmd(cfg.uiAssets))  // always registered — API-only when assets=nil
       return cmd
   }
   ```

8. Modify `cmd/infrafactory/main.go`:
   ```go
   func main() {
       if err := cli.NewRootCmd(cli.WithUIAssets(uiAssets)).Execute(); err != nil {
   ```

9. Update `Makefile` — add `ui-install`, `ui-build`, `ui-dev`, `ui-clean` targets. **Note**: The Makefile has no existing `build` target — create it as a new target (not a modification).

10. Update `.gitignore` — add `ui/node_modules/`, `ui/build/`, `ui/.svelte-kit/`.

**Tests**:
- `internal/api/server_test.go`: server starts; `GET /` with assets returns 200 with HTML; `GET /` with assets=nil returns 404 JSON with dev-mode message
- `internal/api/spa_test.go`: known static file returns content; unknown path returns `index.html`; `/api/*` not caught by SPA
- `go test -tags noui ./...` still passes

**Done when**: `make build && ./bin/infrafactory ui` → browser shows placeholder. `go test -tags noui ./...` green.

---

### SUi-2: Scenario browser + sidebar

**Goal**: Left sidebar shows scenario tree grouped by directory. Clicking shows YAML + metadata.

**Steps**:

1. Create `internal/api/handlers_scenarios.go`:
   - `GET /api/scenarios` — walks `cfg.Paths.Scenarios`, groups by subdirectory (training/holdout), includes `last_run: {run_id, status, terminal_reason}` per scenario by reading latest run from runstore, returns JSON
   - `GET /api/scenarios/{path...}` — reads YAML file, parses with `scenario.LoadWithSchema`, returns raw YAML + parsed fields
   - Path validation: reject `..` segments, ensure path stays under scenarios root

2. Create frontend components:
   - `Sidebar.svelte` — grouped tree with expand/collapse, active item highlight
   - `ScenarioCard.svelte` — dashboard grid card: scenario name + last run badge + small "Run" icon button
   - `+layout.svelte` — two-column: 260px sidebar + flex main area
   - `+layout.ts` — calls `GET /api/scenarios` on mount
   - `scenarios/[...path]/+page.svelte` — shows scenario name, description, YAML in `<pre>`, resource summary

3. Create `ui/src/lib/api.ts` — fetch wrapper with base URL, JSON parsing, error handling
4. Create `ui/src/lib/types.ts` — TypeScript interfaces matching Go response shapes
5. Create `ui/src/lib/stores/scenarios.ts` — writable store for scenario tree

**Tests**:
- `handlers_scenarios_test.go`: GET list returns grouped; GET detail returns YAML; GET 404; path traversal rejected
- `ui/tests/scenarios.spec.ts`: Playwright sidebar renders, click navigates

**Done when**: Sidebar shows scenarios, clicking shows YAML. Handler tests pass.

---

### SUi-3: Run history browser

**Goal**: Browse past runs per scenario, see iterations with stage/failure details.

**Steps**:

1. Add `ListScenarios()` to `internal/runstore/runstore.go`:
   ```go
   func (s *FilesystemStore) ListScenarios() ([]string, error) {
       entries, err := os.ReadDir(s.Root)
       // return sorted directory names
   }
   ```

2. Add `ReadIterationArtifact(scenario, runID string, iteration int) ([]byte, error)` to runstore.

3. Create `internal/api/handlers_runs.go`:
   - `GET /api/runs` — calls `store.ListScenarios()` then `store.ListRuns(scenario)` for each
   - `GET /api/runs/{scenario}` — calls `store.ListRuns(scenario)`
   - `GET /api/runs/{scenario}/{runID}` — calls `store.ReadRunMetadata(scenario, runID)`
   - `GET /api/runs/{scenario}/{runID}/iterations/{n}` — reads iteration JSON from disk

4. Create frontend:
   - `runs/+page.svelte` — table of all runs with scenario name, run ID, status badge, terminal_reason, started_at
   - `runs/[scenario]/[runID]/+page.svelte` — run detail with `IterationTimeline`
   - `IterationTimeline.svelte` — vertical stack: iteration number + stage pipeline + failure cards
   - `RunStatusBadge.svelte` — colored pill: green (success), red (failed), yellow (stuck)
   - `FailureCard.svelte` — expandable: layer/stage/check/detail

**Tests**:
- `handlers_runs_test.go`: all GET endpoints, 404 for nonexistent
- `runstore_test.go`: `ListScenarios()` and `ReadIterationArtifact()` tests
- `ui/tests/runs.spec.ts`: Playwright run list, iteration expand

**Done when**: Can browse `.infrafactory/runs/` data through UI. Tests pass.

---

### SUi-4: Generated code viewer

**Goal**: View .tf files per scenario with HCL syntax highlighting.

**Steps**:

1. Create `internal/api/handlers_output.go`:
   - `GET /api/output/{scenario}` — lists files in `output/{scenario}/`, excludes `.terraform/`, `*.tfstate*`, `*.tfplan`
   - `GET /api/output/{scenario}/{file...}` — reads and returns file content as `text/plain`
   - Path validation: reject `..` segments, reject paths outside output dir

2. Create frontend:
   - `output/[scenario]/+page.svelte` — file list sidebar + content viewer
   - `TfViewer.svelte` — uses highlight.js with HCL grammar, read-only, monospace

3. Install: `npm install highlight.js`

**Tests**:
- `handlers_output_test.go`: list files, get content, path traversal rejected
- `ui/tests/output.spec.ts`: file list renders, click shows highlighted content

**Done when**: Can view generated .tf files with syntax highlighting. Tests pass.

---

### SUi-5: WebSocket infrastructure + log streaming

**Goal**: WebSocket connection established, log events stream to browser.

**Steps**:

1. Add dependency: `go get github.com/coder/websocket`

2. Create `internal/api/hub.go`:
   ```go
   type Hub struct {
       clients    map[*Client]bool
       broadcast  chan []byte
       register   chan *Client
       unregister chan *Client
   }
   func NewHub() *Hub { ... }
   func (h *Hub) Run(ctx context.Context) { ... }  // goroutine: select on channels
   func (h *Hub) Broadcast(msg []byte) { ... }       // non-blocking send to broadcast chan
   ```

3. Create `internal/api/client.go`:
   ```go
   type Client struct {
       hub  *Hub
       conn *websocket.Conn
       send chan []byte  // buffered (256)
   }
   func (c *Client) WritePump(ctx context.Context) { ... }  // drain send chan → conn.Write
   func (c *Client) ReadPump(ctx context.Context) { ... }   // read pings, unregister on close
   ```
   Drop message if `send` channel is full (slow client protection).

4. Create `internal/api/ws_sink.go`:
   ```go
   type WebSocketSink struct { hub *Hub }
   func (s *WebSocketSink) Write(p []byte) (int, error) {
       s.hub.Broadcast(p)
       return len(p), nil
   }
   ```

5. Add WebSocket upgrade handler to `server.go`:
   - `GET /api/ws` — upgrades HTTP to WebSocket, creates `Client`, registers with Hub

6. Create frontend:
   - `ws.ts` — WebSocket client with auto-reconnect (exponential backoff), typed message parsing
   - `stores/liveRun.ts` — writable store updated by WS messages
   - `LogStream.svelte` — terminal-style scrolling div, auto-scroll to bottom, max 1000 lines visible

**Tests**:
- `hub_test.go`: register, unregister, broadcast, slow client drop
- `ws_sink_test.go`: Write broadcasts
- `ui/tests/ws.spec.ts`: WebSocket connects, receives messages

**Done when**: Browser connects WebSocket, log entries stream when backend logs. Tests pass.

---

### SUi-6: Live run execution + pipeline visualization

**Goal**: Start a run from UI, watch stages animate in real time.

**Steps**:

1. **Extract `executeRunLoop`** from `internal/cli/run_command.go`:
   ```go
   // New exported function — the core run loop without Cobra dependency
   func executeRunLoop(ctx context.Context, runtime *CommandRuntime, scenarioPath string,
       sc scenario.Scenario, controls runControls, emitter EventEmitter) (OutputResult, error) {
       // Move lines 59-240 here (setup, iteration loop, holdout checks, result construction)
       // Replace cmd.Context() with ctx; return OutputResult instead of writeCommandOutput
       // Remove unused cmd parameter from runIteration call (line 100)
   }
   ```
   Then `runRunCommand` becomes:
   ```go
   func runRunCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
       controls, err := resolveRunControls(cmd, runtime)
       // ...
       result, err := executeRunLoop(cmd.Context(), runtime, scenarioPath, sc, controls, nil)
       // ... writeCommandOutput(cmd, result)
   }
   ```
   **Pitfall**: `runIteration` (line 308) already takes `ctx context.Context` — the `cmd *cobra.Command` param (line 314) is unused dead code. Simply remove it and update the call site at line 100.

2. Add optional `EventEmitter` callback to `executeRunLoop`:
   ```go
   type RunEvent struct {
       Type string      `json:"type"`
       Data interface{} `json:"data"`
   }
   type EventEmitter func(RunEvent)
   ```
   The run loop calls `emitter(RunEvent{Type: "stage_start", Data: ...})` at key points. `runRunCommand` passes `nil` (no-op). The API handler passes a function that JSON-encodes and sends to the WebSocket hub.

3. Create `internal/api/handlers_run_executor.go`:
   - `POST /api/runs/{scenario}/start` — validates scenario exists, acquires run mutex, returns 202
   - Spawns goroutine: creates `CommandRuntime` with `WebSocketSink` added to `Logger.sinks`
   - Creates `EventEmitter` that wraps events as `{"type": "...", "data": ...}` and sends to `hub.Broadcast()`
   - Calls `executeRunLoop(ctx, runtime, scenarioPath, sc, controls, emitter)`
   - Run mutex: `sync.Mutex` — only one run at a time. Returns 409 if busy.

4. Create frontend:
   - `PipelineView.svelte` — horizontal: three `StageNode` connected by arrows
   - `StageNode.svelte` — states: pending (gray), running (blue pulse CSS animation), pass (green), fail (red), skip (gray dashed)
   - `RunResultBanner.svelte` — large banner: green/red/yellow background + terminal reason text
   - `live/+page.svelte` — auto-shows active run (only one at a time). Connects to WS, shows PipelineView per iteration, stacks `IterationTimeline` vertically, shows `RunResultBanner` on completion, includes `LogStream`. If no run active, shows "No active run" with prompt to start one.

5. Update `scenarios/[...path]/+page.svelte` — add "Run" button that `POST`s to `/api/runs/{scenario}/start` then navigates to `/live`

**Tests**:
- `handlers_run_executor_test.go`: POST returns 202, concurrent 409, mock events sent
- `run_command_test.go`: existing tests pass after extraction
- `ui/tests/live.spec.ts`: run starts, pipeline animates, banner appears

**Done when**: Can start a run from UI and watch it live. Tests pass. Existing `run` command tests unbroken.

---

### SUi-7: Scenario YAML editor

**Goal**: Edit and save scenario YAML with CodeMirror 6 and validation feedback.

**Steps**:

1. Install: `npm install @codemirror/lang-yaml @codemirror/view @codemirror/state codemirror`

2. Create `YamlEditor.svelte`:
   - CodeMirror 6 instance with YAML language support
   - "Save" button that PUTs to `/api/scenarios/{path}`
   - Displays validation errors inline (below editor) on 422 response
   - Displays success confirmation on 200

3. Add PUT handler to `handlers_scenarios.go`:
   - Reads request body as YAML
   - Validates against scenario schema using `scenario.LoadWithSchema`
   - If invalid: returns 422 with error details
   - If valid: writes to disk, returns 200
   - Path traversal check: reject `..` segments

4. Update `scenarios/[...path]/+page.svelte` to use `YamlEditor` instead of `<pre>`

**Tests**:
- `handlers_scenarios_test.go`: PUT valid (200), PUT invalid schema (422), PUT bad YAML (400), PUT path traversal (403)
- `ui/tests/editor.spec.ts`: edit, save, validation error shown, fix, save success

**Done when**: Can edit and save scenarios with validation feedback. Tests pass.

---

### SUi-8: Build pipeline + polish

**Goal**: Single binary distribution works. Docs updated.

**Steps**:

1. Update `Makefile`:
   ```makefile
   ui-install:
   	cd ui && npm install
   ui-build: ui-install
   	cd ui && npm run build
   ui-dev:
   	cd ui && npm run dev
   ui-clean:
   	rm -rf ui/build ui/node_modules ui/.svelte-kit
   build: ui-build
   	$(GO) build -o bin/infrafactory ./cmd/infrafactory
   ```

2. Update `goreleaser.yml`:
   ```yaml
   before:
     hooks:
       - cmd: make ui-build
   ```

3. Update `README.md`:
   - Add `## Web UI` section documenting `infrafactory ui` command
   - Document dev workflow: `make ui-dev` + `go run -tags noui ./cmd/infrafactory ui --addr 127.0.0.1:4173`
   - Document production: `make build`

4. Run full verification:
   - `make build` succeeds
   - `./bin/infrafactory ui` serves full UI
   - `go test -tags noui ./...` passes
   - `go test ./...` passes
   - `goreleaser build --snapshot --clean` succeeds

**Tests**:
- All previous tests pass
- Build produces working binary

**Done when**: `goreleaser build --snapshot --clean` → binary serves full UI. All tests green.

---

## Testing Strategy

### Test matrix per slice

| Slice | Go unit tests | Go integration | Playwright e2e | Build check |
|---|---|---|---|---|
| SUi-1 | server, spa | — | — | `make build`, `go test -tags noui` |
| SUi-2 | handlers_scenarios | — | scenarios.spec | `npm run build` |
| SUi-3 | handlers_runs, runstore | — | runs.spec | `npm run build` |
| SUi-4 | handlers_output | — | output.spec | `npm run build` |
| SUi-5 | hub, ws_sink, client | ws round-trip | ws.spec | `npm run build` |
| SUi-6 | handlers_run_executor, run_command | run executor + ws | live.spec | `npm run build` |
| SUi-7 | handlers_scenarios (PUT) | — | editor.spec | `npm run build` |
| SUi-8 | — | — | — | `make build`, goreleaser |

### Invariants (check after every slice)

1. `go test -tags noui ./...` passes — **mandatory CI gate, every commit**
2. `go test ./...` passes — **only after `npm run build` has populated `ui/build/`** (see Canonical Contracts §5)
3. `cd ui && npm run build` produces valid output — **after any frontend change**
4. All REST handlers have `httptest`-based tests covering success + error paths
5. Path traversal tested on PUT scenarios, GET output

### Security tests

- Path traversal: `../../../etc/passwd` rejected on PUT scenarios and GET output
- Concurrent run: second `POST /api/runs/*/start` returns 409
- Config redaction: API keys / tokens not in `/api/config` response
- Bind address: default is `127.0.0.1` (not `0.0.0.0`)

---

## Pitfalls and Gotchas

1. **`embed.FS` path prefix**: `go:embed all:ui/build` creates paths like `ui/build/index.html`. The SPA handler must strip the `ui/build/` prefix using `fs.Sub(assets, "ui/build")`.

2. **`runIteration` dead Cobra parameter**: `runIteration` (line 308) already takes `ctx context.Context` as its first parameter. It also accepts `cmd *cobra.Command` (line 314) but **never uses it** in the function body — it is dead code. Simply remove the `cmd` parameter from the signature and update the call site at line 100. This avoids importing Cobra in `internal/api`.

3. **Build tag isolation**: `go test -tags noui ./...` must work without Node.js installed. The `embed.go` / `embed_dev.go` split ensures this. The `noui` build returns nil `fs.FS`. The `ui` command is always registered (see Canonical Contracts §3) — when assets are nil, the server runs in API-only mode (SPA requests return 404 with dev-mode message).

4. **SPA routing vs API**: The SPA handler must NOT catch `/api/*` requests. Register API routes first on the mux, then SPA as catch-all.

5. **WebSocket slow clients**: If a client's `send` channel is full (256 buffer), drop the message. Do NOT block the broadcast goroutine.

6. **Config loading for API**: The `ui` command needs to load config to pass to handlers (for scenario paths, output paths, runstore root). Reuse the same `config.Load()` path.

7. **CORS not needed**: SvelteKit dev server proxies `/api` to Go server, so no CORS headers needed. Production serves everything from same origin.

8. **`RunMetadata` lacks `terminal_reason`**: The current `RunMetadata` struct only has `scenario`, `run_id`, `status`, `started_at`. The run list API needs `terminal_reason`. Two options: (a) add `TerminalReason string` to `RunMetadata` and persist it in `run.json` — cleanest, do this. (b) derive from last iteration's failures. The `runRunCommand` already knows the terminal reason — pass it through to `WriteRunMetadata`. Backward compat: existing `run.json` files without the field will show empty terminal_reason.

9. **Dashboard needs last-run-status per scenario**: The dashboard grid cards show last run badge per scenario. Rather than a separate API, `GET /api/scenarios` should include `last_run: {run_id, status, terminal_reason}` per scenario by reading the latest run from runstore. This avoids N+1 fetches on the dashboard.

10. **WebSocket event emission from run loop**: The `executeRunLoop` already logs structured `LogEntry` events via `runtime.Logger.Log()`. The `WebSocketSink` receives these as raw JSON bytes. For typed UI events (`stage_start`, `stage_complete`, `run_complete`), add an optional `EventEmitter` callback to `executeRunLoop` or to the runtime. The API handler sets this callback to send typed JSON messages to the hub. This keeps the run loop clean — it emits events via callback, the callback wraps them in `{"type": "...", "data": ...}` and sends to hub.

11. **`handlers_config.go` uses allowlist, not denylist**: Do NOT serialize the full `config.Config` and then redact. Instead, build a separate response struct containing only the allowlisted fields (see Canonical Contracts §4). This ensures new config fields are omitted by default. The config struct doesn't store API keys (OpenRouter key is from env var), but Mockway URLs, project IDs, and file paths are still internal details that shouldn't leak.

---

## Dependency Chain

```
SUi-1 (skeleton)
  ├── SUi-2 (scenarios) ── SUi-7 (YAML editor)
  ├── SUi-3 (run history)
  ├── SUi-4 (code viewer)
  └── SUi-5 (WebSocket) ── SUi-6 (live run)
                                    └── SUi-8 (build + polish)
```

SUi-2, SUi-3, SUi-4, SUi-5 can run in parallel after SUi-1.
SUi-6 depends on SUi-5. SUi-7 depends on SUi-2.
SUi-8 depends on SUi-6 and SUi-7.

---

## Agent Execution Rules

These rules apply to ALL Slice 21 sub-tickets:

1. **`noui` tag always**: Until `ui/build/` exists, use `go build -tags noui` and `go test -tags noui ./...`. After `npm run build` populates `ui/build/`, both tagged and untagged builds work.
2. **Two-terminal dev workflow**: Run Go API server (`go run -tags noui ./cmd/infrafactory ui --addr 127.0.0.1:4173`) in one terminal and Vite dev server (`cd ui && npm run dev`) in another. Vite proxies `/api` to `127.0.0.1:4173`. The Go server serves API-only (non-API paths return 404 since assets=nil). Frontend hot-reloads, Go server restarts on recompile.
3. **Test invariant**: `go test -tags noui ./...` must pass after every commit — this is the CI gate. `go test ./...` (without tag) only passes after `npm run build`.
4. **No CLI regression**: Existing commands (`run`, `validate`, `generate`, `test`, `mock`) must remain unaffected. The `ui` subcommand is always registered (API-only when assets=nil per Canonical Contracts §3).
5. **Path safety**: All handlers serving files from disk must reject `..` path segments. Test path traversal in every handler that reads from `cfg.Paths.*`.
6. **Single dependency**: The only new Go dependency is `github.com/coder/websocket`. Do not add HTTP routers, logging frameworks, or other dependencies.
