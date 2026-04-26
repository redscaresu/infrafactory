# Plan: Slices 33-39 — Cross-Repo E2E, Pitfall Learning, Enriched Feedback, GCP, and UI Features

## Context

Slices 33-39 cover six themes: cross-repo end-to-end testing (Slice 33), smarter pitfall learning from failures (Slice 34), richer http_probe diagnostics (Slice 35), GCP multi-cloud support (Slice 36), and three UI features — pitfalls management (Slice 37), run comparison (Slice 38), and real-time scenario validation (Slice 39).

ADR-0013 covers the two cross-cutting architectural decisions: the cross-repo test strategy and the GCP multi-cloud approach.

## Quick Reference

| Key | Value |
|---|---|
| Slices | 33-39 |
| Ticket IDs | S33-T1 through S39-T4 |
| Depends on | Slice 32 (done) |
| ADR | 0013 (cross-repo e2e and multi-cloud) |

---

## Slice 33: Cross-Repo E2E Test

Infrafactory and mockway are separate repos. Bugs that span both (e.g., `ip_ids` vs `ip_id`, redis port mismatches) are only caught by manual runs. A Go integration test that starts mockway from source and runs `infrafactory run` against a scenario catches these regressions automatically.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S33-T1 | Test infrastructure: start mockway from source, helper to run infrafactory commands | P1 | — |
| S33-T2 | E2e test for web-app-paris (simplest scenario with topology checks) | P1 | S33-T1 |
| S33-T3 | E2e test for full-stack-paris (complex scenario covering all resource types) | P1 | S33-T1 |

### Acceptance Criteria

- S33-T1: `TestMain` or helper starts mockway binary from `../mockway` source, waits for health check, tears down after tests. Helper wraps `go run ./cmd/infrafactory run` with configurable scenario path.
- S33-T2: `TestE2E_WebAppParis` runs web-app-paris scenario end-to-end, asserts `target_reached` in run output, verifies topology checks pass.
- S33-T3: `TestE2E_FullStackParis` runs full-stack-paris, asserts all resource types created (compute, VPC, LB, K8s, RDB, IAM, registry, redis).

### Key Files

- `internal/e2e/e2e_test.go` (new) — test infrastructure + tests
- `internal/e2e/helpers.go` (new) — mockway startup, infrafactory runner

---

## Slice 34: Pitfall Learning from Failed Runs

Currently, pitfall auto-learning only triggers on successful self-correction (`target_reached`). If the run oscillates (same error alternating across iterations) and eventually exhausts its budget, the pitfall is never learned. This slice adds oscillation detection and learns from failures too.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S34-T1 | Detect oscillation patterns in run loop (same failure signature repeating) | P1 | — |
| S34-T2 | Extract and append pitfalls from oscillation failures (not just target_reached) | P1 | S34-T1 |
| S34-T3 | Tests for oscillation detection and failed-run learning | P1 | S34-T2 |

### Acceptance Criteria

- S34-T1: Run loop detects when a failure signature appears in iteration N, disappears in N+1, reappears in N+2 (oscillation). Exposed as `DetectOscillation(history []IterationResult) []FailureSignature`.
- S34-T2: When a run ends with `repair_budget_exhausted` or `stuck` and oscillation is detected, extract pitfalls from the oscillating failure signatures and append to `pitfalls/{cloud}.yaml` with `source: learned`.
- S34-T3: Unit tests for oscillation detection (no oscillation, simple oscillation, multiple oscillating signatures). Integration test: run with known oscillating failure produces a learned pitfall.

### Key Files

- `internal/cli/run_command.go` — oscillation detection hook
- `internal/generator/pitfalls.go` — extend `ExtractLearnedPitfall` for failure cases
- `internal/cli/run_command_test.go` — oscillation detection tests

---

## Slice 35: Better http_probe Feedback

When `http_probe` fails, the topology derivation knows exactly why (no frontend on that port, no backend attached, no public IP assigned) but the failure message is generic ("http_probe load_balancer:80 expected reachable, got unreachable"). This slice enriches the message with the specific missing link.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S35-T1 | Add diagnostic detail to DeriveTopology when http_probe is false | P1 | — |
| S35-T2 | Include diagnostic in topology evaluation failure messages | P1 | S35-T1 |
| S35-T3 | Tests verifying enriched error messages | P1 | S35-T2 |

