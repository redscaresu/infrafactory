# Slice 52 — Audit infrastructure parity: mockway + fakegcp

Status: planned (2026-05-23)
Owner: TBD

## Motivation

Only `fakeaws` currently has the per-bundle audit infrastructure that
enforces "you can't add a handler file without also adding the test,
example, scenario anchor, and `LandedServices` entry." `mockway` (287
tests, 17 handler files) and `fakegcp` (134 tests, 21 handler files)
have the same risk surface but no audit gate — they rely entirely on
code review to catch partial bundles.

The fakeaws audit consists of four moving parts:

1. **`coverage_matrix.yaml`** — one entry per service-resource pair,
   listing `service`, `resource_type`, `integration_test_pattern`,
   `working_example_dir`, `misconfigured_example_dir`, `scenario_anchor`,
   and optional `*_exempt` fields with documented reasons.
2. **`internal/audit/audit_test.go::TestFullCoverageAudit`** — walks the
   matrix and asserts every entry has a matching integration test, the
   declared example dirs exist (or carry an exemption), and the scenario
   anchor exists.
3. **`handlers/regression_manifest.go::LandedServices`** — flat list of
   service ids whose handler set is fully landed; flipped in the same PR
   that ships the handler.
4. **`handlers/regression_audit_test.go`** — two tests:
   - `TestRegressionSeedAuditManifestMatchesHandlers`: every id in
     LandedServices has ≥1 satisfying file; every service prefix in
     handlers/ has a manifest entry.
   - `TestRegressionSeedAuditNoVacuousPasses`: no test function body
     contains both `requireHandlerImplemented(...)` AND `assert.*`/`require.*`/
     `t.Error*`/`t.Fail*` — that combination means a stub left assertions
     after the service landed.

Plus the runtime gate: `requireHandlerImplemented(t, service, slice, pattern)`
called at the top of regression test functions. It calls `t.Skipf` with a
structured TODO marker when the service isn't in LandedServices yet, so
the same standing-pattern regression test file can be pre-seeded for a
not-yet-landed service without producing spurious failures.

## S52-T1: mockway

Greenfield retrofit. Mockway predates this convention.

### Steps

1. **Audit current handler files** against the prefix-matching contract.
   mockway's files: `admin.go`, `block.go`, `domain.go`, `handlers.go`,
   `iam.go`, `instance.go`, `ipam.go`, `k8s.go`, `lb.go`, `lb_acl.go`,
   `marketplace.go`, `rdb.go`, `redis.go`, `registry.go`,
   `unimplemented.go`, `vpc.go`. All service files use the
   `<service>[_<subresource>].go` convention already — no renames needed.
2. Define `knownNonServiceFiles = {admin.go, handlers.go, unimplemented.go}`.
3. Build `LandedServices = [block, domain, iam, instance, ipam, k8s, lb,
   marketplace, rdb, redis, registry, vpc]`.
4. Inventory existing `handlers/handlers_test.go` (currently a 287-test
   monolith — see follow-up below). Identify which tests are
   "standing-pattern regression" candidates vs per-feature.
5. Build `coverage_matrix.yaml`: one entry per service/resource pair.
   Initial seed from the README's "API Coverage" table.
6. Ship `internal/audit/audit_test.go::TestFullCoverageAudit` reading
   the matrix. Mirror fakeaws's implementation.
7. Ship `handlers/regression_manifest.go` + `regression_audit_test.go`.
   Mirror fakeaws byte-for-byte except for the `LandedServices` body.

**Follow-up worth scoping separately**: mockway's `handlers_test.go` is a
single 287-test file. Splitting per service would be a hygiene win and
make the prefix-matching audit more meaningful.

## S52-T2: fakegcp

Same scope as T1, but with a structural twist. Two service handlers live
under non-conformant filenames:

- `handlers/network.go` — actually `compute_network` (CreateNetwork /
  CreateSubnetwork / etc).
- `handlers/router.go` — actually `compute_router`.

Two options:

- **(A) Rename**: `handlers/network.go` → `handlers/compute_network.go`,
  `handlers/router.go` → `handlers/compute_router.go`. Cleanest fit. Costs
  git blame on those files.
- **(B) Alias map**: extend `serviceFilePrefixes()` in
  `regression_audit_test.go` with an explicit alias map that says "the
  `network` prefix maps to the `compute` service id." Preserves history,
  costs an opaque indirection in the audit.

Recommendation: ship (B). The aliasing is one declarative line; the
audit's purpose is to catch "you forgot LandedServices on a new
prefix," and the alias map makes that catch identical to (A) without
the blame cost.

`LandedServices = [cloudrun, compute, container, dns, iam, loadbalancer,
pubsub, secretmanager, sql, storage]`. Note the absence of `network` and
`router` — those are aliased into `compute`.

## Exit criteria (S52)

- `go test ./handlers/... -run "TestRegressionSeedAudit"` green in both
  repos.
- `go test ./internal/audit/...` green in both repos.
- Adding a new handler file with a brand-new prefix WITHOUT updating
  LandedServices makes the audit fail (verified by adding+removing a
  dummy file as part of the PR review).
- CI workflow gates on the audit (already wired via `go test ./...`).

## Out of scope

- `requireHandlerImplemented` retrofitting on existing tests. mockway and
  fakegcp tests are all on landed services today; the helper is only
  useful when standing-pattern regression tests get pre-seeded for a
  not-yet-landed service. Ship the helper as part of T1/T2 but defer its
  use to the next time a service is added.

## Estimated effort

- S52-T1 (mockway): 4-6h. Bulk is enumerating the coverage_matrix entries
  from the README's coverage table.
- S52-T2 (fakegcp): 2-3h. Smaller surface; the alias-map approach
  avoids the file-rename detour.
