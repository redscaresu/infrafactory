# ADR-0015: Classifier-routed failure handling at stuck/budget termination

## Status
Accepted

## Context
The dynamic-pitfalls loop (ADR-0012) appends to `pitfalls/<cloud>.yaml`
whenever the LLM hits a recurring failure that points at an
LLM-actionable mistake (e.g., `Unsupported argument`, missing required
field). The 2026-05-30/31 and 2026-06-01 sweeps surfaced three failure
classes the loop should NOT learn pitfalls for:

1. **Mock-gap signals** — `501 Not Implemented`,
   `plugin did not respond`, `couldn't find resource`, OAuth-escape
   errors. The LLM can't write HCL to work around a broken mock; the
   fix lives in `fakeaws` / `fakegcp` / `mockway`. Seeding pitfalls
   for these wastes prompt space and lies about the fix path.
2. **Orphan-check failures** — post-destroy `/mock/state` shows
   leftover resources. Five distinct sub-shapes (LLM soft-delete,
   provider soft-delete, parent-cascade gap, async eventual-consistency,
   genuine bug) each have a different fix surface (mock handler vs
   provider bug vs orphan-checker code).
3. **policy_pitfall_conflict** — the LLM's HCL matches an existing
   prescriptive pitfall verbatim, but a rego policy still fires. The
   policy is wrong, not the LLM. Pitfalls can't fix policies.

The current loop only knows the binary "learn a pitfall or terminate
stuck". Both branches mishandle the three classes above.

## Decision
Add three classifier hooks before the stuck/budget termination path
in `internal/cli/run_command.go`. Each consumes the recurring
`FailureSummary` and the LLM's last-generated HCL, and routes
matched failures to a dedicated docs/ surface instead of polluting
`pitfalls/<cloud>.yaml`:

| Hook | File | Routing target |
|---|---|---|
| `generator.IsMockActionable` | `internal/generator/pitfalls_learn.go` | `docs/mock-gaps.md` |
| `generator.ClassifyOrphans` | `internal/generator/orphan_classify.go` | `docs/mock-gaps.md` (typed) |
| `generator.DetectPolicyConflict` | `internal/generator/policy_gap.go` | `docs/policy-gaps.md` |

Hooks run in order; the first match wins. Non-matches fall through
to the existing pitfall-learning path, preserving current behaviour
for genuine LLM mistakes.

Each hook is **conservative by design**: false negatives leave the
existing pitfall path active (no regression); false positives surface
a docs entry for human review (cheap to ignore).

A source-ratchet test
(`internal/generator/pitfalls_source_ratchet_test.go`) blocks
mock-actionable substrings from re-entering committed pitfalls.

## Consequences

### Benefits
- `pitfalls/<cloud>.yaml` only contains rules the LLM can act on,
  preserving prompt-space economy and signal-to-noise.
- Mock bugs, orphan sub-shapes, and policy bugs each route to a
  dedicated docs surface humans can scan after a sweep.
- The 2026-06-01 web-app-paris case (count-based
  `scaleway.vpc_required` false positive) is now self-classifying —
  future occurrences land in `docs/policy-gaps.md` instead of
  terminating stuck.

### Tradeoffs
- Three additional classification calls per stuck-detection cycle.
  Negligible cost (regex + substring scans on the failure detail).
- Keyword-based heuristics (`backtickedIdentRe` for pitfall rules,
  `mockActionableSignals` substring list for mock gaps) are
  approximate. Tuned against observed sweep data; revisited when a
  new sub-shape recurs across 3+ scenarios (see
  `feedback_orphan_check_extractor.md` rule of three).
- Docs files (`mock-gaps.md`, `policy-gaps.md`) are append-only with
  dedup. Stale entries accumulate; periodic prune is a manual task.

### Follow-up
- Extend `OrphanSubshape` table as new sub-shapes recur.
- Watch for false positives in `DetectPolicyConflict`; if observed,
  raise the matching threshold from "all keywords" to "all keywords
  AND no other LLM-visible mistake in the same HCL".
- If a fourth class emerges, generalise these hooks into a single
  `RouteFailure(summary, hcl, ctx) Routing` dispatcher.

## 2026-06-02 amendment — stuck/budget path wiring