### Acceptance Criteria

- S35-T1: `DeriveTopology` returns a `Diagnostics map[string]string` alongside topology maps. Each false http_probe entry gets a diagnostic like "no frontend on port 80" or "no public IP on LB".
- S35-T2: `EvaluateTopology` includes diagnostic text in failure detail when an http_probe criterion fails.
- S35-T3: Unit tests with fixture data covering all diagnostic cases (no frontend, no backend, no IP, frontend exists but wrong port).

### Key Files

- `internal/harness/topology_derive.go` — diagnostic generation
- `internal/harness/mock_deploy.go` — pass diagnostics to evaluation
- `internal/harness/topology_derive_test.go` — diagnostic tests

---

## Slice 36: GCP Support

Add `cloud: gcp` scenario type with full validation parity against Scaleway. This requires: fakegcp AGENTS.md, schema updates, prompt reorganization, GCP prompt templates, GCP topology derivation, GCP OPA policies, GCP pitfalls, mock client generalization, real probe patterns, training scenarios, unit tests, integration tests, double-apply verification, and Playwright e2e tests.

**Prerequisite in fakegcp repo**: Create `AGENTS.md` matching mockway's conventions (provider SDK as contract, double-apply idempotency, SQLite single-connection, naming conventions). Add `google_compute_forwarding_rule` handler if not present (needed for LB topology).

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S36-T0 | fakegcp: create AGENTS.md matching mockway conventions + add forwarding_rule handler | P1 | — |
| S36-T1 | Reorganize prompts: move `prompts/*.md` → `prompts/scaleway/`, update generator template paths | P1 | — |
| S36-T2 | Update `scenario.schema.json`: add `"gcp"` to cloud enum, add GCP resource definitions with conditional validation | P1 | — |
| S36-T3 | GCP prompt templates (`prompts/gcp/phase1,2,3.md`) with GCP provider conventions | P1 | S36-T1 |
| S36-T4 | GCP topology derivation: add cloud dispatch to `DeriveTopology`, GCP resource patterns | P1 | — |
| S36-T5 | GCP topology derivation unit tests with fixture data (same coverage as Scaleway: 10+ tests) | P1 | S36-T4 |
| S36-T6 | `pitfalls/gcp.yaml` with initial GCP pitfalls (machine_type, self_link, SQL naming, GKE version, firewall) | P1 | — |
| S36-T7 | `policies/gcp/` OPA policies: no_public_sql, vpc_required, region_restriction, encryption | P1 | — |
| S36-T8 | Generalize `mockway_client.go` to support fakegcp admin endpoints (/mock/state, /mock/reset, /mock/snapshot) based on scenario cloud field | P1 | — |
| S36-T9 | Add GCP resource patterns to `real_probe.go` (google_compute_instance, google_sql_database_instance, google_compute_forwarding_rule) | P1 | S36-T4 |
| S36-T10 | GCP training scenarios (gcp-vm-network, gcp-gke-cluster, gcp-cloud-sql) | P1 | S36-T2, S36-T3, S36-T6, S36-T7 |
| S36-T11 | Cross-repo e2e tests against fakegcp (reuse S33-T1 infrastructure) + double-apply idempotency | P1 | S33-T1, S36-T10 |
| S36-T12 | Playwright e2e: GCP scenarios appear in UI scenario list, can be browsed | P1 | S36-T10 |

### Acceptance Criteria

