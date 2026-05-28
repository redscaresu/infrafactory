# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added (M86–M95, 2026-05-28 — auto-learning closure + CI hardening session)
- **Auto-learning loop closes itself across all 3 clouds** (M86 + M90 + M91 + M92): three stacked bugs in `internal/generator/pitfalls_learn.go` + `internal/cli/run_command.go` had been silently dropping every apply-time learning opportunity since at least M83. M86 reordered `ExtractLearnedPitfall` so resource-extraction runs before the `genericPatterns` substring rejection ("exit status" was killing every tofu envelope), stripped ANSI codes before truncation, and bumped the failure-detail budget 600 → 2400 chars so resource names actually land in the failure summary. M90 wired `stuck`-detection (2-iter repeated-signature) into the learning path — previously only oscillation (3-iter toggle) triggered learning. M92 extended `resourceNameRe` from `(?:scaleway|google)_\w+` to include `aws_*`. After the chain: first `source: learned` AWS pitfall ever extracted from a stuck `aws-dynamodb` run.
- **M91 ratchet — no human-authored pitfalls.** Stripped 31 `source: seed` entries from `pitfalls/aws.yaml`, 8 from `pitfalls/gcp.yaml`, 14 `source: static` entries from `pitfalls/scaleway.yaml`. The seed crutch had been hiding M86 + M90 bugs for months — every GCP/Scaleway scenario converged before stuck-detection could trip because seeds fed the LLM the answer. Added `TestPitfallsNoHumanSeeding` in `internal/generator/` that fails CI if any pitfalls entry has source != "learned". User principle: "seeding is cheating. The system has to learn itself."
- **M89 scenario-change-gate.** `.github/workflows/scenario-gate.yml` + `scripts/scenario_change_gate.sh` block any PR that touches `scenarios/training/*.yaml` from merging if any changed scenario fails to converge (status != success OR terminal_reason != target_reached). Triggers only on scenario diffs so LLM cost stays bounded to once-per-PR. Gracefully degrades when `OPENROUTER_API_KEY` secret isn't configured.
- **M75 cross-repo parity harness.** `internal/e2e/cross_repo_parity_test.go::TestCrossRepoParity_EveryLandedServiceHasScenario` reads each sibling fake's `handlers/regression_manifest.go::LandedServices` slice and asserts every entry maps to ≥1 scenario in `scenarios/training/` or is explicitly exempted with a written reason. Mockway/fakeaws/fakegcp adding a new handler set without an upstream scenario fails this test on the next infrafactory push.
- **M85 vacuous-audit guard** — `TestRegressionSeedAuditHasPatterns` added to all 3 fakes (`fakeaws@13fb49c`, `fakegcp@2959404`, `mockway@526f487`). Counts `^TestRegression` funcs (excluding `^TestRegressionSeedAudit`), fails if below `min(len(LandedServices), 8)`. Prevents the M75/M79 "audit passes vacuously because patterns list is empty" pattern from returning.
- **M88 + M93 + M94 + M95 sweep harnesses** under `scripts/` for repeatable diagnostic runs: `m88_sweep.sh` (full 39-scenario sweep), `m93_resweep.sh` (failures-only re-run), `m94_aws_proof.sh` (AWS-only with SeaweedFS up), `m95_multipass.sh` (multi-pass convergence study). Portable perl-alarm timeout (macOS lacks `timeout`). M88 baseline + M94 AWS results captured in `docs/m88-sweep-results.tsv` + `docs/m94-aws-resweep-results.tsv`.
- **M94 SeaweedFS in `make mocks-up`** — `seaweedfs-up` / `seaweedfs-down` targets manage the chrislusf/seaweedfs container on port 9090. Every AWS scenario silently failed at `s3 reset: connection refused` before this — root cause of M88's 0/11 AWS pass rate.
- **`docs/scenario-failure-matrix.md`** classifies all 39 training scenarios as pass / fixed-by-SeaweedFS / LLM-stuck.
- **M82 Dependabot** at `.github/dependabot.yml` — weekly grouped updates for gomod + npm (ui only) + github-actions; major vite/sveltekit/svelte bumps get individual review PRs.
- **M84 BACKLOG archive sweep** — BACKLOG.md 507 → 226 lines; 281 done/wontfix slice tickets moved to `BACKLOG_ARCHIVE.md` with identical schema for future sweeps.
- **M81 isolated test-runtime helper** (`isolatedRunOpts` in `internal/cli/command_test_harness_test.go`) + runtime guard at `run_command.go` that fails any test using a relative `cfg.Paths.Output` under `testing.Testing()`. Refactored 27 callsites across `run_command_test.go` / `integration_orchestration_test.go` / `run_command_oscillation_test.go`. Locks in the M71 filesystem-race fix permanently.
- **M73 README badges parity** across the 4 repos — fakeaws/fakegcp/mockway now match infrafactory (CI / License / Go-version).
- **M83 gcp-memorystore proven end-to-end** through the LLM-driven pipeline; converges in 3 iterations against fakegcp + the memorystore handler's `/v1/projects/{p}/locations/{loc}/instances` surface.

