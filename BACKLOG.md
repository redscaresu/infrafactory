# BACKLOG

Single source of ticket status across slices.

Legend: `todo` | `in_progress` | `blocked` | `done`

| id | slice | title | priority | status | deps | owner |
|---|---|---|---|---|---|---|
| S42-T5 | Slice 42 | Playwright e2e: GCP sidebar group, cloud badge, credentials, mock status | P1 | todo | S42-T2 | — |
| S42-T4 | Slice 42 | API: GET /api/scenarios returns cloud field, layer3-status adapts per cloud | P1 | todo | S36-T8 | — |
| S42-T3 | Slice 42 | UI scenario page: mock server status based on cloud (mockway vs fakegcp) | P1 | todo | S42-T2, S36-T8 | — |
| S42-T2 | Slice 42 | UI scenario page: cloud provider badge, dynamic Layer 3 label and credentials | P1 | todo | S42-T1 | — |
| S42-T1 | Slice 42 | UI sidebar: group scenarios by cloud provider (Scaleway / GCP sections) | P1 | todo | — | — |
| S41-T7 | Slice 41 | fakegcp: misconfigured Terraform examples (FK violations, wrong refs, ordering) | P1 | todo | S41-T1 | — |
| S41-T6 | Slice 41 | fakegcp: double-apply idempotency automation for all working examples | P1 | todo | S41-T1 | — |
| S41-T5 | Slice 41 | fakegcp: admin endpoint tests (/mock/state, /mock/reset, /mock/snapshot, /mock/restore) | P1 | todo | S41-T1 | — |
| S41-T4 | Slice 41 | fakegcp: FK cascade delete tests for all parent-child relationships | P1 | todo | S41-T1 | — |
| S41-T3 | Slice 41 | fakegcp: FK violation tests for all cross-resource references | P1 | todo | S41-T1 | — |
| S41-T2 | Slice 41 | fakegcp: repository unit tests (CRUD, schema migration, FK enforcement) | P1 | todo | — | — |
| S41-T1 | Slice 41 | fakegcp: test infrastructure — testutil helpers, coverage reporting, Makefile test targets | P1 | todo | — | — |
| S41-T0 | Slice 41 | fakegcp: initialize git repo, commit existing code, push to GitHub | P1 | todo | — | — |
| S40-T4 | Slice 40 | Error state coverage: empty states, 404 pages, API errors, missing scenarios | P1 | todo | — | — |
| S40-T3 | Slice 40 | Functional spot-checks: verify data rendering across all pages | P1 | todo | — | — |
| S40-T2 | Slice 40 | Visual regression tests: screenshot comparison with pixel diff threshold | P1 | todo | S40-T1 | — |
| S40-T1 | Slice 40 | Playwright visual snapshots: capture baselines for all pages | P1 | todo | — | — |
| S39-T4 | Slice 39 | Playwright e2e tests for real-time validation | P1 | todo | S39-T3 | — |
| S39-T3 | Slice 39 | Inline error display below textarea | P1 | done | S39-T2 | claude |
| S39-T2 | Slice 39 | Debounced validation in scenario page textarea (500ms delay) | P1 | done | S39-T1 | claude |
| S39-T1 | Slice 39 | POST /api/scenarios/validate endpoint (validates without saving) | P1 | done | — | claude+codex |
| S38-T3 | Slice 38 | Playwright e2e tests for run comparison | P1 | todo | S38-T2 | — |
| S38-T2 | Slice 38 | /compare UI page with dual-pane IaC diff viewer | P1 | done | S38-T1 | claude |
| S38-T1 | Slice 38 | GET /api/runs/{scenario}/compare endpoint returning diff data | P1 | done | — | claude |
| S37-T4 | Slice 37 | Playwright e2e tests for pitfalls page | P1 | done | S37-T3 | claude+codex |
| S37-T3 | Slice 37 | /pitfalls UI page with table, source badges, and edit form | P1 | done | S37-T2 | claude+codex |
| S37-T2 | Slice 37 | PUT /api/pitfalls/{provider} endpoint for editing | P1 | done | S37-T1 | claude |
| S37-T1 | Slice 37 | GET /api/pitfalls endpoint returning pitfalls grouped by provider | P1 | done | — | claude |
| S36-T12 | Slice 36 | Playwright e2e: GCP scenarios appear in UI scenario list | P1 | todo | S36-T10 | — |
| S36-T11 | Slice 36 | Cross-repo e2e tests against fakegcp + double-apply idempotency | P1 | todo | S33-T1, S36-T10, S41-T1 | — |
| S36-T10 | Slice 36 | GCP training scenarios (gcp-vm-network, gcp-gke-cluster, gcp-cloud-sql, gcp-full-stack) | P1 | done | S36-T2, S36-T3, S36-T6, S36-T7, S36-T8 | claude |
| S36-T9 | Slice 36 | Add GCP resource patterns to real_probe.go | P1 | done | S36-T4 | claude |
| S36-T8 | Slice 36 | Generalize mockway_client.go for multi-cloud mock (fakegcp admin endpoints) | P1 | done | — | claude |
| S36-T7 | Slice 36 | GCP OPA policies: no_public_sql, vpc_required, region_restriction, encryption | P1 | done | — | claude+codex |
| S36-T6 | Slice 36 | pitfalls/gcp.yaml with initial GCP pitfalls | P1 | done | — | claude+codex |
| S36-T5 | Slice 36 | GCP topology derivation unit tests (10+ tests with fixture data) | P1 | done | S36-T4 | claude |
| S36-T4 | Slice 36 | GCP topology derivation: cloud dispatch + GCP resource patterns | P1 | done | — | claude |
| S36-T3 | Slice 36 | GCP prompt templates (prompts/gcp/phase1,2,3.md) | P1 | done | S36-T1 | claude+codex |
| S36-T2 | Slice 36 | Update scenario.schema.json: add gcp to cloud enum, GCP resource definitions | P1 | done | — | claude+codex |
| S36-T1 | Slice 36 | Reorganize prompts: move prompts/*.md → prompts/scaleway/, update generator paths | P1 | done | — | claude |
| S36-T0 | Slice 36 | fakegcp: create AGENTS.md + add forwarding_rule handler (in fakegcp repo) | P1 | todo | — | — |
| S35-T3 | Slice 35 | Tests verifying enriched http_probe error messages | P1 | done | S35-T2 | claude+codex |
| S35-T2 | Slice 35 | Include diagnostic in topology evaluation failure messages | P1 | done | S35-T1 | claude+codex |
| S35-T1 | Slice 35 | Add diagnostic detail to DeriveTopology when http_probe is false | P1 | done | — | claude+codex |
| S34-T3 | Slice 34 | Tests for oscillation detection and failed-run learning | P1 | done | S34-T2 | claude |
| S34-T2 | Slice 34 | Extract and append pitfalls from oscillation failures | P1 | done | S34-T1 | claude |
| S34-T1 | Slice 34 | Detect oscillation patterns in run loop (same failure signature repeating) | P1 | done | — | claude |
| S33-T3 | Slice 33 | E2e test for full-stack-paris (all resource types) | P1 | done | S33-T1 | claude |
| S33-T2 | Slice 33 | E2e test for web-app-paris (simplest scenario with topology checks) | P1 | done | S33-T1 | claude |
| S33-T1 | Slice 33 | Test infrastructure: start mockway from source, infrafactory command helper | P1 | done | — | claude |
| S32-T1 | Slice 32 | Create `pitfalls/scaleway.yaml` with all 16 existing pitfalls, remove from prompt templates | P1 | done | — | codex |
| S32-T2 | Slice 32 | Add `Pitfalls` + `Cloud` to generator, load by cloud provider from `pitfalls/{cloud}.yaml` | P1 | done | S32-T1 | codex |
| S32-T3 | Slice 32 | Tests: pitfalls loading, rendering, empty file handling, scenario verification | P1 | done | S32-T2 | codex |
| S32-T4 | Slice 32 | Update CONCEPT.md, AGENTS.md, README.md for dynamic pitfalls | P1 | done | S32-T2 | codex |
| S32-T5 | Slice 32 | Design doc: auto-learning pitfalls from run feedback (future, no implementation) | P1 | done | S32-T3 | codex |
| S31-T1 | Slice 31 | ADR-0011 + CONCEPT.md topology derivation docs + BACKLOG/ROADMAP | P1 | done | S30-T5 | codex |
| S31-T2 | Slice 31 | Implement `DeriveTopology()` in `internal/harness/topology_derive.go` | P1 | done | S31-T1 | codex |
| S31-T3 | Slice 31 | Wire derivation into `EvaluateTopology()` with auto-detection of raw state | P1 | done | S31-T2 | codex |
| S31-T4 | Slice 31 | Unit tests + fixtures for topology derivation (11 test cases) | P1 | done | S31-T3 | codex |
| S31-T5 | Slice 31 | Playwright e2e: topology results visible on Live page iteration timeline | P1 | done | S31-T3 | codex |
| S31-T6 | Slice 31 | Integration verification: all 12 training scenarios pass with derived topology | P1 | done | S31-T4, S31-T5 | codex |
| S30-T1 | Slice 30 | Capture `plan-live.txt` artifact during sandbox deploy (tofu plan before apply) | P1 | done | M36 | codex |
| S30-T2 | Slice 30 | Auto-destroy real resources on failed runs (billing protection, Contract #14) | P1 | done | S30-T1 | codex |
| S30-T3 | Slice 30 | Validate generated HCL includes `scaleway_account_project` when Layer 3 enabled | P1 | done | M36 | codex |
| S30-T4 | Slice 30 | Verify holdout checks execute Layer 3 dual-apply pattern (Contract #10) | P1 | done | M36 | codex |
| S30-T5 | Slice 30 | Close S9-T8 governance ticket + update STATUS/BACKLOG/CURRENT_TICKET docs | P1 | done | S30-T2 | codex |
| M36 | Maintenance | Apply second-pass post-slice 22-29 hardening fixes and regressions | P1 | done | M35 | codex |
| M35 | Maintenance | Remediate post-slice 22-29 review findings and add regressions | P1 | done | S29-T2 | codex |
| S29-T1 | Slice 29 | UI: Layer 3 toggle on scenario page + credential status indicator | P1 | done | S28-T1 | codex |
| S29-T2 | Slice 29 | UI: Layer 3 progress + real probe results on Live page; layer3-status endpoint; update README Web UI section | P1 | done | S29-T1 | codex |
| S28-T1 | Slice 28 | Incremental E2E against real Scaleway (3-stage --no-destroy + final destroy, opt-in) | P1 | done | S27-T1, S24-T2 | codex |
| S27-T1 | Slice 27 | Real network probes (connectivity, http_probe, dns_resolution) for Layer 3; update README probe config + criteria status | P1 | done | S26-T2 | codex |
| S26-T1 | Slice 26 | Dual-apply harness: Layer 3 tofu apply with separate terraform-live.tfstate + destroy behavior | P1 | done | S23-T1 | codex |
| S26-T2 | Slice 26 | Layer 3 config/credential validation, prompt updates for project bootstrap, unit + opt-in integration tests; update README Layer 3 docs | P1 | done | S26-T1 | codex |
| S25-T1 | Slice 25 | Backend: persist plan.txt and baseline_state.json artifacts; add GET /api/.../plan and /api/.../baseline endpoints | P1 | done | S24-T2 | codex |
| S25-T2 | Slice 25 | Backend: accept no_destroy and clean flags in POST /api/runs/{scenario}/start; add GET /api/scenarios/{scenario}/run-mode detection endpoint | P1 | done | S25-T1 | codex |
| S25-T3 | Slice 25 | Frontend: add --no-destroy and --clean toggles + auto-detected mode indicator to scenario page start run controls | P1 | done | S25-T2 | codex |
| S25-T4 | Slice 25 | Frontend: add run mode badge, collapsible baseline state panel, and raw plan diff panel to Live page | P1 | done | S25-T3 | codex |
| S25-T5 | Slice 25 | Frontend + backend tests for plan/baseline persistence, flag pass-through, toggle state, and panel rendering; update README Web UI section | P1 | done | S25-T4 | codex |
| S24-T1 | Slice 24 | Single-file multi-stage incremental E2E test: edit scenario, re-run, verify incremental apply (3 stages) | P1 | done | S23-T3 | codex |
| S24-T2 | Slice 24 | `--clean` regression test, post-destruction clean auto-detect, and README incremental workflow docs | P1 | done | S24-T1 | codex |
| S23-T1 | Slice 23 | Add `--clean` and `--no-destroy` flags + incremental auto-detection logic to run loop | P1 | done | S22-T1 | codex |
| S23-T2 | Slice 23 | Update MockDeployHarness: snapshot at run start, restore between iterations (incremental) or reset (clean) | P1 | done | S23-T1 | codex |
| S23-T3 | Slice 23 | Add `incremental` and `previous_run_id` to RunMetadata; preserve .tfstate in incremental mode; update README CLI/config reference | P1 | done | S23-T2 | codex |
| S22-T1 | Slice 22 | Mockway: add `POST /mock/snapshot` and `POST /mock/restore` endpoints with SQLite state copy | P1 | done | — | codex |
| S22-T2 | Slice 22 | Mockway: integration tests for snapshot/restore/reset lifecycle | P1 | done | S22-T1 | codex |
| M34 | Maintenance | End-to-end pipeline stabilization: mockway endpoint fixes, self-review contract tightening, prompt pitfall updates | P1 | done | M33 | codex |
| M33 | Maintenance | Document lazy provider-schema prompt injection and convergence hardening outcomes; close with two no-change review passes | P1 | done | M32 | codex |
| M32 | Maintenance | Harden self-review convergence and stuck-signature specificity for run-loop retries | P1 | done | M31 | codex |
| M31 | Maintenance | Add run-loop feedback observability for model input and sanitize validation stderr signal | P1 | done | M30 | codex |
| M30 | Maintenance | Hard cutover run loop to failure-only retries (remove `iterations_target`) | P1 | done | S17-T1 | codex |
| M29 | Maintenance | Scope opt-in LLM raw stage-response capture ticket with redaction/size safeguards and closure-pass notes | P1 | done | M28 | codex |
| M28 | Maintenance | Plan Slice 16 issue-driven remediation backlog and fresh-context startup guidance | P1 | done | M27 | codex |
| M27 | Maintenance | Re-baseline Slice 12 planning/docs for dual iteration controls and fresh-context readiness | P1 | done | M26 | codex |
| M26 | Maintenance | Refine unfinished-slice governance again (`S9-T8`, `S12`-`S15`) and record two consecutive no-change passes | P1 | done | M25 | codex |
| M25 | Maintenance | Optimize and embed post-slice review-improve prompt across unfinished slices | P1 | done | M24 | codex |
| M24 | Maintenance | Refine all unfinished slices (`S12`-`S15` plus blocked `S9-T8`) and record two consecutive no-change passes | P1 | done | M23 | codex |
| M23 | Maintenance | Plan Slice 15 adaptive retry and transport-resilience policy | P1 | done | M22 | codex |
| M22 | Maintenance | Refine all unfinished slices (`S12`-`S14`) for higher-signal model-guided repair and run two consecutive no-change passes | P1 | done | M21 | codex |
| M21 | Maintenance | Plan Slice 14 high-fidelity run feedback payload wiring for model-guided fixes | P1 | done | M20 | codex |
| M20 | Maintenance | Plan Slice 13 full app-logic logging/observability backlog before implementation | P1 | done | M19 | codex |
| M19 | Maintenance | Plan Slice 12 iteration contract migration (`max_iterations` -> `iterations`, default 3) before implementation | P1 | done | M18 | codex |
| M18 | Maintenance | Strengthen fresh-context startup documentation with run-loop and Mockway operational guardrails | P1 | done | M17 | codex |
| M17 | Maintenance | Wire run-loop failure feedback into iterative LLM generation (no heuristic normalization) | P1 | done | M15 | codex |
| M15 | Maintenance | Auto-inject missing Scaleway provider wiring during generate | P1 | done | M14 | codex |
| M14 | Maintenance | Fail generate early when Scaleway resources are missing provider wiring | P1 | done | M13 | codex |
| M13 | Maintenance | Make mock deploy test path self-contained by running tofu init before apply | P1 | done | M12 | codex |
| M12 | Maintenance | Harden parser/self-review fallback and surface tofu/mock stderr in CLI failures | P1 | done | M11 | codex |
| M11 | Maintenance | Add Claude phase timeout and progress logging to prevent silent generate hangs | P1 | done | - | codex |
| M10 | Maintenance | Re-review README and optimize until two consecutive no-change passes (follow-up) | P1 | done | - | codex |
| M9 | Maintenance | Add explicit README happy-path section for Claude Code end-to-end run | P1 | done | - | codex |
| M8 | Maintenance | Re-review README and optimize until two consecutive no-change passes | P1 | done | - | codex |
| M7 | Maintenance | Add fresh-context repo-state preflight guidance and verify consecutive no-change passes | P1 | done | - | codex |
| M6 | Maintenance | Refresh fresh-context startup guidance and verify with consecutive no-change passes | P1 | done | - | codex |
| M5 | Maintenance | Refine Slice 11 plan iteratively until two consecutive no-change passes | P1 | done | - | codex |
| M4 | Maintenance | Optimize README clarity and consistency via iterative review passes | P1 | done | - | codex |
| M3 | Maintenance | Plan Slice 11 for concrete generator transport integration (`claude -p`, OpenRouter) | P1 | done | - | codex |
| M2 | Maintenance | Add CI workflow to run tests on PR/main and build binary artifact on successful main push | P1 | done | - | codex |
| M1 | Maintenance | Harden runtime/policy regressions captured in `ISSUES.md` + add regression tests | P1 | done | - | codex |
| S1-T1 | Slice 1 | Wire Cobra root and commands (`init`, `generate`, `validate`, `test`, `run`, `mock start`) | P0 | done | - | codex |
| S1-T2 | Slice 1 | Implement `internal/config` loader with defaults and required-field validation | P0 | done | S1-T1 | codex |
| S1-T3 | Slice 1 | Implement `internal/scenario` loader + JSON Schema validation | P0 | done | S1-T1 | codex |
| S1-T4 | Slice 1 | Add Slice 1 focused tests (config/scenario valid+invalid paths) | P0 | done | S1-T2,S1-T3 | codex |
| S2-T1 | Slice 2 | Define `SeedGenerator` interface and generator contracts | P1 | done | S1-T4 | codex |
| S2-T2 | Slice 2 | Implement prompt rendering helpers and feedback context injection | P1 | done | S2-T1 | codex |
| S2-T3 | Slice 2 | Implement `# File:` parser with code-fence stripping + duplicate handling | P1 | done | S2-T1 | codex |
| S2-T4 | Slice 2 | Add generator/parser focused tests | P1 | done | S2-T2,S2-T3 | codex |
| S3-T1 | Slice 3 | Implement static harness (`tofu init/validate/plan/show -json`) | P2 | done | S2-T4 | codex |
| S3-T2 | Slice 3 | Integrate OPA evaluation against plan JSON | P2 | done | S3-T1 | codex |
| S3-T3 | Slice 3 | Add structured static-layer failure reporting + tests | P2 | done | S3-T1,S3-T2 | codex |
| S4-T1 | Slice 4 | Implement mock deploy orchestration (`tofu apply`, mock reset/state client) | P3 | done | S3-T3 | codex |
| S4-T2 | Slice 4 | Add topology checks and state policy checks in harness | P3 | done | S4-T1 | codex |
| S4-T3 | Slice 4 | Add mock deploy layer tests (opt-in where external deps required) | P3 | done | S4-T2 | codex |
| S5-T1 | Slice 5 | Implement destroy flow + orphan verification | P4 | done | S4-T3 | codex |
| S5-T2 | Slice 5 | Implement run store persistence on disk | P4 | done | S5-T1 | codex |
| S5-T3 | Slice 5 | Add destroy/run-store tests | P4 | done | S5-T2 | codex |
| S6-T1 | Slice 6 | Implement feedback loop + max-iteration control | P5 | done | S5-T3 | codex |
| S6-T2 | Slice 6 | Implement stuck detection using failure-signature subset logic (`check`+`resource`+`detail`) | P5 | done | S6-T1 | codex |
| S6-T3 | Slice 6 | Implement criteria-only holdout flow | P5 | done | S6-T2 | codex |
| S7-T1 | Slice 7 | Wire `init` command scaffold generation + next-step output | P0 | done | S6-T3 | codex |
| S7-T2 | Slice 7 | Add shared CLI runtime/context builder and command error formatter | P0 | done | S7-T1 | codex |
| S7-T12 | Slice 7 | Freeze CLI command contract (args/flags/exit codes/output modes) | P0 | done | S7-T2 | codex |
| S7-T16 | Slice 7 | Freeze CLI output contract (human summary + machine JSON schema) | P0 | done | S7-T12 | codex |
| S7-T13 | Slice 7 | Add shared CLI command test harness utility (workspace/setup/capture helpers) | P1 | done | S7-T12 | codex |
| S7-T3 | Slice 7 | Wire `generate` command to config/scenario/generator pipeline + file writes | P0 | done | S7-T12,S7-T16 | codex |
| S7-T4 | Slice 7 | Wire `validate` command to static layer execution + policy reporting | P0 | done | S7-T12,S7-T16 | codex |
| S7-T5 | Slice 7 | Wire `test` command to mock deploy + destroy verification flow | P0 | done | S7-T12,S7-T16 | codex |
| S7-T6 | Slice 7 | Add early CLI orchestration smoke tests (`generate`/`validate`/`test`) | P0 | done | S7-T3,S7-T4,S7-T5 | codex |
| S7-T7 | Slice 7 | Wire `run` command skeleton (single-iteration orchestration path) | P0 | done | S7-T3,S7-T4,S7-T5 | codex |
| S7-T8 | Slice 7 | Add `run` convergence controls (max-iteration + stuck detection integration) | P0 | done | S7-T7 | codex |
| S7-T9 | Slice 7 | Add `run` persistence/reporting integration with runstore | P0 | done | S7-T8 | codex |
| S7-T10 | Slice 7 | Wire `mock start` command to runtime start path + preflight checks | P1 | done | S7-T12 | codex |
| S7-T11 | Slice 7 | Add hermetic CLI orchestration integration tests and regression fixtures | P0 | done | S7-T6,S7-T7,S7-T8,S7-T9,S7-T10 | codex |
| S7-T14 | Slice 7 | Add opt-in real-tool orchestration smoke tests (`tofu` + optional Mockway) | P1 | done | S7-T11 | codex |
| S7-T15 | Slice 7 | Sync README/usage docs to final CLI behavior and examples | P1 | done | S7-T9,S7-T11 | codex |
| S8-T1 | Slice 8 | Add developer-experience automation (`Makefile` + dependency lifecycle targets) | P0 | done | S7-T15 | codex |
| S8-T2 | Slice 8 | Add DX smoke runner targets for real-tool validation flows | P0 | done | S8-T1 | codex |
| S8-T3 | Slice 8 | Sync README/usage docs for DX workflows (deps/tests/smoke/cleanup) | P1 | done | S8-T2 | codex |
| S9-T1 | Slice 9 | Wire default runtime `SeedGenerator` implementation for `generate`/`run` | P0 | done | S8-T3 | codex |
| S9-T10 | Slice 9 | Expand `internal/scenario.Scenario` model to include criteria/layer-routing fields | P0 | done | S9-T1 | codex |
| S9-T2 | Slice 9 | Parse scenario `acceptance_criteria` into typed executable check specs | P0 | done | S9-T10 | codex |
| S9-T11 | Slice 9 | Define criteria support matrix and deterministic unsupported-criteria behavior | P0 | done | S9-T2 | codex |
| S9-T3 | Slice 9 | Execute criteria-driven topology + state-policy checks in `test` | P0 | done | S9-T11 | codex |
| S9-T4 | Slice 9 | Honor `validation.layers.*.enabled` flags in `validate`/`test`/`run` orchestration | P0 | done | S9-T3 | codex |
| S9-T5 | Slice 9 | Upgrade `run` from skeleton to criteria-aware convergence orchestration | P0 | done | S9-T3,S9-T4 | codex |
| S9-T6 | Slice 9 | Wire criteria-only holdout evaluation into `run` completion path | P1 | done | S9-T5 | codex |
| S9-T7 | Slice 9 | Expand `mock` command lifecycle (`start/stop/status/logs`) with parity tests | P1 | done | S9-T4 | codex |
| S9-T8 | Slice 9 | Sandbox/live deploy layer wiring (real Scaleway) | P2 | done | - | codex |
| S9-T9 | Slice 9 | Document sandbox deploy deferment due cost implications | P0 | done | S8-T3 | codex |
| S10-T1 | Slice 10 | Freeze output contract with golden snapshots for all commands/modes | P0 | done | S10-T2,S10-T3,S10-T6 | codex |
| S10-T2 | Slice 10 | Normalize CLI error taxonomy/messages across command paths | P0 | done | S9-T7 | codex |
| S10-T3 | Slice 10 | Version run artifact schema and add backward-compatible readers | P0 | done | S9-T7 | codex |
| S10-T4 | Slice 10 | Add idempotency/retry safety checks for repeated command execution | P1 | done | S10-T1,S10-T2,S10-T3 | codex |
| S10-T5 | Slice 10 | Add performance baseline benchmarks and regression guardrails | P1 | done | S10-T3 | codex |
| S10-T6 | Slice 10 | Add criteria/policy failure explainability summaries | P1 | done | S10-T2,S10-T3 | codex |
| S10-T7 | Slice 10 | Finalize permanent sandbox-block governance docs + ADR | P0 | done | S9-T7,S10-T1 | codex |
| S11-T1 | Slice 11 | Define generator transport contract + config mapping for `claude -p` and OpenRouter | P0 | done | - | codex |
| S11-T2 | Slice 11 | Implement `claude -p` transport adapter with deterministic prompt/phase execution | P0 | done | S11-T1 | codex |
| S11-T3 | Slice 11 | Implement OpenRouter HTTP transport adapter with deterministic retries/timeouts/errors | P0 | done | S11-T1 | codex |
| S11-T4 | Slice 11 | Runtime wiring + selection for concrete transports (replace default transport stub path) | P0 | done | S11-T2,S11-T3 | codex |
| S11-T5 | Slice 11 | Add hermetic adapter tests + opt-in smoke tests for real transport paths | P1 | done | S11-T2,S11-T3 | codex |
| S11-T7 | Slice 11 | Add transport credential safety/redaction guardrails for errors and logs | P1 | done | S11-T2,S11-T3 | codex |
| S11-T6 | Slice 11 | Sync docs and examples for transport configuration/usage and failure modes | P1 | done | S11-T4,S11-T5,S11-T7 | codex |
| S12-T1 | Slice 12 | Define dual-control run contract (`repair_iterations_max`, `iterations_target`) and deterministic stop reasons | P0 | done | - | codex |
| S12-T2 | Slice 12 | Implement config/runtime controls: defaults/validation for `agent.repair_iterations_max` and `agent.iterations_target` | P0 | done | S12-T1 | codex |
| S12-T3 | Slice 12 | Implement CLI controls: `--repair-iterations-max` and `--iterations-target` with deterministic precedence | P0 | done | S12-T1 | codex |
| S12-T4 | Slice 12 | Refresh run/golden/unit tests for failure-repair budget and fixed-pass target behaviors | P0 | done | S12-T2,S12-T3 | codex |
| S12-T6 | Slice 12 | Persist per-iteration failure logs and one terminal control reason (`target_reached|repair_budget_exhausted|stuck`) | P0 | done | S12-T2,S12-T3 | codex |
| S12-T5 | Slice 12 | Sync docs/templates/examples (`infrafactory.yaml`, README, SESSION_START`) to dual controls after Slice 12 implementation closure | P1 | done | S12-T4,S12-T6 | codex |
| S13-T1 | Slice 13 | Define application logging contract (fields/levels/redaction/output format) and rollout rules | P0 | done | S12-T5 | codex |
| S13-T2 | Slice 13 | Add shared logger wiring/runtime integration for all command paths and deterministic log sinks | P0 | done | S13-T1 | codex |
| S13-T3 | Slice 13 | Instrument `run` loop decisions and stage transitions with full iteration/run context logs and failure-class context | P0 | done | S13-T2 | codex |
| S13-T4 | Slice 13 | Instrument `generate`/`validate`/`test`/`mock` command logic for deterministic traceability | P0 | done | S13-T2 | codex |
| S13-T5 | Slice 13 | Add logging regression tests/goldens and redaction assertions | P0 | done | S13-T3,S13-T4 | codex |
| S13-T6 | Slice 13 | Sync operator docs and fresh-context startup guidance for logging runbook | P1 | done | S13-T5 | codex |
| S14-T1 | Slice 14 | Define run feedback contract so iteration N+1 receives detailed validate/test/generate failures with failure-class tagging (not coarse command errors) | P0 | done | S13-T6 | codex |
| S14-T2 | Slice 14 | Refactor `validate` into reusable execution path returning structured failure output for `run` feedback reuse | P0 | done | S14-T1 | codex |
| S14-T3 | Slice 14 | Wire `runIteration` feedback aggregation to propagate detailed stage/check/policy/resource context into `FeedbackJSON` and suppress duplicate terminal control failures | P0 | done | S14-T2 | codex |
| S14-T4 | Slice 14 | Add focused regression tests for feedback payload fidelity across validate/test/generate failure paths | P0 | done | S14-T3 | codex |
| S14-T5 | Slice 14 | Sync docs/fresh-context runbook with feedback-inspection workflow and anti-regression guardrails | P1 | done | S14-T4 | codex |
| S15-T1 | Slice 15 | Define adaptive retry governance contract by failure class (`iac_validation` vs `transport_runtime` vs `orchestration_control`) | P0 | done | S14-T5 | codex |
| S15-T2 | Slice 15 | Implement transport-dominated early-stop policy with deterministic terminal reason mapping | P0 | done | S15-T1 | codex |
| S15-T3 | Slice 15 | Add bounded transport retry budget/backoff and timeout-aware guidance hooks | P0 | done | S15-T1 | codex |
| S15-T4 | Slice 15 | Persist transport diagnostics in run artifacts (phase/timeout/signal/stderr summary/duration) | P0 | done | S15-T2,S15-T3 | codex |
| S15-T5 | Slice 15 | Add regression coverage for adaptive retry behavior and transport-dominated failure outcomes | P0 | done | S15-T4 | codex |
| S15-T6 | Slice 15 | Sync docs and fresh-context runbook for adaptive retry troubleshooting and operator actions | P1 | done | S15-T5 | codex |
| S16-T1 | Slice 16 | Propagate `cmd.Context()` through all command handlers and runtime operations | P0 | done | S15-T6 | codex |
| S16-T2 | Slice 16 | Bound Mockway state response reads to prevent unbounded memory allocation | P0 | done | S16-T1 | codex |
| S16-T3 | Slice 16 | Make env override injection deterministic by de-duplicating overridden keys | P0 | done | S16-T1 | codex |
| S16-T4 | Slice 16 | Enforce scenario schema availability (no silent validation bypass when schema path missing) | P0 | done | S16-T1 | codex |
| S16-T5 | Slice 16 | Clean up config decode error handling and provide explicit empty-config failure message | P1 | done | S16-T1 | codex |
| S16-T6 | Slice 16 | Correct policy semantics and naming regex edge-case behavior from `ISSUES.md` | P0 | done | S16-T4 | codex |
| S16-T7 | Slice 16 | Remove dead runtime scaffolding and make destroy-run schema assignment explicit | P1 | done | S16-T1 | codex |
| S16-T8 | Slice 16 | Sync docs/fresh-context guidance for issue-remediation outcomes and run closure refinement passes | P1 | done | S16-T2,S16-T3,S16-T4,S16-T5,S16-T6,S16-T7 | codex |
| S17-T1 | Slice 17 | Add opt-in per-iteration/per-phase LLM raw response capture artifacts with redaction and size caps | P1 | done | M29 | codex |
| S18-T1 | Slice 18A | K8s standalone scenario: exercise existing mockway K8s handlers, fix gaps (update, cascade delete) | P1 | done | M34 | codex |
| S18-T2 | Slice 18B | IAM standalone scenario: exercise existing mockway IAM handlers, extend scenario schema for `iam` resource type | P1 | done | M34 | codex |
| S18-T3 | Slice 18C | Container Registry scenario + mockway service: new registry CRUD handlers, scenario schema extension | P1 | done | S18-T1,S18-T2 | codex |
| S18-T4 | Slice 18D | Redis scenario + mockway service: new Redis CRUD handlers, scenario schema + mappings extension | P1 | done | S18-T1,S18-T2 | codex |
| S18-T5 | Slice 18E | Composite multi-service scenario: all resource types in one scenario, cross-resource validation | P1 | done | S18-T3,S18-T4 | codex |
| S19-T1 | Slice 19 | Reliability review: audit Slice 18 code in mockway and infrafactory, identify bugs/issues, fix iteratively, run all scenarios until green | P1 | done | S18-T5 | codex |
| S20-T1 | Slice 20A | MySQL HA scenario: mysql engine, medium size, high availability, private networking | P1 | done | S19-T1 | - |
| S20-T2 | Slice 20B | Multi-backend LB scenario: large compute cluster, multi-backend LB with tcp protocol | P1 | done | S19-T1 | - |
| S20-T3 | Slice 20C | K8s medium override scenario: medium K8s with explicit node_type and node_count overrides | P1 | done | S19-T1 | - |
| S20-T4 | Slice 20D | Private LB + large DB override scenario: private LB, large PostgreSQL with node_type/engine_version overrides | P1 | done | S20-T1 | - |
| S20-T5 | Slice 20E | Public registry + selective IAM scenario: is_public registry, IAM with policy=false | P1 | done | S19-T1 | - |
| S20-T6 | Slice 20F | Redis xlarge override scenario: xlarge Redis with node_type override, xlarge compute | P1 | done | S20-T1 | - |
| SUi-1 | Slice 21A | Skeleton server + static asset embed: `infrafactory ui` serves embedded SvelteKit placeholder page | P1 | done | S20-T6 | codex |
| SUi-2 | Slice 21B | Scenario browser + sidebar: left-hand scenario tree with parsed metadata display | P1 | done | SUi-1 | codex |
| SUi-3 | Slice 21C | Run history browser: browse past runs with iteration timeline and failure details | P1 | done | SUi-1 | codex |
| SUi-4 | Slice 21D | Generated code viewer: view .tf files with HCL syntax highlighting | P1 | done | SUi-1 | codex |
| SUi-5 | Slice 21E | WebSocket infrastructure + log streaming: real-time log stream via WebSocketSink | P1 | done | SUi-1 | codex |
| SUi-6 | Slice 21F | Live run execution + pipeline visualization: start runs from UI, animated stage transitions | P1 | done | SUi-5 | codex |
| SUi-7 | Slice 21G | Scenario YAML editor: edit and save scenarios with validation feedback | P1 | done | SUi-2 | codex |
| SUi-8 | Slice 21H | Build pipeline + polish: GoReleaser integration, single binary distribution, docs | P1 | done | SUi-6,SUi-7 | codex |

## Ticket details

| id | in-scope files/packages | out-of-scope | acceptance criteria | required tests |
|---|---|---|---|---|
| S1-T1 | `cmd/infrafactory`, `internal/cli` command tree wiring only | config/scenario parsing, harness behavior | root command exposes all required Slice 1 commands; leaf commands return explicit `not implemented` errors | command discovery tests; stub error tests |
| S1-T2 | `internal/config` load/validate/defaults; command wiring to call config loader only | scenario schema logic, generator/harness flows | config file loads deterministically; missing required fields return typed validation errors | valid config fixture; invalid/missing required-field fixtures |
| S1-T3 | `internal/scenario` parse + JSON schema compile/validate | generator prompt logic, harness execution | scenario loader validates against `scenario.schema.json`; errors include actionable path/context | valid scenario fixture; invalid schema fixture; malformed YAML fixture |
| S1-T4 | Slice 1 integration tests across config+scenario loaders | Slice 2+ features | successful path covers config+scenario parse chain; failure paths isolate config vs scenario errors | table-driven valid/invalid path tests in `internal/config` and `internal/scenario` |
| S2-T1 | `internal/generator` contracts (`SeedGenerator`, request/response types) | provider-specific prompt text changes | interface returns file map and metadata needed by harness; typed errors defined for generator failures | interface behavior tests; typed error wrapping tests |
| S2-T2 | prompt rendering helpers and feedback context injection in `internal/generator` | parser behavior, harness orchestration | prompt templates render deterministic output with scenario+feedback context | template rendering tests for with/without feedback |
| S2-T3 | `internal/generator` `# File:` parser with code-fence stripping + duplicate resolution | LLM call transport | parser emits deterministic map of files, strips fences, handles duplicates by documented rule | parser fixtures: single file, multi-file, fenced blocks, duplicate markers |
| S2-T4 | focused generator/parser suite | Slice 3 harness flows | generator package has hermetic tests covering render and parse success/failure | package-level table-driven tests with malformed output fixtures |
| S3-T1 | `internal/harness` static workflow (`tofu init/validate/plan/show`) | mock deploy and destroy layers | static layer executes commands in fixed order and captures structured outputs | command runner unit tests using fake exec; failure-path tests per stage |
| S3-T2 | OPA policy evaluation against plan JSON (`internal/harness`, `internal/feedback`) | mock state policy checks | plan JSON is passed to OPA; deny results returned as structured failures | OPA fixture tests for pass/fail policy sets |
| S3-T3 | structured static-layer failure reporting | layer 2+ orchestration | failures include stage, command, stderr/stdout summary and machine-readable fields | error-shape tests for validate/plan/OPA failures |
| S4-T1 | mock deploy orchestration (`tofu apply`, mock reset/state client) | sandbox deploy and destruction | harness resets mock state, applies generated code, captures state snapshot deterministically | mock client tests; apply orchestration tests with fake command runner |
| S4-T2 | topology checks + state policy checks in layer 2 | layer 1 plan checks | connectivity and policy checks evaluate mock state graph correctly | topology evaluator tests; state-policy OPA tests |
| S4-T3 | layer 2 test coverage including opt-in integration tests | Slice 5 destroy/run-store logic | deploy layer test suite runs hermetic unit tests and optional integration path | unit tests with fixtures; integration smoke test guarded by env flag |
| S5-T1 | destroy flow and orphan verification (`internal/harness`) | feedback loop logic | `tofu destroy` runs and post-destroy state confirms no orphans | destroy success/failure tests; orphan-detected test |
| S5-T2 | filesystem run-store implementation in `internal/runstore` | DB/CXDB backends | run metadata and iteration artifacts persist under `.infrafactory/runs/` with deterministic paths | runstore write/read/list tests with temp dirs |
| S5-T3 | destroy/run-store combined tests | Slice 6 convergence logic | destruction results and run-store persistence integrate without hidden side effects | end-to-end unit tests for run record creation on success/failure |
| S6-T1 | feedback loop orchestration + max iteration control in `internal/feedback`/`internal/harness` | holdout-specific logic | loop stops on success or max-iteration threshold; iteration outputs persisted | convergence success test; max-iteration stop test |
| S6-T2 | stuck detection using failure signature subset comparison (`check`+`resource`+`detail`) | holdout execution | loop aborts only when failures are unchanged/subset-equivalent by signature including detail context | failure-signature comparator tests with subset/non-subset and same-check-different-detail cases |
| S6-T3 | criteria-only holdout discovery and execution flow | full holdout contract changes | criteria-only holdouts auto-discovered by reference and block without feedback injection | holdout discovery tests; block-without-feedback tests |
| S7-T1 | `internal/cli`, `internal/scenario`, templates/scaffold helpers | generator/harness orchestration | `infrafactory init` writes minimal valid scaffold and prints deterministic next steps | scaffold file content tests; command output tests |
| S7-T2 | `internal/cli` shared runtime/context builder + error formatter + dependency injection points | command-specific orchestration logic | shared command setup loads config/scenario/output context once, provides injectable command dependencies (generator/harness/mock clients), and returns standardized CLI-facing errors | runtime builder tests; dependency-injection tests; error-format tests |
| S7-T12 | `internal/cli` command contract spec/helpers | command implementation internals | command args/flags/exit-code and output-mode contract is explicitly defined and stable before wiring commands | contract tests for parse/exit-code/output mode behavior |
| S7-T16 | `internal/cli` output contract spec/helpers | command orchestration wiring internals | CLI output contract defines stable human summaries and machine JSON schema with deterministic ordering of stages/failures | output-schema tests; golden snapshot tests for human and machine output |
| S7-T13 | `internal/cli` command test helper utilities | command business behavior | shared helper sets up temp workspace/dependencies and captures stdout/stderr deterministically for command tests | helper tests; adoption in at least one command test suite |
| S7-T3 | `internal/cli`, `internal/generator`, output writer helpers | harness layer execution | `infrafactory generate <scenario>` loads config/scenario and writes generated files to output dir | fake generator unit tests; output file write tests |
| S7-T4 | `internal/cli`, `internal/harness` static layer adapters | mock deploy/destroy execution | `infrafactory validate <scenario>` executes static checks and returns structured policy/stage failures | static success/failure command tests |
| S7-T5 | `internal/cli`, `internal/harness` deploy+destroy adapters | feedback loop/orchestration | `infrafactory test <scenario>` executes mock deploy checks and destroy/orphan verification | deploy/destroy command tests with fake clients |
| S7-T6 | `internal/cli` command-level smoke tests for `generate`/`validate`/`test` | full `run` loop and regression matrix | command orchestration smoke coverage catches major wiring regressions early | smoke tests for success and one representative failure path per command |
| S7-T7 | `internal/cli`, `internal/feedback`, `internal/harness` run adapter | convergence controls and persistence | `infrafactory run <scenario>` executes a single-iteration skeleton path with deterministic stage aggregation | run skeleton tests with fake dependencies |
| S7-T8 | `internal/cli`, `internal/feedback` convergence integration | runstore reporting and regression matrix | `run` integrates max-iteration and stuck-detection stop semantics | convergence-control tests for stop-on-success/max/stuck |
| S7-T9 | `internal/cli`, `internal/runstore` run persistence/reporting integration | command preflight/runtime management | `run` persists run metadata and iteration artifacts and prints stable run summary | runstore integration tests for success/failure run outputs |
| S7-T10 | `internal/cli` mock command adapter + process preflight checks | full harness loop | `infrafactory mock start` performs deterministic start/preflight behavior with actionable errors | mock start command tests (success + missing dependency) |
| S7-T11 | cross-package hermetic CLI integration tests + fixtures | external tool/runtime behavior | orchestration command suite has hermetic regression coverage for happy and failure paths | integration tests for `generate/validate/test/run/mock start` flows with fake deps |
| S7-T14 | opt-in orchestration smoke tests using real tools (`tofu`, optional Mockway) | hermetic CI default path | opt-in smoke validates command wiring with real external binaries/services | env-guarded smoke tests for `validate/test/run` critical paths |
| S7-T15 | README/usage documentation synchronization to final CLI behavior | architecture or schema contract changes | user-facing docs and examples reflect actual command behavior/flags/output after wiring completes | doc assertion checklist + command example verification |
| S8-T1 | `Makefile`, `docker-compose.yml` | CLI/runtime orchestration logic | make targets exist for dependency up/down/recreate/clean and hermetic test entrypoints | smoke-free make target dry-runs + command invocation checks |
| S8-T2 | `Makefile`, real-tool smoke wiring | mandatory default hermetic path | make targets run opt-in real-tool smoke with explicit env guards and mockway readiness wait | env-guarded smoke target tests/manual verification |
| S8-T3 | `README.md`, `STATUS.md` | architecture/schema contract changes | docs include canonical DX command set for tests/smoke/dependency lifecycle | doc assertion checklist |
| S9-T1 | `internal/cli/runtime.go`, `internal/generator` | provider prompt/template redesign | runtime provides a non-nil default `SeedGenerator`; `generate`/`run` no longer fail with missing generator dependency when runtime defaults are used | `internal/cli/runtime_test.go` + command-path tests for default runtime behavior |
| S9-T10 | `internal/scenario/scenario.go`, `internal/scenario/*_test.go` | schema file redesign | loaded scenario includes typed fields required by orchestration (`type`, `references`, constraints, acceptance criteria), not just scenario name | scenario loader tests asserting typed decode and backward compatibility of existing valid fixtures |
| S9-T2 | `internal/scenario`, `internal/harness`, `internal/cli` adapters | policy authoring semantics | scenario `acceptance_criteria` entries are mapped deterministically into typed executable check specs (connectivity/http_probe/policy/destruction where supported) | parser/mapping tests for each supported criteria type |
| S9-T11 | `internal/cli`, `internal/harness`, docs | schema semantic changes | criteria outside current executable layers (notably `dns_resolution`) produce deterministic support-matrix output with explicit messaging (auto-pass informational for deferred cloud-only checks); behavior is documented | command tests for support-matrix behavior in human/json output + doc assertions |
| S9-T3 | `internal/cli/test_command.go`, `internal/harness/topology.go`, `internal/harness/state_policy.go` | run-loop convergence internals | `test` executes topology/state-policy evaluators using criteria-derived inputs and reports failures through output contract | command tests for criteria pass/fail and evaluator invocation coverage |
| S9-T4 | `internal/cli/validate_command.go`, `internal/cli/test_command.go`, `internal/cli/run_command.go` | schema contract changes | CLI orchestration consistently honors `validation.layers.static.enabled`, `validation.layers.mock_deploy.enabled`, and `validation.layers.destruction.enabled`; `sandbox_deploy.enabled` remains explicitly blocked with deterministic surfaced status | tests for enabled/disabled combinations and sandbox-enabled blocked-status output |
| S9-T5 | `internal/cli/run_command.go`, `internal/feedback`, `internal/runstore` | runstore backend redesign | `run` performs criteria-aware iteration decisions (not just stage pass-through) and preserves convergence semantics (success/max/stuck) | run command tests for criteria-driven success/failure, max-iteration stop, and stuck-detection stop |
| S9-T6 | `internal/cli/run_command.go`, `internal/scenario/holdout.go` | holdout schema redesign | criteria-only holdouts auto-run after training convergence and block without feeding holdout failures into generator context | holdout discovery/execution tests for pass/block/no-feedback behavior |
| S9-T7 | `internal/cli/root.go`, `internal/cli/mock_*`, `Makefile` (if needed) | external image publishing strategy | `mock` command supports `start`, `stop`, `status`, and `logs` with deterministic output and errors | command tests for stop/status/logs happy-path + missing-runtime/dependency failures |
| S9-T8 | `internal/harness`, `internal/cli`, docs | mock deploy layer and hermetic defaults | Superseded by Slices 26-30 (ADR-0010). Layer 3 real Scaleway deploy fully implemented with dual-apply, auto-destroy, real probes, and production hardening. | Done. |
| S9-T9 | `README.md`, `STATUS.md`, `ROADMAP.md` | runtime behavior changes | docs explicitly state sandbox/live deploy deferment due cost implications | doc assertion checklist |
| S10-T1 | `internal/cli/output_contract*`, command tests, fixtures/goldens | command business logic changes | final human/json outputs for all commands are frozen via golden snapshots with deterministic ordering and schema assertions after Slice 10 contract-shaping tickets land | golden tests for `init/generate/validate/test/run/mock *` in human+json modes and deterministic fixture update guardrails |
| S10-T2 | `internal/cli/runtime.go`, command adapters, error helpers | schema/provider behavior changes | CLI errors use consistent codes, operation names, and actionable detail shapes across config/scenario/runtime/dependency failures | table-driven error-shape tests for representative failure classes per command |
| S10-T3 | `internal/runstore`, `internal/cli/run_command.go`, docs | new storage backend | run artifacts include explicit schema version; readers remain backward-compatible for pre-versioned artifacts | runstore read/write compatibility tests and run artifact schema assertions |
| S10-T4 | `internal/cli/*_command.go`, `internal/cli/integration_*` | external orchestration redesign | repeated command execution in same workspace remains deterministic and safe under partial prior failures | idempotency integration tests for repeated `generate/validate/test/run/mock` flows |
| S10-T5 | `internal/*` benchmarks, CI scripts/docs | feature behavior changes | baseline benchmark suite exists for key flows with documented thresholds and regression checks without requiring network/external services | benchmark tests + CI guard script assertions with env-guarded execution in default hermetic path |
| S10-T6 | `internal/cli/output_contract.go`, failure mapping adapters, docs | policy authoring semantics | failure output includes concise explainability summaries linking criteria/policy checks to actionable context | output contract tests for explainability fields in human/json outputs |
| S10-T7 | `docs/decisions/*`, `docs/decisions/README.md`, `README.md`, `ROADMAP.md`, `STATUS.md` | runtime deployment implementation | permanent sandbox/live block governance is codified in ADR and synchronized docs with unambiguous policy language | doc hygiene + ADR index/checklist assertions |
| S11-T1 | `internal/generator`, `internal/config`, `internal/cli/runtime.go`, `infrafactory.yaml`, docs | transport implementation details | transport selection contract is explicit and stable (`agent.type`/required env+config), including phase sequencing and delay semantics (`agent.phases`, `agent.phase_delay_seconds`) for both providers | config/runtime contract tests for transport selection, phase config handling, and typed validation failures |
| S11-T2 | `internal/generator` claude adapter package/files, prompt execution helpers | OpenRouter transport details | `claude -p` adapter executes configured phases via command-runner abstraction and returns parsed file map + metadata with typed transport errors | hermetic unit tests with fake command runner; parser integration tests; opt-in smoke test behind env flag |
| S11-T3 | `internal/generator` openrouter adapter package/files | claude CLI transport details | OpenRouter adapter executes deterministic request flow (timeouts/retries/error mapping/model selection) and returns parsed file map + metadata | hermetic HTTP client tests with fake server; parser integration tests; opt-in smoke test behind env flag |
| S11-T4 | `internal/cli/runtime.go`, `internal/generator/default.go`, `internal/cli/*_command_test.go` | schema contract changes outside generator/runtime | runtime injects concrete transport-backed generator for supported `agent.type`; unknown/misconfigured agent types fail with deterministic typed errors; stub-not-implemented path removed from normal configured flows | runtime wiring tests and command-path tests covering both agent types and deterministic failure shapes |
| S11-T5 | `internal/generator/*_test.go`, `internal/cli/realtool_smoke_test.go`, CI/docs gating notes | mandatory non-hermetic CI path | adapter behavior is covered by hermetic tests and opt-in smoke runs for real external transport dependencies without affecting default hermetic CI path | table-driven unit tests + env-guarded smoke tests for both transports |
| S11-T7 | `internal/generator`, output/error formatting helpers, tests | transport feature behavior unrelated to secret handling | transport adapters never leak raw API keys/tokens/prompts in surfaced errors/log metadata; redaction behavior is deterministic across provider failures | redaction unit tests for error wrapping and failure output mapping |
| S11-T6 | `README.md`, `STATUS.md`, `SESSION_START.md` | architecture governance policy changes | docs include transport setup, required credentials/tools, invocation examples, and provider-specific troubleshooting/runbook notes | doc checklist assertions |
| S12-T1 | `internal/config`, `internal/cli/root.go`, ADR/docs planning surfaces | behavioral implementation | dual-control contract is explicit: config keys (`agent.repair_iterations_max`, `agent.iterations_target`), CLI controls (`--repair-iterations-max`, `--iterations-target`), defaults, and deterministic stop reasons | contract tests and doc/ADR checklist updates |
| S12-T2 | `internal/config/config.go`, config validation/defaulting/tests, `infrafactory.yaml` | CLI flag migration behavior | config supports dual controls with deterministic defaults/validation and no legacy `max_iterations` compatibility path | config load/default/validation tests for both keys and invalid combinations |
| S12-T3 | `internal/cli/root.go`, `internal/cli/run_command.go`, command contract tests/goldens | config parser internals | `run` exposes `--repair-iterations-max` and `--iterations-target`; CLI precedence over config is deterministic; repair-budget exhaustion and target-reaching signals are deterministic | run command parse/usage tests + output contract/golden updates |
| S12-T4 | `internal/cli/run_command_test.go`, integration/orchestration tests, golden fixtures | docs-only updates | run loop behavior validates bounded failure retries and fixed total pass targets, including optional continuation after success | focused unit/integration tests + golden refresh where applicable |
| S12-T6 | `internal/cli/run_command.go`, output/log helpers, command tests | run-loop convergence semantics | each failed iteration writes deterministic failure summaries to app logs before retry/stop; terminal reason is singular and canonical (`target_reached`, `repair_budget_exhausted`, `stuck`) | unit tests asserting failure-log emission and single terminal reason behavior |
| S12-T5 | `README.md`, `SESSION_START.md`, `STATUS.md`, `CURRENT_TICKET.md` | runtime logic changes | user/operator docs reflect dual controls and deterministic terminal reasons; README optimization pass runs after slice completion and records two consecutive no-change passes | doc checklist assertions + hygiene checks |
| S13-T1 | `internal/cli/output_contract.go`, logging helpers (new), docs/ADR surfaces | implementation of instrumentation breadth | logging contract is explicit and deterministic: mandatory fields (command/run_id/iteration/stage/check/status/event), log levels, redaction rules, and format expectations are documented and testable | contract tests + doc/ADR checklist updates |
| S13-T2 | `internal/cli/runtime.go`, command bootstrap/helpers, logger package/files, run artifact path helpers | command-specific logic instrumentation | runtime provides shared logger plumbing for all commands with deterministic behavior across human/json output modes and stable sinks (`stderr` + run-scoped log artifact path) | runtime/logger wiring tests + command bootstrap tests |
| S13-T3 | `internal/cli/run_command.go`, `internal/feedback`, runstore integration points | transport adapter internals | run loop emits structured logs for iteration start/end, failure signatures, stuck/max-iteration decisions, convergence outcomes, and failure class context (for example `iac_validation`, `transport_runtime`, `orchestration_control`) with run/iteration IDs | run command logging tests across success/stuck/max/failure paths including failure-class assertions |
| S13-T4 | `internal/cli/generate_command.go`, `internal/cli/validate_command.go`, `internal/cli/test_command.go`, `internal/cli/mock_*` | run-loop algorithm changes | non-run commands emit deterministic decision-path logs (dependency checks, stage start/end, mapped failures) without leaking secrets | command logging tests for representative success/failure flows |
| S13-T5 | logging fixture/golden files, `internal/cli/*_test.go`, redaction tests | docs-only updates | logging behavior is frozen with regression coverage and credential/prompt redaction assertions | golden snapshot tests + redaction-focused unit tests |
| S13-T6 | `README.md`, `SESSION_START.md`, `STATUS.md`, `CURRENT_TICKET.md` | runtime behavior changes | operator docs include logging runbook (where logs are written, how to correlate run_id/iteration, troubleshooting workflow) and fresh-context startup includes logging guardrails and exact log-inspection commands | doc checklist assertions + hygiene checks |
| S14-T1 | `internal/cli/run_command.go`, `internal/cli/generate_command.go`, prompts/docs planning surfaces | broad logging sink rollout | feedback contract explicitly requires detailed failure payloads (layer/stage/check/command/detail plus optional policy/resource) with failure-class tagging (`iac_validation`, `transport_runtime`, `orchestration_control`) for iteration-to-iteration regeneration; coarse-only errors are non-compliant | contract tests and doc checklist updates |
| S14-T2 | `internal/cli/validate_command.go`, `internal/cli/run_command.go`, validate tests | harness internals | `run` can consume the same structured validate failure output surfaced by `validate` command path (including OPA/policy failures) without losing detail | validate/run unit tests for structured failure propagation |
| S14-T3 | `internal/cli/run_command.go`, `internal/cli/run_command_test.go` | prompt template redesign | `runIteration` feedback payload includes detailed validate/test/generate failure entries, preserves failure-class tagging, and omits synthetic coarse duplicates including duplicate terminal-stop markers | run command tests asserting feedback JSON detail fields and terminal-stop de-dup behavior |
| S14-T4 | `internal/cli/run_command_test.go`, `internal/generator/*_test.go`, golden fixtures as needed | docs-only updates | regression suite prevents fallback to generic `validation failed` feedback when structured details exist; coverage includes transport timeout and parse-failure classes and verifies only one terminal control failure reason is emitted per stop event | focused unit tests + golden refresh if output contract changes |
| S14-T5 | `README.md`, `SESSION_START.md`, `STATUS.md`, `CURRENT_TICKET.md` | runtime behavior changes | docs show how to inspect run artifacts to verify feedback quality and instruct fresh contexts to perform two consecutive no-change refinement passes on slice-planning updates | doc checklist assertions + hygiene checks |
| S15-T1 | `internal/cli/run_command.go`, `internal/feedback`, docs/contract surfaces | transport adapter implementation details | retry governance contract explicitly defines which failure classes are model-correctable vs transport/runtime-limited and how each class affects iteration continuation/stop behavior | contract tests + doc checklist updates |
| S15-T2 | `internal/cli/run_command.go`, `internal/cli/output_contract.go`, run command tests | prompt template content changes | transport-dominated runs stop early with deterministic terminal reason and do not consume full iteration budget unless contract allows | run command tests for transport-dominated early-stop paths and output reason assertions |
| S15-T3 | `internal/generator/*`, `internal/cli/run_command.go`, config surfaces if needed | scenario schema redesign | transport retry budget/backoff behavior is deterministic and bounded; timeout-specific remediation hints are surfaced without leaking secrets | unit tests for retry counters/backoff + run output tests for remediation hint mapping |
| S15-T4 | `internal/runstore`, `internal/cli/run_command.go`, generator metadata adapters | output-contract redesign unrelated to diagnostics | run artifacts persist normalized transport diagnostics per failed iteration with stable schema and backward-compatible reads | runstore/schema tests + iteration artifact assertions |
| S15-T5 | `internal/cli/run_command_test.go`, `internal/generator/*_test.go`, golden fixtures as needed | docs-only updates | regression suite covers mixed failure-class runs and ensures adaptive retry policy avoids max-iteration churn when transport failures dominate | focused unit/integration tests + golden refresh where needed |
| S15-T6 | `README.md`, `SESSION_START.md`, `STATUS.md`, `CURRENT_TICKET.md` | runtime behavior changes | docs provide deterministic troubleshooting split (scenario/IaC fixes vs transport/runtime tuning) and fresh-context notes for adaptive retry diagnostics workflow | doc checklist assertions + hygiene checks |
| S16-T1 | `internal/cli/*_command.go`, `internal/cli/runtime.go`, command integration tests | policy/config semantics | all command/harness/generator/mock call sites use `cmd.Context()` (or derived context) rather than `context.Background()` so signal cancellation propagates deterministically | focused command tests asserting context propagation/cancellation behavior where feasible |
| S16-T2 | `internal/cli/mockway_client.go`, mockway client tests | topology evaluator logic | `State()` reads response bodies with bounded `io.LimitReader` and surfaces deterministic over-limit/truncated-read errors | client tests for normal state read and bounded oversized payload handling |
| S16-T3 | `internal/cli/exec_runner.go`, exec-runner tests | runtime config loading | env overrides are applied with deterministic key de-duplication so each overridden variable appears once in subprocess env | unit tests covering duplicate base keys, override precedence, and stable env output |
| S16-T4 | `internal/cli/runtime.go`, `internal/scenario`, runtime tests/docs | scenario schema redesign | scenario loading does not silently bypass schema validation when schema file is missing; runtime returns deterministic actionable failure or uses guaranteed embedded schema path | runtime/scenario tests for missing-schema behavior and schema-enforced validation failures |
| S16-T5 | `internal/config/config.go`, config tests/fixtures | config schema/field contract changes | empty config files produce explicit deterministic decode failures and no redundant `io.EOF` no-op branch remains | config loader tests for empty file and malformed YAML decode failures |
| S16-T6 | `policies/scaleway/encryption_at_rest.rego`, `policies/common/naming.rego`, policy tests/docs | CLI runtime orchestration logic | policy checks/messages align with real semantics (no versioning-as-encryption confusion) and naming rule behavior matches intended minimum-length contract | policy regression tests for bucket encryption/versioning intent and single-character naming behavior/message |
| S16-T7 | `internal/cli/runtime.go`, `internal/harness/run_record.go`, focused tests | broader run-loop behavior changes | remove unused `notImplementedRuntime` path and explicitly set run metadata schema in destroy persistence path for consistency/auditability | compile + focused tests covering destroy-run metadata schema field |
| S16-T8 | `README.md`, `SESSION_START.md`, `STATUS.md`, `CURRENT_TICKET.md`, `ROADMAP.md`, `BACKLOG.md` | runtime logic changes | docs reflect Slice 16 outcomes; refinement loop records one improvement pass plus two consecutive no-change passes for closure | doc checklist assertions + `bash scripts/check_all.sh` |
| S17-T1 | `internal/generator/*`, `internal/cli/run_command.go`, `internal/runstore`, `README.md`, `SESSION_START.md` | default logging contract changes outside capture mode | opt-in-only raw LLM stage responses are persisted per iteration/phase in run artifacts with deterministic redaction and byte caps; default behavior remains unchanged | focused generator/runtime tests for disabled-by-default behavior, enabled capture writes, redaction, and truncation/size-limit enforcement |
| S18-T1 | `scenarios/training/k8s-cluster-paris.yaml`, mockway `handlers/k8s.go`, mockway `handlers/k8s_test.go` | new mockway services, schema extensions | K8s standalone scenario passes `infrafactory run` on first iteration; mockway K8s PATCH (update) handler exists; cluster delete cascades to pools; mockway K8s CRUD tests pass | mockway K8s CRUD + cascade delete + update tests; infrafactory first-iteration pass |
| S18-T2 | `scenarios/training/iam-policies-paris.yaml`, `scenario.schema.json`, `internal/scenario/scenario.go`, mockway `handlers/iam.go`, mockway `handlers/iam_test.go` | new mockway services beyond IAM | IAM standalone scenario passes `infrafactory run` on first iteration; scenario schema extended with `iam` resource type; `scenario.go` `Resources` struct includes `IAM` field; mockway IAM update handlers exist if missing; mockway IAM CRUD tests pass | mockway IAM CRUD tests; scenario schema validation tests for `iam`; infrafactory first-iteration pass |
| S18-T3 | `scenarios/training/registry-paris.yaml`, `scenario.schema.json`, `internal/scenario/scenario.go`, mockway `handlers/registry.go`, mockway `repository/repository.go`, mockway `handlers/registry_test.go`, `prompts/phase2_generate_hcl.md` (if needed) | redis, composite scenarios | Registry scenario passes `infrafactory run` on first iteration; mockway Registry namespace CRUD (POST/GET/LIST/PATCH/DELETE at `/registry/v1/regions/{region}/namespaces`) implemented; `container_registry_namespaces` table in mockway repo; scenario schema extended with `registry` resource type | mockway registry CRUD tests; infrafactory first-iteration pass |
| S18-T4 | `scenarios/training/redis-paris.yaml`, `scenario.schema.json`, `internal/scenario/scenario.go`, `mappings.yaml`, mockway `handlers/redis.go`, mockway `repository/repository.go`, mockway `handlers/redis_test.go`, `prompts/phase2_generate_hcl.md` (if needed) | registry, composite scenarios | Redis scenario passes `infrafactory run` on first iteration; mockway Redis cluster CRUD (POST/GET/LIST/PATCH/DELETE at `/redis/v1/zones/{zone}/clusters`) implemented; `redis_clusters` table in mockway repo; `mappings.yaml` includes redis sizes (RED1-MICRO, RED1-S, RED1-M, RED1-L); scenario schema extended with `redis` resource type | mockway redis CRUD tests; infrafactory first-iteration pass |
| S18-T5 | `scenarios/training/full-stack-paris.yaml` | individual service scenarios | Composite scenario with compute + networking + database + K8s + IAM + registry + redis passes `infrafactory run` on first iteration; cross-resource interaction issues resolved | infrafactory first-iteration pass for composite scenario |
| S19-T1 | All Slice 18 files in mockway and infrafactory (scenarios, handlers, schema, mappings, prompts, scenario.go, schema_filter.go) | features unrelated to Slice 18 | 1. All Slice 18 code reviewed for bugs, edge cases, error handling, and correctness. 2. All identified issues fixed with regression tests. 3. All standalone scenarios (`k8s-cluster-paris`, `iam-policies-paris`, `registry-paris`, `redis-paris`) and composite (`full-stack-paris`) pass `infrafactory run` on first iteration after fixes. 4. `go test ./...`, `cd ../mockway && go test ./...`, and `bash scripts/check_all.sh` all pass. 5. Run all scenarios repeatedly until no further issues found. | Regression tests for each bug fixed; all scenario first-iteration passes; mockway + infrafactory test suites green |
| S20-T1 | `scenarios/training/mysql-ha-paris.yaml`, mockway handlers (if mysql-specific fixes needed), prompts (if mysql/HA hints needed) | new mockway services, schema extensions | MySQL HA scenario passes `infrafactory run` on first iteration; mysql engine + medium size + HA=true + private networking exercised | scenario schema validation; infrafactory first-iteration pass |
| S20-T2 | `scenarios/training/compute-lb-multi-paris.yaml`, mockway handlers (if multi-backend fixes needed), prompts (if needed) | new mockway services | Multi-backend LB scenario passes `infrafactory run` on first iteration; large compute (count=3) + multi-backend (80/http + 443/tcp) exercised | scenario schema validation; infrafactory first-iteration pass |
| S20-T3 | `scenarios/training/k8s-medium-override-paris.yaml`, mockway handlers (if override-specific fixes needed) | new mockway services | K8s medium override scenario passes `infrafactory run` on first iteration; medium K8s + explicit node_type/node_count overrides exercised | scenario schema validation; infrafactory first-iteration pass |
| S20-T4 | `scenarios/training/private-lb-db-paris.yaml`, mockway handlers (if private LB fixes needed) | new mockway services | Private LB + large DB override scenario passes on first iteration; private LB + large PostgreSQL with node_type/engine_version overrides exercised | scenario schema validation; infrafactory first-iteration pass |
| S20-T5 | `scenarios/training/public-registry-iam-paris.yaml` | new mockway services | Public registry + selective IAM scenario passes on first iteration; is_public=true registry + IAM with policy=false exercised | scenario schema validation; infrafactory first-iteration pass |
| S20-T6 | `scenarios/training/redis-xlarge-session-paris.yaml`, mockway handlers (if xlarge-specific fixes needed) | new mockway services | Redis xlarge override scenario passes on first iteration; xlarge Redis with node_type override + xlarge compute exercised | scenario schema validation; infrafactory first-iteration pass |
| SUi-1 | `ui/` (SvelteKit scaffold), `cmd/infrafactory/embed.go`, `cmd/infrafactory/embed_dev.go`, `internal/api/server.go`, `internal/api/spa.go`, `internal/api/handlers_config.go`, `internal/cli/ui_command.go`, `internal/cli/root.go`, `cmd/infrafactory/main.go`, `Makefile`, `.gitignore` | REST handlers, WebSocket, frontend components | `make build && ./bin/infrafactory ui` serves placeholder SvelteKit page at `127.0.0.1:4173`; `go test -tags noui ./...` passes; `NewRootCmd(opts ...RootOption)` functional options pattern implemented; `GET /api/config` returns redacted config | server startup test; SPA fallback test; config redaction test; existing CLI tests pass with `noui` tag |
| SUi-2 | `internal/api/handlers_scenarios.go`, `ui/src/lib/api.ts`, `ui/src/lib/types.ts`, `ui/src/lib/stores/scenarios.ts`, `ui/src/routes/+layout.svelte`, `ui/src/lib/components/Sidebar.svelte`, `ui/src/routes/scenarios/[path]/+page.svelte` | run history, WebSocket, code viewer | Sidebar lists scenarios grouped by directory; clicking scenario shows raw YAML + parsed metadata; `GET /api/scenarios` returns grouped list | handler unit tests with `httptest`; scenario list/detail tests |
| SUi-3 | `internal/api/handlers_runs.go`, `internal/runstore/runstore.go` (`ListScenarios`, `ReadIterationArtifact`, generated-file helpers), `ui/src/routes/runs/` pages, `ui/src/lib/components/IterationTimeline.svelte`, `RunStatusBadge.svelte`, `FailureCard.svelte` | live execution, YAML editor | Browse `.infrafactory/runs/` data ordered newest-first; expand iterations to see stage results and failure details; `GET /api/runs/{scenario}/{runID}/iterations/{n}` returns iteration JSON; run detail route exposes per-run IaC viewer plus iteration snapshots, diff support, IaC bundle/full-artifact downloads, and `GET /api/runs/{scenario}/{runID}/files{,/path...}` | handler unit tests; `ListScenarios` + `ReadIterationArtifact` + generated-file runstore unit tests; iteration timeline rendering |
| SUi-4 | `internal/api/handlers_output.go`, `ui/src/lib/components/TfViewer.svelte`, `ui/src/routes/output/[scenario]/+page.svelte` | scenario editor, WebSocket | `GET /api/output/{scenario}` lists .tf files (excludes `.terraform/`, state); `GET /api/output/{scenario}/{file}` returns file content; scenario-level output remains the mutable latest output while immutable per-run IaC history is served from run-scoped APIs; per-run viewer renders richer syntax-highlighted IaC | handler unit tests; path traversal prevention test |
| SUi-5 | `internal/api/hub.go`, `internal/api/client.go`, `internal/api/ws_sink.go`, `ui/src/lib/ws.ts`, `ui/src/lib/stores/liveRun.ts`, `ui/src/lib/components/LogStream.svelte`, `go.mod` | run execution, YAML editor | WebSocket upgrade at `/api/ws`; `WebSocketSink` implements `io.Writer` and plugs into `AppLogger.sinks`; hub broadcasts to all connected clients; slow clients are dropped | hub register/unregister/broadcast unit tests; WebSocket connect/disconnect integration test |
| SUi-6 | `internal/api/handlers_run_executor.go`, `internal/cli/run_command.go` (extract `executeRunLoop` + `EventEmitter` callback), `internal/runstore/runstore.go` (`TerminalReason` field in `RunMetadata`), `ui/src/lib/components/PipelineView.svelte`, `StageNode.svelte`, `RunResultBanner.svelte`, `ui/src/routes/live/+page.svelte`, `/diagnostics` page | YAML editor, code viewer | `POST /api/runs/{scenario}/start` starts async run (202); WebSocket streams typed events (`stage_start`, `stage_complete`, `iteration_complete`, `run_complete`) via `EventEmitter`; pipeline nodes animate through pending/running/pass/fail; `RunResultBanner` shows terminal reason; single concurrent run mutex enforced (409); diagnostics/readiness checks and clearer failure hints are available from the UI | handler unit test; `executeRunLoop` extraction test; `EventEmitter` invocation test; diagnostics tests; existing `run` command tests still pass |
| SUi-7 | `ui/src/lib/components/YamlEditor.svelte`, `internal/api/handlers_scenarios.go` (PUT validation) | live execution, code viewer | `PUT /api/scenarios/{path}` validates YAML against scenario schema before saving (422 on error); editor shows inline validation feedback; path traversal prevented | PUT handler validation test; path traversal prevention test; schema error response test |
| SUi-8 | `goreleaser.yml`, `Makefile`, `README.md` | feature implementation | `goreleaser build --snapshot --clean` produces binary with embedded UI; `make build` chains `ui-build` then Go compile; README documents `infrafactory ui` command and dev workflow | goreleaser snapshot build; `make build` success; README section exists |

## Operating notes
- Update `status` and dependencies as work evolves.
- Keep exactly one `in_progress` ticket at a time.
- Use `CURRENT_TICKET.md` for session-level execution details.
- Post-slice review-improve protocol (for unfinished slices): after each slice implementation, run a review pass, apply improvements, and repeat until two consecutive passes report no further improvements; log each pass in `STATUS.md` and `CURRENT_TICKET.md`.
- Blocked-slice refinement protocol: for blocked tickets (for example `S9-T8`), refinement scope is governance/docs/risk communication only; do not implement blocked runtime behavior unless blocking ADR/policy is explicitly superseded.
- **Slice 18 autonomous execution rule**: Do NOT ask for human confirmation during `S18-T1`..`S18-T5` execution. Write scenarios, run infrafactory, diagnose failures, fix mockway or infrafactory, write regression tests, rebuild Docker, and rerun autonomously until each scenario achieves first-iteration pass. This includes fixing bugs in either codebase, extending schemas, adding mockway handlers/tables, and updating prompts — all without stopping to ask.
- **Slice 19 autonomous execution rule**: Do NOT ask for human confirmation during `S19-T1` execution. Review all Slice 18 code in both mockway and infrafactory, identify bugs/issues/edge cases, fix them with regression tests, rebuild Docker, and rerun all scenarios until every one passes on first iteration. Keep running until all issues are resolved — no human interaction required.