- S36-T0: fakegcp has `AGENTS.md` with same sections as mockway (Architecture, Testing, Conventions). `google_compute_forwarding_rule` handler exists with CRUD + idempotent apply.
- S36-T1: Existing Scaleway prompts moved to `prompts/scaleway/`. Generator loads templates from `prompts/{cloud}/`. `EnsureProviderSchema` dispatches by cloud to extract the correct provider schema (`scaleway/scaleway` or `hashicorp/google`) via `tofu providers schema`. All 12 Scaleway scenarios still pass.
- S36-T2: `scenario.schema.json` accepts `cloud: gcp`. GCP scenarios validate with GCP-specific resource definitions (compute, networking, database, kubernetes, storage, iam).
- S36-T3: GCP prompt templates exist under `prompts/gcp/`. Phase 2 includes `{{.Pitfalls}}` and GCP-specific pitfalls section. Phase 3 includes GCP self-review checklist.
- S36-T4: `DeriveTopology` dispatches by cloud. GCP derivation handles: `google_compute_instance`, `google_compute_forwarding_rule`, `google_sql_database_instance`, `google_container_cluster`.
- S36-T5: 10+ unit tests for GCP topology derivation with fixture data (same patterns as Scaleway: full web-app, no LB, public DB, empty state, etc.).
- S36-T6: `pitfalls/gcp.yaml` has at least 5 pitfalls. Auto-loaded when `cloud: gcp`.
- S36-T7: At least 4 OPA policies under `policies/gcp/` covering: no public Cloud SQL, VPC required for GKE, region restriction, encryption.
- S36-T8: Mock client detects cloud from scenario and calls correct mock server. Reset/state/snapshot work for both mockway and fakegcp.
- S36-T9: `real_probe.go` handles GCP resource types for IP extraction from terraform state.
- S36-T10: At least 3 GCP training scenarios pass `infrafactory run` against fakegcp.
- S36-T11: Cross-repo e2e test starts fakegcp from source, runs GCP scenario, verifies success. Double-apply: `terraform apply` twice, second is no-op.
- S36-T12: Playwright test navigates to a GCP scenario page, verifies scenario name and YAML content load.

### Execution Order

```
S36-T0 (fakegcp AGENTS.md) ─── first (in fakegcp repo)
S36-T1 (prompt reorg) ─────┐
S36-T2 (schema) ───────────┤
S36-T4 (topology derive) ──┼── parallel, no deps
S36-T6 (pitfalls) ──────────┤
S36-T7 (OPA policies) ─────┤
S36-T8 (mock client) ──────┘
S36-T3 (GCP prompts) ────────── depends on T1
S36-T5 (topology tests) ─────── depends on T4
S36-T9 (real probes) ────────── depends on T4
S36-T10 (training scenarios) ── depends on T2, T3, T6, T7
S36-T11 (cross-repo e2e) ────── depends on S33-T1, T10
S36-T12 (Playwright e2e) ────── depends on T10
```

### Key Files

- `../fakegcp/AGENTS.md` (new, in fakegcp repo)
- `internal/generator/claude_adapter.go` — template path dispatch by cloud
- `internal/generator/openrouter_adapter.go` — same
- `internal/cli/runtime.go` — provider schema extraction dispatch by cloud
- `prompts/scaleway/phase*.md` (moved from `prompts/`)
- `prompts/gcp/phase1_plan_architecture.md` (new)
- `prompts/gcp/phase2_generate_hcl.md` (new)
- `prompts/gcp/phase3_self_review.md` (new)
- `scenario.schema.json` — add GCP cloud + resources
- `internal/harness/topology_derive.go` — GCP derivation + cloud dispatch
- `internal/harness/topology_derive_test.go` — GCP fixture tests
- `internal/harness/real_probe.go` — GCP resource patterns
- `internal/cli/mockway_client.go` — generalize for multi-cloud mock
- `pitfalls/gcp.yaml` — GCP pitfalls
- `policies/gcp/*.rego` — GCP OPA policies
- `scenarios/training/gcp-*.yaml` — GCP training scenarios

---

## Slice 37: Pitfalls UI

A `/pitfalls` page in the web UI showing all pitfalls with source (static/learned), resource, and provider. Supports editing so operators can curate learned pitfalls without editing YAML files manually.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S37-T1 | GET /api/pitfalls endpoint returning pitfalls grouped by provider | P1 | — |
| S37-T2 | PUT /api/pitfalls/{provider} endpoint for editing | P1 | S37-T1 |
| S37-T3 | /pitfalls UI page with table, source badges, and edit form | P1 | S37-T2 |
| S37-T4 | Playwright e2e tests for pitfalls page | P1 | S37-T3 |

### Acceptance Criteria

- S37-T1: `GET /api/pitfalls` returns JSON with pitfalls grouped by provider, each pitfall including resource, rule, source, and optional discovered_from.
- S37-T2: `PUT /api/pitfalls/{provider}` accepts updated pitfalls array, writes to `pitfalls/{provider}.yaml`. Validates structure before writing.
- S37-T3: UI page shows pitfalls in a table with provider tabs, source badges ("static" / "learned"), and inline edit form. Save button calls PUT endpoint.
- S37-T4: Playwright tests: page loads pitfalls, edit a pitfall, save, reload and verify persistence.