### Changed (M71–M82, M87)
- **M71 — `go test -race` in CI.** infrafactory was the only repo of the 4 without race detection; now matches fakeaws/fakegcp/mockway. Combined with the existing `-count=2`, surfaces both Go-memory races and parallel-subtest filesystem collisions.
- **M77 — Go 1.25.0 toolchain.** infrafactory + mockway bumped from 1.24.x to align with fakeaws + fakegcp. Cross-repo shared deps aligned: `testify` 1.11.1, `modernc.org/sqlite` 1.50.0.
- **M76 — UI dep majors.** vite ^6 → ^8, @sveltejs/kit ^2.15 → ^2.61.1, @sveltejs/vite-plugin-svelte explicit ^7.1.2, @tailwindcss/vite ^4 → ^4.3. Closes 4 high-severity npm CVEs (vite path-traversal + WS fs-bypass + @sveltejs/kit BODY_SIZE_LIMIT bypass).
- **M78 — Terraform plugin cache in CI** via `actions/cache@v4` + `TF_PLUGIN_CACHE_DIR`. Saves ~200MB provider download per CI run.
- **M80 — `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24=true`** at workflow scope, ahead of GitHub's June 2 2026 forced flip. Verified vite-8 + sveltekit + playwright + Go 1.25 stack survives Node 24.
- **M87 — README accuracy pass.** CLI banner ("for Scaleway" → "for AWS, GCP, and Scaleway"), badge + quickstart Go pin (1.24+ → 1.25+), `make mocks-up` description includes SeaweedFS:9090, `infrafactory mock start/stop/status/logs` cobra Short strings + README command table honestly describe Mockway-only scope (pointing at `make mocks-*` for all-3 coverage). `make mocks-status` + `make mocks-down` now probe the listening port via `lsof` as source of truth (pidfile-bookkeeping bug fix — `go run` parent PID differed from binary child PID).

### Fixed (M86, M90, M92, M94)
- **M86** — pitfalls auto-learning was suppressed for every apply-time failure (the most common error shape) because `genericPatterns` substring-rejected "exit status". The resource-extraction fallback now runs BEFORE generic-pattern rejection.
- **M86** — ANSI escape codes (tofu emits liberal color codes) were consuming the 600-char stderr truncation budget so resource names couldn't fit in the failure detail. `stripAnsi()` runs before truncation; budget bumped to 2400.
- **M90** — the "auto-learn on stuck/exhausted" branch at `run_command.go:291` was dead code. `IsStuck` fires at 2 iterations but `DetectOscillation` requires ≥3 with a toggle, so the most common failure mode (LLM makes the same mistake twice) produced zero learned pitfalls. Now also extracts pitfalls from the last iteration's failure signatures directly when oscillation returns nothing.
- **M92** — `resourceNameRe = (?:scaleway|google|aws)_\w+` (was scaleway|google only). AWS scenarios couldn't auto-learn even with M86+M90 active because no resource name extracted.
- **M94** — `make mocks-up` now starts SeaweedFS (port 9090). Previously every AWS scenario failed at `s3 reset: connection refused` before the LLM was invoked; the M88 0/11 AWS rate had nothing to do with the model.
- **M72** — `gitleaks` CI job added; previously only the local pre-commit hook enforced secret scanning (bypassable with `--no-verify`, no fork-PR coverage).

