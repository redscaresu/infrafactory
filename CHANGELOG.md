# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
