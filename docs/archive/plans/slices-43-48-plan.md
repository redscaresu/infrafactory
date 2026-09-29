# Plan: Slices 43-48 — fakeaws (AWS API mock + infrafactory integration)

## Context

Slices 43-48 build a third sibling mock for AWS, modelled after mockway (Scaleway) and fakegcp (GCP). LocalStack consolidated into a paid product in April 2026; fakeaws keeps the freedom-to-modify story alive for infrafactory's AWS scenarios.

The full design lives in `../fakeaws/concepts.md` (sibling repo). This plan covers the per-slice deliverables and exit criteria so the work can be picked up incrementally by any agent.

## Quick Reference

| Key | Value |
|---|---|
| Slices | 43–48 |
| Ticket IDs | S43-T1 through S48-T8, plus S44-T0 / S45-T0 / S46-T0 / S47-T0 (per-phase pitfalls-extension tickets added in pass 2; the per-service prompt-matrix design they originally carried was retracted in pass 4 — see concepts.md § "Resolved decisions" item 11) plus M39 + M40 (maintenance — ARN builder + cross-pollination policy) |
| Total tickets | 68 fakeaws S-ticket rows (S43 14 + S44 13 + S45 11 + S46 11 + S47 11 + S48 8) of which 2 are vacated/MOVED in S48 (66 active), plus 2 maintenance M-tickets (M39 + M40) folded into S43-T2 / S48-T4 acceptance — see concepts.md § Phasing |
| Depends on | Slice 42 (multi-cloud UI), Slice 36 (GCP infra) |
| Out-of-tree work | New sibling repo `fakeaws/` (alongside the infrafactory checkout) |

## Quality bar

Non-negotiable; every slice exit gate enforces this. Detail in `fakeaws/concepts.md` § "Quality guarantees".

The bar that mockway and fakegcp set:
- 280+ handler tests in mockway, 90+ tests + 33 codex review passes for fakegcp.
- Every contract pinned by a test before the feature is considered shipped.
- Every wire shape driven through the live `hashicorp/aws` provider via `tofu apply → plan -detailed-exitcode → destroy` at least once.
- Two consecutive codex `NOTHING_TO_IMPROVE` review passes scoped to each phase's diff.

The mechanism that landed that bar without paying for it post-hoc: gates wired into the workflow *before* writing handler code.

### Per-phase exit gates (S43–S47, ten gates each)

1. CRUD test for every resource in scope (Create → Get → List → Update → Delete → 404).
2. FK violation tests for every cross-resource reference (same-account + cross-account).
3. Cascade / dependent-delete tests for every parent-child FK.
4. Update-path FK tests (post-merge validation, mirror of fakegcp pass 28).
5. State-machine tests where applicable (terminal-state, status transitions).
6. `examples/working/<service>` applies + plans no-op + destroys cleanly.
7. `examples/misconfigured/<service>` fails with the correct error code through tofu.
8. `examples/updates/<service>` reaches v2 in-place via `v1.tfvars → v2.tfvars`.
9. `TestE2E_AWS_<Service>` gated runner (mirror of `runGCPServiceScenario`) green.
10. Two consecutive codex `NOTHING_TO_IMPROVE` passes scoped to phase diff.

### Phase 6 (S48): codex review iteration loop

Slice 48 is dedicated to the same loop that landed fakegcp. Budget: 20–35 passes based on fakegcp's 33. Restart count on any `BLOCKING:` finding; only `NOTHING_TO_IMPROVE` advances the counter. Cross-pollinate findings back to mockway/fakegcp where they apply.

### Day-1 invariant: tracked secret-scanning gate before any handler code

`.git/hooks/` is not versioned, so a tracked hook file there is not shippable as a "first-commit" gate. Instead, the fakeaws repo's *first commit* contains all four of: `.gitleaks.toml` (allowlist contract), `.githooks/pre-commit` (executable, tracked, runs `gitleaks protect --staged --no-banner` *before* `go test ./...`), `Makefile` with `install-hooks` target (sets `git config core.hooksPath .githooks`; idempotent), and `.github/workflows/ci.yml` running `gitleaks detect --redact --no-banner --source=.` as a required check on `main`. Local hook is fast-fail convenience; CI is the security boundary. S43-T1 acceptance criteria enforce all four; S48-T8 is reduced to a sweep/audit (synthetic `AKIAIOSFODNN7EXAMPLE` injection test against both the local hook and CI). Detail in `fakeaws/concepts.md` § "Secret scanning from day one".

### Day-1 invariants beyond secret scanning

Codex review pass 1 caught that several patterns the plan claimed to "mirror from fakegcp" had been deferred to Phase 6 — defeating the load-bearing fakegcp-pass-1 lesson that quality discovered post-hoc is exactly what makes the codex loop blow up. Three additional Day-1 deliverables landed in S43 (not S48):