### Key Files

- `internal/api/handlers_pitfalls.go` (new)
- `internal/api/handlers_pitfalls_test.go` (new)
- `ui/src/routes/pitfalls/+page.svelte` (new)

---

## Slice 38: Run Comparison

A UI page that diffs two runs side-by-side using existing per-run IaC snapshots. Helps operators understand what changed between runs.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S38-T1 | GET /api/runs/{scenario}/compare?run1=X&run2=Y endpoint returning diff data | P1 | — |
| S38-T2 | /compare UI page with dual-pane IaC diff viewer | P1 | S38-T1 |
| S38-T3 | Playwright e2e tests | P1 | S38-T2 |

### Acceptance Criteria

- S38-T1: Endpoint returns file-level diffs between two runs. Each diff entry has filename, status (added/removed/modified), and unified diff text.
- S38-T2: UI page has run selectors (dropdowns populated from run history), dual-pane diff view with syntax highlighting, and file list sidebar.
- S38-T3: Playwright tests: select two runs, verify diff renders, verify file list matches expected changes.

### Key Files

- `internal/api/handlers_compare.go` (new)
- `internal/api/handlers_compare_test.go` (new)
- `ui/src/routes/compare/+page.svelte` (new)

---

## Slice 39: Real-Time Scenario Validation in UI

Validate scenario YAML as the user types (debounced), showing errors inline instead of only on save. Gives immediate feedback on schema violations.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S39-T1 | POST /api/scenarios/validate endpoint (validates without saving) | P1 | — |
| S39-T2 | Debounced validation in scenario page textarea (500ms delay) | P1 | S39-T1 |
| S39-T3 | Inline error display below textarea | P1 | S39-T2 |
| S39-T4 | Playwright e2e tests | P1 | S39-T3 |

### Acceptance Criteria

- S39-T1: `POST /api/scenarios/validate` accepts raw YAML body, returns `{valid: bool, errors: [{line: int, message: string}]}`. Does not write to disk.
- S39-T2: Scenario edit page sends validation request 500ms after last keystroke. Cancels in-flight requests on new input.
- S39-T3: Errors display below the textarea with line numbers. Valid state shows a green check.
- S39-T4: Playwright tests: type invalid YAML, verify error appears; fix it, verify error clears.

### Key Files

- `internal/api/handlers_scenarios.go` — add validate endpoint
- `internal/api/handlers_scenarios_test.go` — validate tests
- `ui/src/routes/scenarios/[...path]/+page.svelte` — debounced validation + inline errors

---

## Execution Order

```
Slice 33 (cross-repo e2e)  ─── independent, can start immediately
Slice 34 (pitfall learning) ── independent, can start immediately
Slice 35 (http_probe feedback) ── independent, can start immediately

Slice 36 (GCP support) ──────── independent, can start immediately
                                  T1/T2/T3 parallel → T4 → T5

Slice 37 (pitfalls UI) ──────── independent, can start immediately
Slice 38 (run comparison) ───── independent, can start immediately
Slice 39 (scenario validation) ── independent, can start immediately
```

All 7 slices are independent of each other and can be executed in any order.

---

## Slice 40: Visual UI Regression Testing

Use Playwright's screenshot comparison and functional assertions to catch UI regressions. Browse each page, spot visual/layout/data issues, and write targeted tests.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S40-T1 | Playwright visual snapshots: capture baselines for all pages (home, scenario, live, runs, diagnostics) | P1 | — |
| S40-T2 | Visual regression tests: screenshot comparison with pixel diff threshold, fail on unexpected changes | P1 | S40-T1 |
| S40-T3 | Functional spot-checks: verify data rendering (YAML loads, run metadata displays, iteration timeline populates, stage pills correct) | P1 | — |
| S40-T4 | Error state coverage: empty states, 404 pages, API errors, missing scenarios, stale run data | P1 | — |

### Acceptance Criteria

- S40-T1: Baselines captured for `/`, `/scenarios/training/web-app-paris`, `/live` (completed run), `/runs`, `/diagnostics`. Stored in `ui/e2e/screenshots/`.
- S40-T2: `npx playwright test --update-snapshots` updates baselines. Tests fail when pixel diff > 0.1%. Part of `make test`.
- S40-T3: Tests verify: scenario page YAML matches file content, run metadata card has all fields, iteration timeline count matches run.json, stage pills show correct pass/fail.
- S40-T4: Tests for: empty scenario list, non-existent scenario URL, Live page with no runs, Diagnostics readiness checks, run with zero iterations.

