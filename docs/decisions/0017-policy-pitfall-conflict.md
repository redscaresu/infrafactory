# ADR-0017: policy_pitfall_conflict detection

## Status
Accepted

## Context
[ADR-0015](0015-classifier-routing.md) defines the classifier-routing
pattern. This ADR specifies the third hook: detecting when a rego
policy fires against HCL that already follows the prescriptive
pitfall, i.e. the policy is wrong, not the LLM.

The 2026-06-01 deterministic sweep produced the motivating case:
`web-app-paris` and `compute-lb-multi-paris` both failed twice with
`policy=scaleway.vpc_required` despite the LLM producing count-based
`scaleway_instance_server` + matching count-based
`scaleway_instance_private_nic` (exactly as the existing prescriptive
pitfall recommended). The system terminated `stuck` with no actionable
signal; the actual bug was in `policies/scaleway/vpc_required.rego`
comparing planned addresses literally against symbolic refs.

PR #8 fixed that specific rego bug. This ADR captures the **detection
pattern** so future occurrences self-classify.

## Decision
`generator.DetectPolicyConflict(failureDetail, hcl, pitfalls, cloud,
scenario, timestamp)` returns a `PolicyGap` when:
1. `failureDetail` contains `policy=<pkg>.<rule>` (rego deny signal),
2. a resource address (e.g. `scaleway_instance_server.web[0]`) is
   present in the detail,
3. a same-resource prescriptive pitfall exists in
   `pitfalls/<cloud>.yaml`, AND
4. the LLM's HCL contains every backticked keyword from that pitfall's
   `rule` text.

Matched failures append to `docs/policy-gaps.md` (per-cloud table,
dedup-keyed by `(policy, resource)`). Non-matches fall through to the
existing pitfall path.

Conservative by design: false negatives leave the existing pitfall
path active; false positives surface a human-reviewable docs entry.

## Consequences

### Benefits
- Policy bugs no longer terminate `stuck` with no signal — they
  surface as `docs/policy-gaps.md` entries pointing at the rego file.
- `pitfalls/<cloud>.yaml` doesn't accumulate rules the LLM can't act
  on (e.g., "use a different count expression to dodge the rego bug").
- Keyword-extraction reuses backticked tokens from existing pitfalls,
  so no new prompt-engineering vocabulary is introduced.

### Tradeoffs
- Keyword matching is substring-based; an unrelated identifier in the
  HCL could spuriously match. Acceptable because the hook only fires
  after stuck-detection (the LLM has already retried twice without
  progress), which is itself a strong signal of pitfall-vs-policy
  disagreement.
- Backticked-identifier extraction skips short tokens (`≤3 chars`)
  and a small narrative skip-list (`count`, `for`, `and`, `the`,
  `not`). Future false-positive cases may require extending the list.

### Follow-up
- If false positives accumulate, raise the bar from "all keywords
  present" to "all keywords present AND no other LLM-visible mistake
  in the same HCL".
- Consider a periodic prune of `docs/policy-gaps.md` after rego fixes
  land (currently append-only with dedup).

## 2026-06-02 amendment — policy field plumbing

The 2026-06-01 sweep showed `DetectPolicyConflict` never matched any
real failure because the regex hunted for `policy=X.Y` inside
`f.Detail`, but `FailureSummary` and `feedback.Failure` expose
`Policy` as a structured field — `Detail` only carries the rego
deny message ("scaleway_instance_server.api[0] is not attached…").
Fix: change the signature to `DetectPolicyConflict(policy, detail,
hcl, …)` and pass `f.Policy` from the caller. The legacy
`policy=X.Y` extraction is kept as a fall-through so the existing
test cases stay valid. Routing rules unchanged.