- **Pre-seeded `regression_test.go`** with the 16 standing patterns (13 from fakegcp's 33-pass loop + 3 unique-from-mockway 14-bug catalogue, after pass-17 collapsed one duplicate "resource-existence gate" bullet). Tests stub-fail until the corresponding handler exists; vacuous-pass detection ensures no stub silently green-lights an unimplemented contract.
- **Cross-resource FK validators** (`resolveSameAccountName` helper + post-merge PATCH-validation pattern) live in the repository skeleton from line one, not retrofitted in Phase 6.
- **`internal/harness/destroy.go::countOrphans` extension** for fakeaws's universal bookkeeping tables `operations` + `audit` (S43), with `sqs_messages` appended in S46 when SQS lands. DynamoDB streams are out of scope at v1 so no streams cursor table exists. countOrphans is an always-on destroy gate — pushing the universal tables to S48 means every interim phase's destroy assertions fail spuriously throughout S43-S47.

### Standing patterns to seed `regression_test.go` on day one

Canonical list (16 patterns: 13 drawn from fakegcp's 33-pass findings + 3 unique from the mockway 14-bug catalogue, after pass-17 deduplication) lives in `fakeaws/concepts.md` § "Standing patterns to seed regression_test.go on day one". S43-T10's acceptance criterion requires every pattern there to be pre-pinned in `regression_test.go` before Phase 1 handlers land (was previously deferred to S48-T1; pass-1 codex review moved Day-1 seeding to S43-T10).

---

## Slice 43: Foundation — IAM + S3 + complete infrafactory integration

Boot the repo with the secret-scanning + CI gates *and* every Day-1 invariant codex pass-1 surfaced. Land `awsproto/` with per-protocol error mappers, the repository skeleton with cache lifecycle baked in, IAM (foundational — every other service references roles/policies/instance-profiles), then S3, then the *full* infrafactory integration surface (**16 surfaces** codex found hard-coded for two clouds, including the lazy provider-schema invocation restructure), the generator surfaces (prompts/aws, pitfalls/aws, policies/aws — with loader plumbing) so the agent loop can emit AWS HCL, and a regression-test seed of all 16 standing patterns. IAM lands before S3 because every later service depends on it.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S43-T1 | fakeaws: **`git init` in the sibling `fakeaws/` checkout is the first action** (the directory exists today only as a concepts.md draft, not as a git repo); then repo scaffold + Day-1 gates in commit 1 (cmd, handlers, models, testutil, Makefile incl. `install-hooks` target, README, AGENTS.md, go.mod, **`.gitleaks.toml` allowlisting `examples/.*\.tf$`, tracked `.githooks/pre-commit` running `gitleaks protect --staged --no-banner` then `go test ./...`, `.github/workflows/ci.yml` with gitleaks+test+race+vet+build as required checks, `coverage_matrix.yaml` schema-only header (entries land per service-bundle PR), provider-version constraint file recording `hashicorp/aws ~> 5.70` per concepts.md "Resolved decisions" item 14**); commit author `redscaresu <ukashouri@gmail.com>` | P1 | — |
| S43-T2 | fakeaws: `awsproto/` helper package — XML response writer, JSON 1.0/1.1 helpers, x-amz-target parser, query-RPC parser, **per-protocol error mappers** (one per wire format, each tested for `ErrInUse`/`ErrTerminalState`/`ErrConflict`/`ErrNotFound` round-trip into terraform-provider-aws's expected error code and shape) | P1 | S43-T1 |
| S43-T3 | fakeaws: `repository/repository.go` skeleton — modernc.org/sqlite, file-backed (`--db`) and in-memory modes, schema migrate, FK enforcement, snapshot/restore lifecycle covering SQLite *plus* in-process caches that exist in v1 (SQS visibility timeouts in S46; Route53 change-id cache in S47; DynamoDB stream cursors are out of v1 scope), `models.Err*` sentinels including `ErrInUse` / `ErrTerminalState`, `resolveSameAccountName` + post-merge PATCH validation helpers | P1 | S43-T1 |
| S43-T4 | fakeaws: `handlers/admin.go` (/mock/reset, /mock/snapshot, /mock/restore, /mock/state, /mock/state/{service}) + admin_test.go; `/mock/state` schema documented inline so topology_derive_aws has a stable contract | P1 | S43-T3 |
| S43-T5 | fakeaws: repository support for iam_roles + iam_policies + iam_instance_profiles + iam_users + iam_access_keys + role_policy_attachments (FK from attachments → roles + policies) | P1 | S43-T3 |
| S43-T6 | fakeaws: `handlers/iam.go` (Role / Policy / InstanceProfile / User / AccessKey CRUD + AttachRolePolicy / DetachRolePolicy / AddRoleToInstanceProfile) + handlers_test covering EVERY endpoint with success path + 404 + FK-violation cases | P1 | S43-T5, S43-T2 |
| S43-T7 | fakeaws: repository support for s3_buckets + s3_bucket_configs (versioning / encryption / policy / public-access-block / ownership-controls / tagging) | P1 | S43-T3, S43-T6 |
| S43-T8 | fakeaws: `handlers/s3.go` (Bucket CRUD + versioning + encryption + tagging + policy + public-access-block + ownership-controls; Object PUT/HEAD/GET/DELETE/List with payload discarded) + handlers_test covering EVERY endpoint with success path + 404 + FK-violation cases **where applicable** (S3 v1 has no cross-resource FK refs per concepts.md § "Resource coverage matrix § S3" — bucket policy refs are inert strings at v1; the misconfigured-exemption is documented in `examples/README.md` per S43-T12). | P1 | S43-T7, S43-T2 |
| S43-T9 | infrafactory wiring (the *full* surface — single ticket so nothing slips): `scenario.schema.json` adds `aws` to cloud enum + new `dynamodb` and `messaging` resource types per concepts.md § "Resolved decisions" + new optional `aws_resource_anchors: [string]` top-level field (list of per-API resource types the scenario asserts end-to-end coverage for; required on aws scenarios, ignored on others — feeds S48-T7's coverage audit so coarse-grained scenario keys like `compute` don't trivially claim coverage of multiple AWS resources); `internal/scenario/scenario.go` mirrors schema; `internal/config/config.go::Config` gains `Fakeaws FakeawsConfig`; `internal/cli/runtime.go` + `mockway_client.go` route `cloud: aws` to fakeaws URL; `internal/harness/topology_derive.go::detectCloud` + new `deriveTopologyAWS`; `internal/harness/destroy.go::countOrphans` ignored-roots/-collections list extended for fakeaws's universal bookkeeping tables `operations` + `audit` (S43); `sqs_messages` appended when SQS lands in S46. DynamoDB streams out of scope at v1 — no streams_cursor table; `internal/harness/real_probe.go::probeTargetResourceTypes` adds aws resource types per probe target; `internal/harness/provider_schema.go::ExtractProviderSchema` becomes cloud-aware AND the call site `CommandRuntime.EnsureProviderSchema()` (invoked from `internal/cli/generate_command.go` line ~166; the scaleway-only extractor lives in `internal/cli/runtime.go` line ~118) is restructured so extraction happens **after** `LoadScenario` returns the active cloud (per-scenario lazy extraction) — the current pre-scenario extraction order means there's no `cloud` value at extraction time, so cloud-aware extraction is impossible without this change; **policy plumbing**: `infrafactory.yaml::validation.layers.static.policy_paths` extended with `./policies/aws`. The cloud-aware filter is implemented in `internal/cli/validate_command.go` (which today loads every configured static policy path unconditionally — line ~64) — extend the loader to filter by the active scenario's `cloud` field so each cloud's policy dir is only evaluated for that cloud's scenarios. AND state-policy dispatch in `internal/cli/test_command.go` (which already does per-cloud lookup with a flat fallback) gains an `aws` mapping for AWS-specific resource types so AWS scenarios route to `policies/aws/*.rego` instead of falling through to the flat default; `internal/e2e/helpers.go::StartFakeaws` mirrors `StartFakegcp` (URL, FetchState, LogPath, t.Cleanup); the existing helpers.go config-writer at line ~321 currently hard-codes `fakegcp:` URL and `policy_paths: [common, scaleway, gcp]` for every test setup — extend it to also emit `fakeaws:` URL and add `aws` to `policy_paths` whenever an aws scenario is in scope (test asserts the emitted YAML includes the aws keys when `cloud: aws` is requested); `Makefile` adds `run-fakeaws` (port 8082) and extends `run-mocks`. Every changed file has at least one targeted aws-path test. **No regression on existing clouds**: every modified file in `internal/cli/`, `internal/harness/`, `internal/scenario/`, and `internal/e2e/` also serves mockway+fakegcp paths — `go test ./internal/...` must stay green across all three clouds before this ticket can ship. | P1 | S43-T6, S43-T8 |
| S43-T10 | fakeaws: pre-seed `handlers/regression_test.go` with all 16 standing patterns (13 from fakegcp pass-loop + 3 unique from mockway 14-bug catalogue, after pass-17 deduplication — see concepts.md § "Standing patterns"); ship the audit **infrastructure only**: `handlers/regression_manifest.go` + `handlers/regression_audit_test.go` (two test funcs) + `fakeaws/internal/audit/audit_test.go` (`TestFullCoverageAudit`) reading `fakeaws/coverage_matrix.yaml` + an *empty* `coverage_matrix.yaml` (just the schema header — no entries yet). Wire both into the CI `regression-seed-audit` and `coverage-audit` jobs so they run on every push from this slice onwards. The audit's contract is: **for every entry in `coverage_matrix.yaml`, all five invariants must hold**. An entry is added in the same PR that ships the bundle satisfying it (handler + integration test + working dir + misconfigured dir + updates dir + scenario with anchors). So at end of S43-T10 the matrix is empty and the audit passes vacuously; both IAM and S3 entries are added in S43-T14's PR (which bundles all the IAM and S3 handlers + scenarios + examples + e2e); subsequent phases follow the same rule (one or more matrix entries per phase-exit PR, depending on how many services that phase ships). This sidesteps the staging contradiction: the audit is always-on but only enforces what's claimed-as-landed. | P1 | S43-T1 |
| S43-T11 | infrafactory generator surfaces: `prompts/aws/{phase1_plan_architecture,phase2_generate_hcl,phase3_self_review}.md` (3 cloud-aware prompt files mirroring `prompts/gcp/` and `prompts/scaleway/` — service guidance does NOT live here); `pitfalls/aws.yaml` seeded with ≥8 terraform-provider-aws footgun entries; `policies/aws/{region_restriction,encryption,vpc_required,no_public_db}.rego` (all 4 lands at once); `cloudMockStateRouter` full test matrix: (a) dispatches `cloud: aws` to fakeaws URL, (b) rejects unknown cloud values with a clear error, (c) falls back gracefully when `Config.Fakeaws.URL == ""` (returns a deterministic error rather than panicking — same fallback shape used for empty `Fakegcp.URL`), (d) `reset/snapshot/restore` are per-cloud (an aws scenario's reset only touches fakeaws, not mockway or fakegcp; verified in a 3-mock concurrent test). | P1 | S43-T9 |
| S43-T12 | fakeaws examples + auto-discovery contract — `examples/provider_smoke_test.go` (mirror of `mockway/e2e/provider_smoke_test.go`) walks all three trees `working/`, `misconfigured/`, `updates/` and applies the per-tree assertion (working: apply→plan→destroy clean; misconfigured: apply must fail with the documented AWS error code; updates: apply v1→plan→apply v2→plan→destroy clean). Adding a directory to *any* of the three automatically registers it. Day-1 directories: `working/iam_role`, `working/s3_bucket`, `misconfigured/iam_attachment_missing_role` (S3 has no v1 FK refs — exemption noted in `examples/README.md`), `updates/update_iam_role_description` (mutate the role's `description` field via `aws_iam_role.description` — `display_name` is a fakegcp Service Account field, NOT an AWS IAM Role field; AWS IAM roles expose `Description`, `MaxSessionDuration`, and `AssumeRolePolicyDocument` as mutable scalars, of which `Description` is the simplest to demo), `updates/update_s3_bucket_versioning` (in-place flip enabled→suspended). | P1 | S43-T6, S43-T8, S43-T11 |
| S43-T13 | infrafactory training scenarios `aws-iam.yaml` + `aws-s3.yaml` — declare resource shape, region, acceptance criteria; loaded via `cloud: aws` and fakeaws's URL. Mirror of `gcp-*.yaml`. | P1 | S43-T9, S43-T11 |
| S43-T14 | fakeaws Phase 1 gated e2e: `TestE2E_AWS_IAM` + `TestE2E_AWS_S3` in `infrafactory/internal/e2e/aws_services_test.go`, using `runAWSServiceScenario` (clone of `runGCPServiceScenario`) with identity-preservation hooks (snapshot resource `name`/`arn` from `/mock/state` pre-update, assert stability post-update). **Same PR adds IAM + S3 entries to `fakeaws/coverage_matrix.yaml`** per the per-bundle rule in S43-T10. | P1 | S43-T6, S43-T8, S43-T9, S43-T10, S43-T12, S43-T13 |

### Acceptance criteria

- S43-T1: `go build ./cmd/fakeaws` succeeds. `go test ./...` passes. **Day-1 invariant** ships in the first commit: `.gitleaks.toml` (allowlists `examples/.*\.tf$` only), `.githooks/pre-commit` (executable, runs `gitleaks protect --staged --no-banner` *before* `go test ./...`), `Makefile install-hooks` target (idempotent, sets `core.hooksPath .githooks`), `.github/workflows/ci.yml` with the **named CI jobs** declared and **stub packages** they target: T1 includes empty stub files `fakeaws/handlers/regression_audit_test.go` (one no-op test that always passes) and `fakeaws/internal/audit/audit_test.go` (one no-op test) so `go test ./...` resolves cleanly from day one; T10's PR replaces the no-op bodies with the real audit logic. The CI workflow has these jobs declared from day one (each a required check on `main`): `lint` (`go vet`), `build`, `test` (`go test ./... -race`), `gitleaks` (`gitleaks detect --redact --no-banner --source=.`), `regression-seed-audit` (`go test ./handlers/ -run "TestRegressionSeedAudit"` — passes vacuously against the stub until T10 fills it in), `coverage-audit` (`go test ./internal/audit/ -run TestFullCoverageAudit` — passes vacuously against the stub until T10 fills it in), `coverage` (two-step: `go test -coverprofile=cov.out -covermode=atomic ./handlers/...` then `go tool cover -func=cov.out`; CI parses the `total:` line and fails if < 80%). The stub-package pattern means S43-T1 has NO dependency on S43-T10 — both can land in parallel; once T10 lands, the same workflow jobs immediately start running real assertions. Synthetic-positive: a temp file containing `AKIAIOSFODNN7EXAMPLE` is rejected by both the local hook *and* the CI gitleaks job.
- S43-T2: `awsproto.WriteAWSError(w, ErrInUse)` produces the correct wire shape per protocol. One test per (wire shape × error sentinel) cell: 5 wire shapes (XML, Query-RPC, JSON 1.0, JSON 1.1, JSON-REST) × 4 sentinels (ErrNotFound, ErrInUse, ErrTerminalState, ErrConflict) = **20 cells minimum**. Helpers tested in `awsproto/awsproto_test.go`. **No silent error path**: every distinct domain error reaches at least one handler test that asserts the response body, not just the status code. **ARN-builder fold-in (M39)**: `awsproto/arn.go` ships per-service builders rather than one generic builder, because real AWS ARN formats vary per service (IAM omits region: `arn:aws:iam::<account>:role/<name>`; S3 is bucket-scoped: `arn:aws:s3:::<bucket>`; Route53 is global; EC2 / RDS / EKS / SQS / SecretsManager / DynamoDB embed region + account). Each in-scope service gets `BuildXxxARN(...)` with arguments shaped to its real format; a top-level test asserts each helper's output matches the format documented in the corresponding `arn:aws:<service>` AWS reference. Closes M39.
- S43-T3: Reset/snapshot/restore lifecycle pinned by `TestRepositoryAdminLifecycle` covering SQLite *and* in-process caches (analog of fakegcp's `TestResetClearsDNSChangeCache`). FK constraints enforced (`PRAGMA foreign_keys = ON`, `SetMaxOpenConns(1)`). `resolveSameAccountName` test covers cross-account, wrong-collection, relative-path branches.
- S43-T4: `/mock/state` returns the full SQLite contents as JSON keyed by service. Reset clears all tables and the snapshot baseline. Schema documented inline so topology_derive_aws has a stable target.
- S43-T5/T6: Working IAM CRUD across **every endpoint** (Role, Policy, InstanceProfile, User, AccessKey, AttachRolePolicy, DetachRolePolicy, AddRoleToInstanceProfile). Each endpoint has at least one success-path unit test, one 404 test, and one FK-violation test where applicable.
- S43-T7/T8: Working bucket CRUD across **every endpoint** including versioning, encryption, tagging, policy, public-access-block, ownership-controls. Object endpoints accept PUT, return ETag, but discard the payload (documented inline). Per-endpoint test coverage as above.
- S43-T9: `cloud: aws` scenarios validate against the schema. `StartFakeaws` boots fakeaws on a free port; lifecycle bound to `t.Cleanup`. `detectCloud` returns `aws` for fakeaws-shaped state; `countOrphans` returns 0 after a clean teardown of an aws scenario; `real_probe` finds aws hosts; `provider_schema` extracts `hashicorp/aws` *and* extraction runs lazily per-scenario (test asserts a scaleway scenario followed by an aws scenario in the same process gets each provider schema correctly); state-policy dispatch maps `cloud: aws` to the aws-specific evaluator path (test asserts the dispatch resolves to a stub aws evaluator, not the flat default — the actual rego files land in S43-T11). **Each of the 13 surfaces modified has a targeted aws-path test** — this is what stops codex review pass 27's "wired-but-tested-on-the-other-cloud" failure mode. The "policies/aws/*.rego actually load and evaluate" assertion lives on S43-T11 (which owns the rego files), not here.
- S43-T10: `regression_test.go` exists with 16 named test functions (one per standing pattern), each with a narrative comment header explaining the regression it pins. No bare `t.Skip()` calls — tests for not-yet-landed services call `requireHandlerImplemented(t, "<service-id>")`, a manifest-gated helper that calls `t.Skipf("TODO(slice=...,service=...,pattern=...)")` when the id isn't in `regression_manifest.go::LandedServices`. Manifest schema and audit semantics defined in concepts.md § "Resolved decisions" item 12 (single tracked file `fakeaws/handlers/regression_manifest.go`, exported `LandedServices []string`, top-level service-ids — `iam`, `s3`, `ec2`, etc., single lowercase tokens). The two audits live in `fakeaws/handlers/regression_audit_test.go` (a single file with two functions): (a) `TestRegressionSeedAuditManifestMatchesHandlers` walks `LandedServices` ↔ files in `handlers/` and asserts the bijection holds; (b) `TestRegressionSeedAuditNoVacuousPasses` parses test bodies via `go/ast` and fails if any `func` body contains both `requireHandlerImplemented(...)` and an `assert.` / `require.` call. `go test ./...` stays green at every slice exit because the seed tests skip cleanly until their handler is in the manifest.
- S43-T11: `prompts/aws/phase1_plan_architecture.md`, `phase2_generate_hcl.md`, `phase3_self_review.md` all exist and follow the same template as `prompts/gcp/` (3 phase files, NOT a per-service matrix). `pitfalls/aws.yaml` has at least 8 entries seeded from terraform-provider-aws known footguns. All four `policies/aws/{region_restriction,encryption,vpc_required,no_public_db}.rego` files exist and evaluate against IAM + S3 examples without false positives. **End-to-end policy evaluation test** (depends on S43-T9's dispatch infrastructure): a `cloud: aws` scenario actually routes through state-policy dispatch and the aws rego files fire — this assertion lives here, not in S43-T9, because the rego files don't exist before this ticket. **`cloudMockStateRouter` full test matrix**: (a) `cloud: aws` dispatches to `Config.Fakeaws.URL`; (b) unknown cloud values rejected with a clear error; (c) graceful fallback when `Fakeaws.URL == ""` (deterministic error, no panic — mirrors the existing `Fakegcp.URL == ""` fallback); (d) per-cloud reset/snapshot/restore — a 3-mock concurrent test boots mockway + fakegcp + fakeaws on different ports, runs an aws scenario's reset, and asserts only fakeaws state was cleared (mockway + fakegcp untouched).
- S43-T12: `examples/provider_smoke_test.go` (mirror of `mockway/e2e/provider_smoke_test.go`) walks all three trees — `examples/working/`, `examples/misconfigured/`, `examples/updates/` — and applies the per-tree contract: working = `apply → plan -detailed-exitcode → destroy` clean; misconfigured = apply must fail with the documented AWS error code (asserted by grep on the tofu output); updates = `apply v1 → plan no-op → apply v2 → plan no-op → destroy` clean. Day-1 directories `working/iam_role`, `working/s3_bucket`, `misconfigured/iam_attachment_missing_role`, `updates/update_iam_role_description`, `updates/update_s3_bucket_versioning` all pass the gate. Adding a directory to *any* of the three trees in any later phase automatically registers it — no per-example test ticket in S44-S47.
- S43-T13: `aws-iam.yaml` + `aws-s3.yaml` validate against schema, are loaded by `cloud: aws`, exercise every IAM/S3 resource type that infrafactory's resource matrix names. Coverage check: every resource type in `scenario.schema.json::resources` that's relevant to AWS at v1 has at least one scenario referencing it across the full S43-S47 scenario set (foundational gate; per-phase scenarios extend coverage).
- S43-T14: `INFRAFACTORY_ENABLE_E2E=1 go test ./internal/e2e -run TestE2E_AWS_IAM` and `TestE2E_AWS_S3` both green. Identity-preservation assertion: `name` and `arn` of every primary resource is byte-identical pre/post-update; failure mode "destroy + recreate" trips the test.

### Coverage requirements (load-bearing — applies across all phases)

These three coverage rules are non-negotiable per the user. They apply phase-wide, not per-ticket; the per-phase exit gates above enforce them at slice boundaries.

1. **Every endpoint has a unit test**: every handler function exposed in `handlers/<service>.go` has at least one corresponding test in the shared `handlers/handlers_test.go` (mirroring fakegcp's pattern — one shared integration test file, NOT per-service test files; named `TestXxx<Service>...` so the audit-test regex can find them). Each test asserts the success path, 404, and (where applicable) FK-violation, terminal-state, and cascade behaviour. CI fails if a new handler ships without a matching test (verified by S48-T7's `TestFullCoverageAudit` plus the **aggregate** `handlers/...` coverage gate ≥ 80% lines on the `total:` line — see concepts.md § "Coverage targets and CI" for the exact two-step command).
2. **Every example is wired into the test suite**: `examples/working/`, `examples/misconfigured/`, `examples/updates/` are auto-discovered by `examples/provider_smoke_test.go`. Adding a directory automatically registers it for the smoke gate. CI runs the discovery + smoke loop on every push. The auto-discovery pattern is implemented in S43-T12 and reused by every later phase — no hand-curated per-service smoke ticket.
3. **Every infrafactory resource type has at least one aws scenario**: by the end of S47, every entry in `scenario.schema.json::properties.resources` that has an aws mapping is referenced by at least one `scenarios/training/aws-*.yaml`. This rule is *enforced* by `TestFullCoverageAudit`, whose **infrastructure** ships in S43-T10 (audit Go test, coverage_matrix.yaml schema, CI job that runs it on every push) so the gate is always-on from Phase 1 onwards. **The audit only enforces invariants on entries already present in `coverage_matrix.yaml`**: an entry is added in the same PR that ships the bundle satisfying it (handler + integration test + working/misconfigured/updates dirs + scenario with `aws_resource_anchors`). The matrix starts empty in S43-T10 and grows one entry per service-bundle PR (IAM in S43-T14, S3 in S43-T14, EC2 services in S44-T12, etc.). S48-T7 is the **completeness checkpoint** that asserts (i) every entry in the matrix passes, AND (ii) the matrix's set of entries covers every aws-relevant resource type promised by concepts.md § "Resource coverage matrix" (the second check is what catches a missing entry; the first catches an entry whose bundle drifted). New aws_resource_types added in any phase must ship the bundle and the entry in the same PR.

---

## Slice 44: Networking + compute (EC2)

Land VPCs, subnets, security groups, route tables, internet gateways, EIPs, NAT gateways, and finally instances. EC2 is XL scope but well-defined; FK chains are the load-bearing complexity.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S44-T0 | infrafactory: `pitfalls/aws.yaml` extended with EC2-specific rules (security-group inline vs `aws_security_group_rule`, route-table FK shape, subnet/AZ pairing, IGW-attachment vs IGW-create ordering). No new prompt files — concepts.md § "Resolved decisions" item 11 retracted the per-service prompt-matrix design; service guidance lives in pitfalls only. | P1 | S43-T11 |
| S44-T1 | Phase 2 design note: EC2 query-RPC wire format vs SDK expectations; FK chain VPC→Subnet→Instance and SG→Instance | P1 | S43-T2 |
| S44-T2 | fakeaws: `awsproto` query-RPC parser + XML response writer for EC2 (Action=Foo / Version=YYYY-MM-DD) | P1 | S43-T2 |
| S44-T3 | fakeaws: repository support for ec2_vpcs + ec2_subnets + ec2_security_groups + ec2_route_tables + ec2_internet_gateways + ec2_eips with FK | P1 | S43-T6 |
| S44-T4 | fakeaws: `handlers/ec2_network.go` (VPC, Subnet, InternetGateway, RouteTable, Route, EIP, NAT gateway) | P1 | S44-T3 |
| S44-T5 | fakeaws: `handlers/ec2_security.go` (SecurityGroup + ingress/egress rules, AuthorizeSecurityGroupIngress / Revoke...) | P1 | S44-T4 |
| S44-T6 | fakeaws: repository support for ec2_instances + ec2_key_pairs + ec2_amis (read-only fixture set) | P1 | S44-T4 |
| S44-T7 | fakeaws: `handlers/ec2_instance.go` (Instance create/describe/modify/terminate, KeyPair, AMI fixture data) | P1 | S44-T6 |
| S44-T8 | fakeaws: handlers_test for EC2 (CRUD across all resources, FK validation, cascade, instance state transitions) | P1 | S44-T7 |
| S44-T9 | fakeaws: regression coverage for instance create/modify/terminate + ENI attachment + EIP lifecycle | P1 | S44-T7, S44-T8 |
| S44-T10 | infrafactory: `scenarios/training/aws-vpc-network.yaml` + `aws-instance.yaml` + loader update | P1 | S44-T9, S43-T9 |
| S44-T11 | fakeaws: `examples/working/basic_instance` + `working/vpc_network` + `misconfigured/instance_missing_subnet` + `updates/update_security_group_rules` | P1 | S44-T9 |
| S44-T12 | fakeaws Phase 2: gated `TestE2E_AWS_VPC` + `TestE2E_AWS_Instance` + `TestE2E_AWS_SecurityGroup` in infrafactory. Same PR adds matching coverage_matrix.yaml entries per the per-bundle rule. | P1 | S44-T9, S44-T10, S44-T11 |

### Acceptance criteria

- S44-T1 design note: pinned in `fakeaws/PLAN.md` covering query-RPC body parsing, XML response writing, and the four primary FK chains (VPC→Subnet, VPC→InternetGateway, VPC→SecurityGroup, Subnet→Instance).
- All ten phase exit gates from "Quality bar" green for S44.
- `aws-vpc-network.yaml` and `aws-instance.yaml` scenarios validate against the schema.
- `examples/working/basic_instance` fully exercises VPC → subnet → SG → instance creation through the AWS provider.
- `examples/misconfigured/instance_missing_subnet` fails apply with the correct AWS error code (the provider must surface fakeaws's 404 as a Terraform error containing `InvalidSubnetID.NotFound`).
- `examples/updates/update_security_group_rules` flips ingress rules via `v2.tfvars` without recreating the SG.

---

## Slice 45: Stateful data (RDS + DynamoDB)

RDS shares the query-RPC protocol with EC2, so the parser from S44 carries through. DynamoDB is its own JSON dialect with `x-amz-target` routing.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S45-T0 | infrafactory: `pitfalls/aws.yaml` extended with RDS rules (`aws_db_subnet_group` requirement, `aws_db_instance` parameter-group attachment, deletion-protection footgun) and DynamoDB rules (PK/SK shape, attribute-name uniqueness). No new prompt files — see concepts.md § "Resolved decisions" item 11. | P1 | S43-T11 |
| S45-T1 | Phase 3 design note: RDS query-RPC + DynamoDB JSON; collapsed state machines; DBSubnetGroup ↔ EC2 subnet FK chain (RDS instance must reference a DBSubnetGroup; DBSubnetGroup must reference EC2 subnets in the same VPC — this is the AWS shape, not Scaleway's "private network" concept) | P1 | S43-T2 |
| S45-T2 | fakeaws: repository support for rds_instances + rds_clusters + rds_subnet_groups + rds_parameter_groups (FK on subnet group → EC2 subnets) | P1 | S43-T6, S44-T2, S44-T3 |
| S45-T3 | fakeaws: `handlers/rds.go` (DBInstance + DBCluster + DBSubnetGroup + DBParameterGroup + ClusterParameterGroup) | P1 | S45-T2 |
| S45-T4 | fakeaws: repository support for dynamodb_tables + dynamodb_items (item PK index) | P1 | S43-T6 |
| S45-T5 | fakeaws: `handlers/dynamodb.go` (Table CRUD + minimal item PutItem/GetItem/UpdateItem/DeleteItem/Query/Scan) | P1 | S45-T4 |
| S45-T6 | fakeaws: handlers_test for RDS + DynamoDB (CRUD, FK, cascade, basic item ops) | P1 | S45-T3, S45-T5 |
| S45-T7 | fakeaws: regression coverage for RDS read-replica chain + DynamoDB table-state transitions (note: GSI/LSI is OUT OF SCOPE at v1 per concepts.md § "Resource coverage matrix § DynamoDB", which promises only `basic; no transactions/streams at v1`. If a scenario actually needs GSI later, it gets its own ticket.) | P1 | S45-T5, S45-T6 |
| S45-T8 | infrafactory: `scenarios/training/aws-rds.yaml` + `aws-dynamodb.yaml` + loader update | P1 | S45-T7, S43-T9 |
| S45-T9 | fakeaws: auto-discovered `examples/working/rds_instance` + `working/dynamodb_table` + matching `misconfigured` + `updates` dirs (registered automatically by the `examples/provider_smoke_test.go` from S43-T12) | P1 | S45-T7 |
| S45-T10 | fakeaws Phase 3: gated `TestE2E_AWS_RDS` + `TestE2E_AWS_DynamoDB` in infrafactory. Same PR adds matching coverage_matrix.yaml entries. | P1 | S45-T7, S45-T8, S45-T9 |

### Acceptance criteria

- All ten phase exit gates green.
- RDS state machine: instance lifecycle states (creating → available → modifying → deleting) collapsed to "always available" except where the AWS provider expects to wait — pin the exact subset in tests.
- DynamoDB Query/Scan returns paginated, attribute-projected responses matching the SDK's expectations on `Count`, `ScannedCount`, `LastEvaluatedKey`. (Basic ops only at v1 — no GSI/LSI/Streams; that's out-of-scope per the v1 surface.)
- RDS read-replica via `CreateDBInstance` with `SourceDBInstanceIdentifier` pinned by a regression test (mirror of fakegcp's parent-FK rebinding tests).
- Per-resource coverage rule: every endpoint in `handlers/rds.go` and `handlers/dynamodb.go` has at least one unit test asserting success path + 404 + FK-violation (where applicable) + cascade (where applicable). CI fails if a new handler ships without a matching test.

---

## Slice 46: Containers + queues (EKS + SQS)

EKS is JSON-REST (modern flavour). SQS is JSON 1.0 with `x-amz-target`.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S46-T0 | infrafactory: `pitfalls/aws.yaml` extended with EKS rules (cluster→nodegroup ordering, IAM-role `eks.amazonaws.com` trust policy, subnet IPv4-block requirement) and SQS rules (DLQ `RedrivePolicy` JSON shape, FIFO queue suffix). No new prompt files — see concepts.md § "Resolved decisions" item 11. | P1 | S43-T11 |
| S46-T1 | Phase 4 design note: EKS JSON-REST and SQS x-amz-target wire formats; cluster/nodegroup state-machine simplification | P1 | S43-T2 |
| S46-T2 | fakeaws: repository support for eks_clusters + eks_node_groups + eks_addons with FK cascade + IAM/EC2 cross-resource validation | P1 | S43-T6, S43-T8, S44-T2, S44-T3, S44-T5 |
| S46-T3 | fakeaws: `handlers/eks.go` (Cluster + NodeGroup + AddOn; FK against IAM roles + EC2 subnets + security groups) | P1 | S46-T2 |
| S46-T4 | fakeaws: repository support for sqs_queues + sqs_messages with at-least-once visibility-timeout collapsed | P1 | S43-T6 |
| S46-T5 | fakeaws: `handlers/sqs.go` (Queue + minimal SendMessage / ReceiveMessage / DeleteMessage) | P1 | S46-T4 |
| S46-T6 | fakeaws: handlers_test for EKS + SQS (CRUD, FK, cascade, message lifecycle) | P1 | S46-T3, S46-T5 |
| S46-T7 | fakeaws: regression coverage for EKS cluster→nodegroup→addon dependencies + SQS DLQ + visibility-timeout edge cases | P1 | S46-T5, S46-T6 |
| S46-T8 | infrafactory: `scenarios/training/aws-eks.yaml` + `aws-sqs.yaml` + loader update | P1 | S46-T7, S43-T9 |
| S46-T9 | fakeaws: `examples/working/eks_cluster` + `working/sqs_queue` + matching `misconfigured` + `updates` dirs | P1 | S46-T7 |
| S46-T10 | fakeaws Phase 4: gated `TestE2E_AWS_EKS` + `TestE2E_AWS_SQS` in infrafactory. Same PR adds matching coverage_matrix.yaml entries (incl. countOrphans `sqs_messages` extension). | P1 | S46-T7, S46-T8, S46-T9 |

### Acceptance criteria

- All ten phase exit gates green.
- EKS cluster create FK-validates the IAM role ARN (real AWS API surface), the subnet **IDs** (the EKS API and terraform-provider-aws both pass subnet IDs, NOT subnet ARNs), and the security group IDs in one transaction.
- SQS visibility-timeout collapsed to in-memory tracking (no real timeout enforcement); the test pins the response shape, not the actual timing.
- DLQ semantics: a queue whose `RedrivePolicy` references a non-existent DLQ fails create with the right AWS error code.

---

## Slice 47: DNS + secrets (Route53 + Secrets Manager)

Mirrors fakegcp's DNS and Secret Manager almost line-for-line — same atomic changes API for Route53, same DESTROYED-is-terminal contract for Secrets Manager.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S47-T0 | infrafactory: `pitfalls/aws.yaml` extended with Route53 rules (atomic ChangeResourceRecordSets shape, hosted-zone NS-record auto-creation) and Secrets Manager rules (`recovery_window_in_days` for delete, `force_overwrite_replica_secret` semantics). No new prompt files — see concepts.md § "Resolved decisions" item 11. | P1 | S43-T11 |
| S47-T1 | Phase 5 design note: Route53 XML wire format + Secrets Manager JSON-1.1 in awsproto, validate against terraform-provider-aws expectations | P1 | S43-T2 |
| S47-T2 | fakeaws: repository support for route53_hosted_zones + record_sets, FK + non-empty-zone delete refusal | P1 | S43-T6 |
| S47-T3 | fakeaws: `handlers/route53.go` (HostedZone + ResourceRecordSet via `ChangeResourceRecordSets`, transactional) | P1 | S47-T2 |
| S47-T4 | fakeaws: repository support for secretsmanager_secrets + versions, with state column + FK from versions → secrets | P1 | S43-T6 |
| S47-T5 | fakeaws: `handlers/secretsmanager.go` (Secret + Version state machine; DESTROYED terminal; tagging) | P1 | S47-T4 |
| S47-T6 | fakeaws: handlers_test for Route53 + Secrets Manager (CRUD, FK, cascade, change-id scoping) | P1 | S47-T3, S47-T5 |
| S47-T7 | fakeaws: regression coverage for Route53 changes API atomicity + Secrets Manager terminal-state contract (`DeleteSecret` with `recovery_window_in_days` → scheduled deletion; `RestoreSecret` reverses scheduled deletion when called within the recovery window; `RestoreSecret` after the secret has fully destroyed returns 409 `InvalidRequestException`. The legacy `:destroy/:enable` shorthand from the fakegcp Pub/Sub world is not the AWS API surface — `RestoreSecret` is.) | P1 | S47-T5, S47-T6 |
| S47-T8 | infrafactory: `scenarios/training/aws-route53.yaml` + `aws-secrets-manager.yaml` + loader | P1 | S47-T7, S43-T9 |
| S47-T9 | fakeaws: `examples/working/route53` + `working/secrets_manager` + matching `misconfigured` + `updates` dirs (v1/v2 tfvars) | P1 | S47-T7 |
| S47-T10 | fakeaws Phase 5: gated `TestE2E_AWS_Route53` + `TestE2E_AWS_SecretsManager` in infrafactory/internal/e2e. Same PR adds matching coverage_matrix.yaml entries. | P1 | S47-T7, S47-T8, S47-T9 |

### Acceptance criteria

- All ten phase exit gates green.
- Route53 `ChangeResourceRecordSets` is transactional: a batch with one bad change rejects the whole batch with no partial state. Mirror of fakegcp's pass-1 DNS atomicity fix.
- `GetChange` poll endpoint scoped by (account, hosted-zone, change-id) tuple — change ids from one zone don't leak across zones. Mirror of fakegcp's pass-17 `(project, zone, id)` keying.
- Secrets Manager terminal state: `DeleteSecret` schedules deletion with `recovery_window_in_days`; `RestoreSecret` succeeds within the window; `RestoreSecret` *after* the recovery window has fully elapsed (the secret is destroyed) returns 409 with `InvalidRequestException` — the AWS-spec terminal-state code. No fakegcp `:destroy/:enable` shorthand anywhere; tests assert against real AWS API names.
- `examples/working/route53` exercises the changes API with both A and AAAA records; updates example flips TTL via `v1.tfvars` → `v2.tfvars`.

---

## Slice 48: Polish + codex review iteration loop

Not a feature slice. This exists to run the same 33-pass-style review loop that landed fakegcp at quality. Until two consecutive `NOTHING_TO_IMPROVE` returns, fakeaws v1.0 is not shippable. Three deliverables that earlier drafts placed here have moved to S43 (Day-1) per codex review pass 1: `regression_test.go` seed (now S43-T10), `countOrphans` extension (now part of S43-T9), and cross-resource FK validators (now part of S43-T3). What remains is genuine polish — codex iteration, doc completion, and the secret-scanning audit.

### Tickets

| id | title | priority | deps |
|---|---|---|---|
| S48-T1 | fakeaws Phase 6 / polish: incremental regression top-up across S44–S47 — every codex `BLOCKING` and accepted `SUGGEST` finding from each phase's review pins a new test in `handlers/regression_test.go`, building on the Day-1 seed from S43-T10. Deps on phase-exit e2e tickets (where the codex review for that phase actually runs and surfaces findings), not on the pre-exit regression tickets | P1 | S44-T12, S45-T10, S46-T10, S47-T10 |
| S48-T2 | (vacated — `countOrphans` extension landed in S43-T9; this row preserved for ID stability and explicitly marked WONTFIX/MOVED rather than re-used so historical references in commit messages still resolve) | — | — |
| S48-T3 | (vacated — cross-resource FK validators landed in S43-T3; preserved as MOVED) | — | — |
| S48-T4 | fakeaws Phase 6 / polish: codex review iteration loop until 2 consecutive `NOTHING_TO_IMPROVE` passes. Each pass archived under `fakeaws/docs/review-passes/passN.md` (prompt + response). Restart count on any `BLOCKING:` finding. Commit message format: `Codex pass N: <NOTHING_TO_IMPROVE \| applied finding summary>`. Budget 20–35 passes. | P1 | S47-T10 |
| S48-T5 | fakeaws Phase 6 / polish: README + AGENTS + PLAN + BACKLOG docs in fakeaws repo. AGENTS.md ports the mockway 14-bug catalogue (concepts.md § "Anti-patterns: the mockway 14-bug catalogue") with AWS-flavoured examples; per-protocol wire-format reference; handler-registration walkthrough (`func (app *Application) RegisterRoutes(r chi.Router)`); proxy-capture workflow guidance for new endpoint discovery; `--echo` mode usage. | P1 | S43-T1 |
| S48-T6 | fakeaws Phase 6 / polish: working/ + misconfigured/ + updates/ coverage gap-fill across all in-scope services (the auto-discovery from S43-T12 keeps registering, but the directories themselves still need writing for any service that landed without one). Final audit confirms every service has at least one of each, with documented exemptions for services where misconfigured doesn't apply. | P1 | S43-T14, S44-T12, S45-T10, S46-T10, S47-T10 |
| S48-T7 | fakeaws Phase 6 / polish: machine-verifiable full coverage **completeness checkpoint** — the audit infrastructure (`fakeaws/internal/audit/audit_test.go::TestFullCoverageAudit` + `fakeaws/coverage_matrix.yaml` schema + CI job) shipped in S43-T10 and has been gating every push since Phase 1; this ticket *certifies* that at the end of S47 every entry in `coverage_matrix.yaml` passes (no exemption-without-reason; every aws_resource_type has integration test + working/misconfigured/updates dirs or documented exemption + ≥1 aws-scenario in aws_resource_anchors). Plus end-of-phase aggregate `handlers/...` coverage gate ≥80% (aggregate, not per-package — see concepts.md § "Coverage targets and CI"). The work here is rolling up audit clean-runs into the v1 release sign-off, not introducing the audit. | P1 | S48-T6 |
| S48-T8 | fakeaws Phase 6 / polish: secret-scanning sweep — re-run `gitleaks detect --no-banner --source=.` across full repo history, audit `.gitleaks.toml` for allowlist drift, synthetic-positive injection test (`AKIAIOSFODNN7EXAMPLE` rejected by both local hook and CI), confirm `make install-hooks` is idempotent and documented in README. (Initial four-file Day-1 invariant landed in S43-T1.) | P1 | S48-T7 |

### Acceptance criteria

- S48-T1: every accepted finding from S44–S47 codex passes is pinned by a named test in `regression_test.go`. The Day-1 seed (S43-T10) is no longer empty — by end of S47 it has at least 16 + N tests where N = total accepted codex findings across phases.
- S48-T4: 2 consecutive `NOTHING_TO_IMPROVE` codex passes documented in commit messages, with the prompts + responses archived under `fakeaws/docs/review-passes/passN.md`. Pass numbers are monotonic. A `BLOCKING` finding restarts the count to 0. **Cross-pollination fold-in (M40)**: at every phase exit (S43-T14, S44-T12, S45-T10, S46-T10, S47-T10), the codex pass for that phase explicitly enumerates which findings are AWS-specific vs cross-cloud. For each cross-cloud finding, a follow-up `M<n>` ticket is opened against mockway and/or fakegcp before the next phase begins (filing != closing — they may legitimately stay todo). Closes M40.
- S48-T5: README, AGENTS, PLAN, BACKLOG all populated. AGENTS.md is the "fresh agent" entry point and includes: (1) the mockway 14-bug catalogue ported to AWS examples, (2) per-protocol wire-format reference (XML / Query-RPC / JSON 1.0 / JSON 1.1 / JSON-REST), (3) handler-registration walkthrough, (4) proxy-capture workflow for discovering new endpoint requirements, (5) `--echo` mode usage. PLAN.md tracks landed phases. BACKLOG.md is the open-work tracker.
- S48-T6: every service in scope has at least one `examples/working/<svc>` that passes the auto-discovered smoke gate, AND at least one `examples/updates/<svc>` (v1.tfvars → v2.tfvars) that passes the updates contract, AND at least one `examples/misconfigured/<svc>` UNLESS the service has no v1 FK refs (in which case a documented exemption sits in `examples/README.md` naming the service and explaining why misconfigured doesn't apply). All three contracts are auto-discovery-validated by S43-T12's smoke test — S48-T6 is the gap-fill of any service that landed without a directory in one or more trees, plus the audit confirming the coverage rule holds.
- S48-T7: full coverage audit is **machine-verifiable** — implemented as a Go test `TestFullCoverageAudit` in `fakeaws/internal/audit/audit_test.go`. The audit's source of truth is *not* the prose matrix in concepts.md (which is human-friendly but not machine-readable — bullets group multiple types). Instead, **S43-T10** ships a tracked `fakeaws/coverage_matrix.yaml` (S43-T1 ships only the empty CI placeholder job that runs against it) whose schema is each entry has the keys: `aws_service` (e.g. `iam`, `s3`, `ec2`), `aws_resource_type` (per-API resource id, e.g. `aws_iam_role`, `aws_route_table`, `aws_eks_cluster`), `scenario_resource_type` (cloud-neutral scenario.schema.json `resources.<key>` it maps to — must be one of the actual keys in `infrafactory/scenario.schema.json::properties.resources.properties`: `compute`, `networking`, `database`, `kubernetes`, `iam`, `registry`, `redis`, `storage`, `pubsub`, `dns`, `cloud_run`, `secret_manager`, plus the new `dynamodb` and `messaging` keys S43-T9 adds; or `null` for resources not exposed to the generator like AMI fixtures), `working_exempt` (bool), `working_exempt_reason` (string, required iff exempt), `misconfigured_exempt` + `misconfigured_exempt_reason`, `updates_exempt` + `updates_exempt_reason`, `working_dir_name` / `misconfigured_dir_name` / `updates_dir_name` (per-tree directory names under each of the three example trees — they differ in practice: working might be `iam_role`, misconfigured `iam_attachment_missing_role`, updates `update_iam_role_description`. Each defaults to `aws_resource_type` minus the `aws_` prefix when the directory matches the resource verbatim; when it diverges (most cases for misconfigured/updates), the matrix entry names the actual directory. Each can be omitted if the corresponding `*_exempt: true` is set), and `integration_test_func_name` (a regex that must match at least one `Test...` function in `handlers/handlers_test.go`, defaults to `^Test.*<aws_resource_type-camel-cased>.*` — e.g. `^Test.*IamRole.*`). The Go test loads `coverage_matrix.yaml` and asserts five invariants per entry: (a) `handlers/handlers_test.go` (the SHARED integration test file mirroring fakegcp's pattern, NOT a per-service test file) contains at least one `func TestXxx` whose name matches `integration_test_func_name`; (b) `examples/working/<working_dir_name>` exists OR `working_exempt: true` with a non-empty `working_exempt_reason`; (c) `examples/misconfigured/<misconfigured_dir_name>` exists OR `misconfigured_exempt: true` with reason; (d) `examples/updates/<updates_dir_name>` exists OR `updates_exempt: true` with reason; (e) for entries with non-null `scenario_resource_type`, at least one `infrafactory/scenarios/training/aws-*.yaml` has that key under `resources:` AND the matrix entry's `aws_resource_type` is named in the scenario's `aws_resource_anchors:` list (a small new optional field added to `scenario.schema.json` in S43-T9 — a list of strings, each a per-API resource type the scenario is asserting end-to-end coverage for). Without `aws_resource_anchors`, multiple distinct AWS resources collapsing onto a coarse-grained scenario key like `compute` would all trivially "pass" by sharing one scenario. The anchor list is optional on non-aws scenarios; for aws scenarios the audit requires it to enumerate the specific aws_resource_types the scenario covers, so a single `aws-vpc-network.yaml` doesn't claim coverage of every entry that maps to `compute`. New aws scenarios must populate `aws_resource_anchors` in the same PR that adds them. Plus the **aggregate** coverage gate (not per-package): `go test -coverprofile=cov.out -covermode=atomic ./handlers/...` followed by `go tool cover -func=cov.out` reports ≥ 80% on the `total:` line; CI fails if any of these regress. Auto-discovery alone is not enough because it only sees directories that exist — this test sees directories that *should* exist. The matrix file is a tracked design artifact: any new aws_resource_type added to handlers must come with a matrix entry in the same PR.
- S48-T8: `gitleaks detect --no-banner --source=.` across full history returns clean. `.gitleaks.toml` allowlist diff vs fakegcp is intentional (any drift documented inline). Synthetic-positive injection test: a temp file containing `AKIAIOSFODNN7EXAMPLE` is rejected by both the local hook *and* the CI `gitleaks` step. `make install-hooks` is idempotent (re-running is a no-op) and documented as a quickstart step in README. (Initial four-file Day-1 invariant landed in S43-T1, not here.)

---

## Cross-pollination back to mockway and fakegcp

Findings from S48 that reveal a class of bug shared by the older mocks must land back in mockway/fakegcp before the relevant fakeaws phase exits. Concrete instances likely:

- New cross-resource FK validators discovered during the EC2 phase translate to mockway's VPC↔private-network FK chain.
- AWS state-machine refinements (RDS modifying-state) may surface gaps in mockway's RDB read-replica handling.
- Wire-format error-shape consistency tests apply to all three mocks.

Track cross-pollination tickets as `M<n>` (Maintenance) entries in `BACKLOG.md` rather than re-opening fakegcp/mockway slices.

---

## Open questions

Canonical list lives in `fakeaws/concepts.md` § "Open questions". Resolutions land there first; this plan picks up the answer when the resolution affects a slice's acceptance criteria.
