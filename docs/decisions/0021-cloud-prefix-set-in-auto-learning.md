# ADR-0021: Cloud-prefix set in the auto-learning pipeline

Status: accepted
Date: 2026-06-06
Tags: learning-system, multi-cloud, regression

## Context

The dynamic learning loop's resource-name extraction uses regex patterns to identify the Terraform resource family from a failure detail. Three sites carry the set:

- `internal/generator/pitfalls_learn.go::resourceNameRe`
- `internal/generator/prescriptive_extractor.go::addressRe`
- `internal/cli/run_command.go::pitfallResourceMatchesCloud`

For the first three clouds (Scaleway, GCP, AWS), the set was `(scaleway|google|aws)_` everywhere. When S114 added Genesys as a peer cloud, none of these three sites were updated. The result was undetected for ~3 weeks: every Genesys failure produced an empty `Resource`, `ExtractDescriptivePitfall` returned nil, and the run-finalization "learn-from-budget-exhausted" path silently dropped each candidate. Sustain sweep 1 surfaced it via `genesys-architect-flow` exhausting its 5-iteration repair budget without emitting a single pitfall.

## Decision

The cloud-prefix set in all three sites is the canonical source-of-truth for which clouds the learning pipeline supports. Adding a new cloud (or any new Terraform resource-name prefix) requires updating all three sites in lockstep. There is no fallback: a missed site silently disables learning for that cloud.

Current set (post-S118): `scaleway | google | aws | genesyscloud`. `random_` is whitelisted in `addressRe` for the prescriptive extractor's diff-based path (it's a meta-provider that appears across clouds).

## Consequences

- Every new cloud arc must touch these three files explicitly.
- Regression test `TestExtractResourceFromDetail_Genesys` (and any future per-cloud equivalent) fences off the silent-no-op failure mode.
- The diagnostic protocol for "auto-learning didn't fire" is now codified in `AGENTS.md § The auto-learning pipeline is load-bearing — never excuse its silence`.

## Alternatives considered

1. **Generic prefix matcher (any `\w+_\w+`)**. Rejected: too permissive — Terraform's own resource-name shape is too loose, and we'd lose the cross-cloud pollution guard (`pitfallResourceMatchesCloud`).
2. **Compile-time enum drawn from `internal/cli/cloudConstraintPolicies` keys**. Rejected: the constraint-policies map is downstream of the extraction layer; couples two concerns that are independently evolved. Better to fix the gap with a regression test + the explicit checklist in AGENTS.md than to over-engineer the linkage.
3. **Late-binding via a registry hook in each `internal/generator/*.go` file**. Rejected: same coupling concern; the three sites are small enough that the "lockstep edit" rule is cheaper than a registry.

## Related

- ADR-0020 (fakegenesys as the 4th cloud).
- ADR-0019 (learning vocabulary rename — same family of pipeline concerns).
- infrafactory#96 (the fix).
