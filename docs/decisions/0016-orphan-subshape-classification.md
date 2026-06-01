# ADR-0016: Orphan-check sub-shape classification

## Status
Accepted

## Context
[ADR-0015](0015-classifier-routing.md) introduces classifier-routed
failure handling. This ADR specifies the orphan-check classifier
sub-shapes, since "leftover resources after destroy" is not a single
fix-shape — five distinct sub-shapes are observed across sweeps and
each routes to a different fix surface.

| # | Sub-shape | Example | Fix surface |
|---|---|---|---|
| 1 | LLM soft-delete | `aws.kms.keys` keeps deleted-marked entries | Update LLM prompt + scenario validation |
| 2 | Provider soft-delete | `google_kms_crypto_key` cannot be hard-deleted by Terraform | Mock handler: ignore expected-leftovers |
| 3 | Parent cascade gap | Mock keeps child after parent destroy | Mock handler: cascade-on-delete |
| 4 | Async eventual-consistency | Resource still listed for ~30s after delete | Orphan-checker: retry with backoff |
| 5 | Genuine bug | Resource never deleted | Mock handler: implement DELETE |

Sub-shape table is seeded with 6 entries (`aws.kms.keys`,
`gcp.kms.crypto_keys`, `gcp.sql.databases`, `aws.iam.roles`,
`scaleway.lb.backends`, `gcp.storage.objects`) covering the cases seen
in 2026-05-30 and 2026-06-01 sweeps. Lookup is by `(cloud, resource)`.

## Decision
`generator.ClassifyOrphans(mockStateJSON, cloud, scenario, timestamp)`
returns an `OrphanRouting` that names the sub-shape, the fix surface,
and an optional `AppendMockGap` target. Unknown
`(cloud, resource)` pairs fall through to the existing pitfall path
(conservative default; rule-of-three before adding to the table per
`feedback_orphan_check_extractor.md`).

The orphan-checker hook fires in `internal/cli/run_command.go`
between `IsMockActionable` and `DetectPolicyConflict`. Order matters:
mock-actionable signals (501, OAuth) win first because they're more
specific than "leftover state".

## Consequences

### Benefits
- Orphan-check failures no longer terminate `stuck` with no
  actionable signal; each sub-shape has a defined home.
- `docs/mock-gaps.md` collects orphan-check leftovers grouped by
  sub-shape, giving a per-mock backlog to drive future handler work.
- Table-driven design avoids a 5-arm conditional in the classifier.

### Tradeoffs
- Seed table is small (6 entries). New sub-shapes will require code
  changes — but rule-of-three keeps the table from churning on every
  one-off scenario.
- Async eventual-consistency (sub-shape 4) is currently a routing
  label only; no retry-with-backoff is implemented. That's a future
  follow-up if it recurs.