The 2026-06-01 deterministic sweep (30/39 pass) surfaced that
`IsMockActionable` was wired ONLY into the self-correction path at
`run_command.go:194` and not the stuck/budget-exhausted termination
path at line 372. Four GCP scenarios (`gcp-cloud-run`, `gcp-cloud-sql`,
`gcp-gke-cluster`, `gcp-storage`) hit `repair_budget_exhausted` and
re-learned the same OAuth-escape and plugin-crash pitfalls that N2
had just pruned. Fix: mirror the same `IsMockActionable` guard in
the stuck/budget loop. The classifier-routing pattern itself is
unchanged; the bug was in coverage, not design.

## Amendment (2026-06-02, S68 — coverage extension)

S63's post-collapse sweep surfaced two failure shapes that
`IsMockActionable` didn't recognize:

1. **Provider polling on a mock-side state field that doesn't
   persist** — the provider acks the Update, then polls until the
   field flips to the requested value, times out because the mock
   never persisted the change. Shapes:
   - `waiting for state to become 'true' (last state: 'false', timeout: ...)` (lower-case, EC2 Subnet MapPublicIpOnLaunch)
   - `waiting for state to become 'TRUE' (last state: 'FALSE', timeout: ...)` (upper-case, AWS KMS rotation)
2. **Mock acks Create but Read returns 0 rows** — distinct from the
   pre-existing `ResourceNotFoundException` shape (which uses a
   structured exception type). The Route 53 case in S63 used
   `Error: reading Route 53 Record (...): empty result`.

Both are textbook mock-side state-divergence — the LLM's HCL is
correct, the mock just doesn't persist the change. Without the
extension, these failures landed in `pitfalls/<cloud>.yaml` as
`learned` entries on every sweep.

S68 added four substring signals (two upper/lower case variants
each for `waiting for state to become`, plus `empty result`) and
three regression tests pinning the real S63 failure strings. The
existing `TestPitfallsNoMockActionableSeeds` ratchet caught a
pre-existing stale `aws_subnet` entry on first CI run after the
extension; it was pruned in the same PR.

Same conservative-substring-match discipline as the original
classifier: false negatives leave the legacy learning path active;
false positives drop a learning that arguably belongs in pitfalls.

## Amendment (2026-06-02, S78 — GCP escape resource carve-out)

S73 retired GCP phase2 rules 9 + 12 (the prescriptive "don't use
google_project_service / google_project_iam_member" prompt rules)
on the theory that the N10→N13 auto-derivation pipeline would
re-learn the avoid patterns from sweep failures. S76 sustained at
37/39 — but during that sweep, every `ACCESS_TOKEN_TYPE_UNSUPPORTED`
failure on those resource types correctly classified as
mock-actionable per the original 2026-06-02 amendment, routing them
to `docs/mock-gaps.md` rather than `pitfalls/gcp.yaml`. This is the
right call for the typical case (the v5 provider escapes to real
cloud because fakegcp lacks the route) but wrong for this specific
class: the LLM-side fix (drop the resource) IS attainable, and the
mock-side fix is structurally impossible (these resources can't be
modeled in fakegcp). Without a carve-out, N13 never sees these
failures on stuck termination, and the system has no mechanism to
re-learn the rule a future scenario will require.

S78 added a narrow exclusion: when the matched signal is
`access_token_type_unsupported` AND the failure detail contains a
reference to one of `google_project_service`,
`google_service_networking_connection`, `google_project_iam_member`,
`google_project_iam_binding`, or `google_project_iam_policy`, the
classifier returns false. The bare-signal case (no escape-resource
reference) and the signal-on-other-GCP-resource case both stay
mock-actionable.

Two regression tests pin both shapes (`TestIsMockActionable_GCPEscapeCarveOut`).

This is the first carve-out from the conservative "first signal
wins" pattern. The general design holds: false negatives leave the
legacy path active. The carve-out is narrow (one signal × five
resource types) and motivated by a specific architectural mismatch
(LLM-actionable resource × unmockable backend); it should not be
generalized without a similar concrete case.

## Amendment (2026-06-02, S78 — Makefile target)

S77 surfaced that every prior sustain-ratchet sweep reinvented the
same shell harness in `/tmp/sweep-*.sh`. S78 landed
`scripts/sweep_39.sh` + `make sweep-39` as the canonical entry
point. The harness uses `./bin/infrafactory mock reset` (the S67
CLI) between scenarios — a bare `curl -X POST /mock/reset` to
fakeaws does NOT cascade to SeaweedFS, which caused the S54 state-
leak post-mortem. Discards `pitfalls/*.yaml` additions per
`feedback_sweep_protocol.md` (sweep noise re-emerges on the next
run). Not a routing change; a workflow ratchet.
