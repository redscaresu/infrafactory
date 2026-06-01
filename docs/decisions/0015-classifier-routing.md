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