### Key Files

- `ui/e2e/visual.spec.ts` (new)
- `ui/e2e/functional.spec.ts` (new)
- `ui/e2e/error-states.spec.ts` (new)
- `ui/e2e/screenshots/` (baseline images)
- `ui/playwright.config.ts` (add screenshot comparison config)

---

---

## Slice 41: fakegcp Test Coverage Parity

Bring fakegcp to mockway-level test coverage so it's reliable enough for a blog post with the same guarantees as mockway. mockway has 280+ tests, 22 Terraform examples, 74.9% coverage. fakegcp currently has 52 handler tests, 4 examples, no repository tests.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S41-T0 | Initialize git repo, commit existing code, push to GitHub | P1 | — |
| S41-T1 | Test infrastructure: testutil helpers, coverage reporting, Makefile test targets | P1 | — |
| S41-T2 | Repository unit tests (CRUD for all 13 tables, schema migration, FK enforcement) | P1 | — |
| S41-T3 | FK violation tests for all cross-resource references (15+ tests) | P1 | S41-T1 |
| S41-T4 | FK cascade delete tests for all parent-child relationships | P1 | S41-T1 |
| S41-T5 | Admin endpoint tests (/mock/state, /mock/reset, /mock/snapshot, /mock/restore) | P1 | S41-T1 |
| S41-T6 | Double-apply idempotency automation for all working Terraform examples | P1 | S41-T1 |
| S41-T7 | Misconfigured Terraform examples demonstrating FK violations, wrong refs, ordering | P1 | S41-T1 |

### Acceptance Criteria

- S41-T0: fakegcp has a GitHub repo with CI-ready state. All existing code committed.
- S41-T1: `go test ./...` runs all tests. `go test -cover ./...` reports coverage. Makefile has `test`, `test-cover`, `test-e2e` targets. testutil provides `NewTestServer`, HTTP helpers matching mockway's pattern.
- S41-T2: Repository tests cover Create/Get/List/Update/Delete for all tables: compute_instances, compute_networks, compute_subnetworks, compute_firewalls, compute_disks, compute_addresses, container_clusters, container_node_pools, sql_instances, sql_databases, sql_users, iam_service_accounts, iam_sa_keys, storage_buckets, operations.
- S41-T3: FK violation tests: instance→network, subnetwork→network, firewall→network, node_pool→cluster, sql_database→instance, sql_user→instance, sa_key→service_account. Each returns 404 when parent doesn't exist.
- S41-T4: Cascade delete tests: cluster→node_pools, sql_instance→databases+users. Verify child resources are deleted when parent is deleted.
- S41-T5: Admin tests: `/mock/reset` clears all tables, `/mock/state` returns all resources grouped by service, `/mock/snapshot` + `/mock/restore` preserves/restores state, `/mock/state/{service}` returns service-specific state.
- S41-T6: For each `examples/working/` directory: `tofu init && tofu apply && tofu apply` (second apply must be no-op with 0 changes). Gated by env var `FAKEGCP_ENABLE_E2E=1`.
- S41-T7: At least 5 misconfigured examples: instance missing network, subnetwork wrong network ref, node pool missing cluster, SQL database missing instance, firewall wrong network. Each demonstrates a 404/409 that `terraform validate/plan` cannot catch.

### Key Files (all in fakegcp repo)

- `repository/repository_test.go` (new)
- `handlers/handlers_test.go` (expand from 52 to 150+ tests)
- `examples/working/` (expand from 4 to 8+)
- `examples/misconfigured/` (expand from 1 to 6+)
- `scripts/e2e.sh` (new — double-apply automation)
- `Makefile` (add test targets)

---

## Verification

```bash
# After each slice:
go build -tags noui ./...
go test -tags noui ./...
# For UI slices (37, 38, 39):
cd ui && npm test && npm run build
```

## Out of Scope

- AWS support (future, same pattern as GCP)
- Pitfall promotion workflow (learned -> static review UI)
- Three-way run comparison
- Scenario validation against live cloud state