### Security
- M72 — gitleaks now runs as a required CI job; the local pre-commit hook is no longer the only defense.

### Removed
- M91 — every `source: seed` and `source: static` entry from `pitfalls/{aws,gcp,scaleway}.yaml`; total -53 entries. The auto-learning loop now reconstructs lessons from real runs.

---

### Added
- `TestE2E_AWSFullStack` now exercises the full multi-service composition (VPC + 2 subnets + 2 IAM roles + EKS cluster + node group + S3 bucket + RDS subnet group + parameter group + Postgres DB instance + Secrets Manager secret + version) end-to-end through infrafactory's 4-layer pipeline; 232s per run. Closes M61 + M62 + M57.
- Third-party S3 backend integration (M59): SeaweedFS replaces fakeaws's stripped S3 handler for `terraform-provider-aws`'s bucket Read flow. `cloudMockStateRouter` dispatches by (cloud, service) tuple; `mergeS3IntoAWSState` polyfills the S3 surface into the AWS state JSON; `resetS3Backend` uses the standard S3 list+delete contract. SeaweedFS chosen after empirical evaluation against Adobe S3Mock (returns `ListBucketResult` for `?policy` queries instead of 404), Garage (AGPLv3 + cluster bootstrap), LocalStack (community killed), MinIO (no longer maintained). `CONCEPT.md` "Third-Party Mock Integration" section documents the decision trail.
- Containerized multi-mock orchestration (M60): `docker-compose.mocks.yml` ships all four mocks (mockway:8080 / fakegcp:8081 / fakeaws:8082 / SeaweedFS:9090) with sibling-source builds as the default. Makefile gains `mocks-up-containers` / `mocks-down-containers` / `mocks-pull` / `mocks-status-containers` / `mocks-logs-containers`. Healthchecks wired through every service.
- Per-cloud full-stack composition scenarios + e2e tests: `aws-full-stack.yaml` joins the existing GCP + Scaleway full-stack scenarios; `TestE2E_AWSFullStack` mirrors `TestE2E_GCPFullStack` and `TestE2E_FullStackParis`.
- UI walkthrough demo recording: `docs/demo/ui-walkthrough.webm` (24s) captures the `full-stack-paris` scenario end-to-end via the SvelteKit dashboard; `make demo-ui` re-records via Playwright.
- Pre-commit hook auto-refreshes Playwright visual baselines when `scenarios/training/*.yaml` changes (M56). Wired via `make install-hooks`.
- AWS provider support end-to-end (Slices 43–48): the `fakeaws` sibling mock landed 9 services across 5 wire formats (IAM, S3, EC2, RDS, DynamoDB, EKS, SQS, Route53, Secrets Manager); 11 `aws-*` training scenarios; `policies/aws/` rego files (no_public_db, vpc_required, region_restriction, encryption); `pitfalls/aws.yaml`; provider schema extraction wired for `hashicorp/aws ~> 5.70`; cross-repo e2e tests under `internal/e2e/aws_services_test.go`.
- GCP provider support end-to-end (Slice 36): the `fakegcp` sibling mock; 8 `gcp-*` training scenarios; `policies/gcp/` rego files; `pitfalls/gcp.yaml`; provider schema extraction wired for `hashicorp/google`; cross-repo e2e tests + a `TestE2E_GCPDoubleApplyIdempotency` idempotency check (S36-T11).
- Multi-cloud UI (Slice 42 + S36-T12): SvelteKit web UI groups scenarios by cloud in the sidebar; each scenario page now surfaces a cloud-pill badge and a dynamic "Layer 3 (Real Scaleway/GCP/AWS)" label; `mock_provider` status pill reflects whether `mockway`, `fakegcp`, or `fakeaws` is the active backend.
- Visual regression + functional spot-check Playwright suites (Slice 40): 7 captured baselines under `ui/e2e/visual.spec.ts-snapshots/`; threshold gates configured in `playwright.config.ts`; spot-checks for every page + error-state coverage (unknown SPA route, missing scenario, 404 surfaces).
- Per-cloud provider auto-injection (M38 + Slice 14/15): `ensureGoogleProviderWiring` and `ensureScalewayProviderWiring` add the `required_providers` + `provider {}` blocks when a scenario's HCL references resources from that cloud's prefix.
- `cloudMockStateRouter` (S43-T9) dispatches every Layer-2 admin call (`/mock/state`, `/mock/reset`, `/mock/snapshot`, `/mock/restore`) to mockway / fakegcp / fakeaws based on the loaded scenario's `cloud:` field — a single `infrafactory run` invocation iterates across all three clouds without restarting any mock.
- Per-criterion `params` field on policy criteria (S51) — replaces the previous top-level `constraints:` scenario map; `EvaluatePlanPoliciesWithParams` reads `input.params` instead of `input.constraints`; `EnsureProviderSchema(ctx, cloud)` now caches per cloud.
- Apache-2.0 LICENSE rolled out across all four sibling repos (`infrafactory`, `mockway`, `fakegcp`, `fakeaws`) so the project family is licensed consistently.

