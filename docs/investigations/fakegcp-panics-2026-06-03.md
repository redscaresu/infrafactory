# fakegcp `plugin did not respond` triage (S86)

Date: 2026-06-03
Slice: S86

## Scope

`docs/mock-gaps.md` carries five `plugin did not respond` entries against the GCP and Scaleway mock surfaces:

| Resource | Scenario | First seen | Detail shape |
|---|---|---|---|
| `google_kms_crypto_key_iam_member` | gcp-gke-cluster | 2026-06-01 | "Provider produced inconsistent result after apply" |
| `google_container_node_pool` | gcp-gke-cluster | 2026-06-01 | "Plugin did not respond" |
| `google_compute_instance` | gcp-full-stack | 2026-06-01 | "Plugin did not respond" |
| `google_sql_database_instance` | gcp-full-stack | 2026-06-01 | "Plugin did not respond" |
| `_(none)_` | compute-lb-multi-paris | 2026-06-02 | "Plugin did not respond" via plugin6.GRPCProvider.ApplyResourceChange |

The plan called for identifying panic sources and picking one to fix (rule-of-three).

## Finding: all five entries are non-reproducible in the current sweep state

Cross-referenced against S81 sweep results:

| Scenario | S81 outcome | Mock-gap status |
|---|---|---|
| gcp-gke-cluster | target_reached iter 1 / 101s | NOT reproducible |
| gcp-full-stack | repair_budget_exhausted iter 5 — but on `service_networking_connection` escape, NOT on `compute_instance` or `sql_database_instance` | NOT reproducible (different failure shape) |
| compute-lb-multi-paris | target_reached iter 2 / 160s | NOT reproducible |

The cumulative fakegcp + fakeaws improvements between 2026-06-01 and 2026-06-02 — S77 KMS rotation, S79 KMS tags, S80 s3router shim, plus organic mock-actionable fixes — appear to have silently resolved the underlying panics. Direct inspection of `/private/tmp/infrafactory-mocks/fakegcp.log` (current session, mocks-restart from S81+S82 boundary) shows zero `panic`, `recovered`, `runtime error`, or `nil pointer` lines.

## Signal-vs-detail mismatch on the first entry

The `google_kms_crypto_key_iam_member` row has signal `plugin did not respond` but the detail body starts with "Provider produced inconsistent result after apply." Those are two distinct error classes — the row's signal label was approximate. The actual error is a state-divergence between fakegcp's Create response and Read response, which is closer to the "wrong-shape" mock-actionable family than the panic family.

## Recommendation for S87

Pivot S87 from "fix the highest-impact panic" to one of:

1. **Trim stale entries from mock-gaps.md.** Cheap close-out; `docs/mock-gaps.md` is a git-untracked runtime artifact that regenerates from sweep classifier output. The next sweep won't re-add these entries unless the panics actually recur.
2. **Add a panic-detection harness for fakegcp.** Run all GCP scenarios in sequence; tail fakegcp log; fail loudly if any `panic\|recovered` line appears. Catches regressions before they reach mock-gaps. Bounded scope: ~1 hour.
3. **Investigate the `Provider produced inconsistent result` entry separately.** That's a distinct error class from "plugin did not respond" panics. Probably worth a focused look at fakegcp's `google_kms_crypto_key_iam_member` Read flow vs Create flow.

S87 will pick (1) + (2) for the close-out, since neither requires reproducing a non-reproducible bug. (3) deferred to next arc.

## Why this happens (architectural pattern)

`docs/mock-gaps.md` is append-only with dedup on `(cloud, signal, resource)`. The dedup prevents bloat but doesn't expire stale entries — a once-flaky panic stays in the file forever unless something explicitly trims it. Sweeps that pass don't decrement the entry; only an explicit prune removes it.

This is a known design choice (the file is an audit log, not a live ticket queue). For S87's sake, the right move is to acknowledge it: the next sweep will surface what's still broken. The historical entries are valuable for *what we used to be broken on*, not *what's broken now*.