### Changed
- README rewritten for OSS clarity — 1022 → 188 lines; reorganized around the Quickstart-then-coverage-table flow; ADR moved out of the main body; demo placeholders + UI walkthrough section added.
- Scenario YAML schema: dropped the top-level `constraints:` field; parametric values (`region`, `zone`) now live as `params:` on the `region_restriction` policy criterion. 35 scenarios in `scenarios/training/` + 1 in `scenarios/holdout/` migrated in S51-T4.
- `policies/scaleway/region_restriction.rego` reads `input.params.{region,zone}` instead of `input.constraints.{region,zone}`. The other two `region_restriction.rego` files (GCP, AWS) already did the right thing for the new shape.
- CI workflow now sets up Node 20 and runs `npm ci && npm run build` before `go test`, fixing the long-running `go:embed` failure on every push (`cmd/infrafactory/embed.go` requires `cmd/infrafactory/ui/build/` which is gitignored).
- Validate-time policy filtering now drops `./policies/{other-cloud}/` subdirs based on the scenario's `cloud:` field (`filterPolicyPathsByCloud`) — an AWS scenario no longer has Scaleway or GCP regos vacuously evaluated against its plan.

### Removed
- `Scenario.Constraints` Go field, `scenarioDetailResponse.Constraints` API field, `ScenarioDetail.constraints` TS type, `PromptContext.Constraints` template field, and the always-empty `Constraints` field on `ClaudeTransportConfig` / `OpenRouterTransportConfig` (S51-T3).
- S49 fakeaws fault-injection / IAM-attach latency knob — briefly shipped at `fakeaws@0fa51c2`, reverted at `fakeaws@26e97c8` on design grounds (mocks optimize for fast feedback, not real-cloud fidelity).

### Security
- `gitleaks` pre-commit hook enforced via `make install-hooks`; full-history sweep across all four repos (413 commits) returned zero leaks (verified 2026-05-23).
- `SECURITY.md` added with private vulnerability reporting via GitHub Security Advisories and an email backup.
