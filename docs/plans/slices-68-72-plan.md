# Slices 68–72 — classifier coverage + extractor polish + OPA false-fire fix + sustaining retirements

Status: planned (2026-06-02)
Owner: next-session claude (designed for autonomous execution)
Follows: `slices-63-67-plan.md` (post-collapse validation arc — closed 2026-06-02)

## Big picture

The S63–S67 arc closed with the auto-learning + retirement machinery validated end-to-end. This arc cleans up the remaining open items in `BACKLOG.md` + the carryover follow-ups from S63's audit, and lands the OPA known-after-apply fix that's been outstanding since M98 was filed.

- **S68** plugs the two N3 classifier coverage gaps from S63's audit.
- **S69** closes M96 by auditing whether `ExtractLearnedPitfall` still produces descriptive (non-prescriptive) output in any path that N10/N13 don't cover; close-as-superseded or shrink the legacy path.
- **S70** promotes the throwaway `cmd/n10extract` helper to a permanent CLI command, so future N11 retirements have a stable forced-extract path.
- **S71** lands the M98 OPA known-after-apply fix — the policy false-fire that masks legitimate HCL on networked GCP scenarios.
- **S72** runs an N11 sustaining audit on AWS phase3 + Scaleway phase3 per ADR-0018; expect 1–2 more Category-A retirements per cloud.

Total ~6–10 focused hours. One autonomous loop session.

## Slices

| Slice | Title | Effort |
|---|---|---|
| S68 | N3 classifier coverage gap (S63 audit finding 2) | ~30 min |
| S69 | M96 close-out — audit ExtractLearnedPitfall vs N10/N13 | ~1 hr |
| S70 | Permanent `cmd/n10extract` CLI command | ~1–2 hr |
| S71 | M98 — OPA known-after-apply false-fire fix | ~half-day |
| S72 | ADR-0018 sustaining audit — AWS + Scaleway phase3 retirements | ~2–3 hr |

**Total**: ~6–10 hr. Standing rules inherit from `slices-54-62-plan.md`.

**Not in this arc** (carryover):
- **M53**: OSS visibility flip + branch protection. Click-ops only; cannot be solved by an agent. Captured in NEXT_SESSION as a permanent reminder until the repo owner does it.

## Standing rules

Inherit all rules from `slices-54-62-plan.md` § "Standing rules". Same authority to `gh pr merge --squash --admin --delete-branch`, same pitfall-pollution discipline, same per-PR scope.

## S68 — N3 classifier coverage gap (S63 audit finding 2)

### Motivation
The S63 sweep let two mock-actionable failures land as `learned` pitfalls instead of routing to `docs/mock-gaps.md`:
- `aws_kms_key`: "waiting for KMS Key rotation update: timeout while waiting for state to become 'TRUE'"
- `aws_route53_record`: "Error: reading Route 53 Record … empty result"

`IsMockActionable` (in `internal/generator/`) recognizes the established mock-gap signals (501, "Plugin did not respond", OAuth-escape, Describe*-404) but not these two shapes. The pitfalls files accumulate these as `learned` entries on every sweep until the predicate catches them.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S68-T1 | Extend `IsMockActionable` to recognize: (a) `waiting for .* update: timeout while waiting for state to become` (provider polling on a mock-side field that doesn't persist — the same pattern as `aws_subnet` MapPublicIpOnLaunch); (b) `Error: reading .* empty result` (Route 53 / similar reads that the mock's read handler returns empty for). | P0 | — |
| S68-T2 | Add unit tests pinning both patterns. Existing tests in `internal/generator/pitfalls_learn_test.go`. | P0 | S68-T1 |
| S68-T3 | Extend the `TestPitfallsNoMockActionableSeeds` ratchet so any future sweep that lands one of these as `learned` would CI-fail. | P0 | S68-T1 |
| S68-T4 | PR + STATUS + ADR-0015 amendment (mock-actionable classifier coverage). | P0 | S68-T3 |

### Exit criteria
- `IsMockActionable` returns true for both real S63 failure-detail strings.
- Two regression tests pin the new signals.
- M91 ratchet covers the new mock-actionable shapes.

## S69 — M96 close-out — audit ExtractLearnedPitfall vs N10/N13

### Motivation
M96 in `BACKLOG.md`: the legacy `ExtractLearnedPitfall` produces descriptive rules ("X failed because…") rather than prescriptive ones. N10 (`ExtractPrescriptiveFix`, addition-as-fix) and N13 (`ExtractPrescriptiveAvoid`, removal-as-fix) now cover the prescriptive shape. Question: is `ExtractLearnedPitfall` still firing for any pattern N10/N13 don't cover? If yes, document the remaining gap. If no, close M96 as superseded.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S69-T1 | Read `internal/generator/pitfalls_learn.go::ExtractLearnedPitfall` + every call site in `run_command.go`. Enumerate every output shape (oscillation-detected branch, stuck-detection branch, the M97 templates). | P1 | — |
| S69-T2 | For each shape, decide: (a) covered by N10's addition-as-fix on a passing iter pair; (b) covered by N13's deletion-as-fix; (c) still uniquely owned by `ExtractLearnedPitfall` (e.g. truly-descriptive policy failures with no fix shape). | P0 | S69-T1 |
| S69-T3 | For any (c) cases: either keep + document the niche, or refactor to feed N10/N13 if there's a path. For (a)/(b) cases: confirm the new extractors cover them in practice (audit recent sweep output). | P0 | S69-T2 |
| S69-T4 | PR closing M96 as superseded OR documenting the remaining niche. Update BACKLOG.md status. | P0 | S69-T3 |

### Exit criteria
- M96 is either closed (superseded) or has a tightened scope with concrete examples it still handles.
- No code is removed unless every call site is provably covered by N10/N13.

## S70 — Permanent `cmd/n10extract` CLI command

### Motivation
N11 step 2's protocol says: "If no `learned_from_diff` entry exists for the pattern, force one via recorded-run replay." The 2026-06-02 loop session did this with a hand-rolled `cmd/n10extract/main.go` (~50 LoC) that was written + deleted in the same session. Future N11 retirements will need the same tool — making it permanent removes the friction.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S70-T1 | Add `cmd/n10extract/main.go` taking `--failed-dir`, `--passing-dir`, `--failure-detail`, `--failure-resource`, `--cloud`, `--scenario`, `--mode {fix,avoid}` flags. Calls `generator.ExtractPrescriptiveFix` or `generator.ExtractPrescriptiveAvoid` based on `--mode` and prints the resulting `LearnedPitfall` as YAML to stdout. | P0 | — |
| S70-T2 | Convenience flag `--run-dir <dir>`: auto-discover the failed/passing iter pair from an `.infrafactory/runs/<scenario>/<run-id>/iterations/` tree. Pick the last failing + first passing iter. | P1 | S70-T1 |
| S70-T3 | Unit tests + an example invocation in the README or in `docs/decisions/0012` § N11 protocol. | P1 | S70-T1 |
| S70-T4 | PR + ADR-0012 amendment (cmd/n10extract permanence). | P0 | S70-T3 |

### Exit criteria
- `./bin/n10extract --run-dir .infrafactory/runs/gcp-storage/<run-id>/ --failure-detail "..." --mode fix` produces a valid pitfall snippet on stdout.
- Documented invocation in ADR-0012's N11-protocol section.

## S71 — M98 — OPA known-after-apply false-fire fix

### Motivation
M98 in `BACKLOG.md` (P1): `policies/gcp/vpc_required.rego` checks `resource.values.network_interface[_].subnetwork != null/""`, but at plan time `google_compute_subnetwork.NAME.id` resolves to `null` in `planned_values` because the subnetwork is `known after apply`. Policy flags correct HCL as broken. Same shape probably applies to `encryption.rego` and similar.

The auto-learning loop can't possibly close gcp-full-stack failures while this gate false-fires on correct output. (S63's sweep DID pass gcp-full-stack, but iter counts were inflated.)

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S71-T1 | Audit all rego policies under `policies/`. For each policy, check whether it inspects `planned_values` for an attribute that resolves to `known after apply` at plan time. Build a list. | P0 | — |
| S71-T2 | For each affected policy, decide: (a) read from `configuration` (the symbolic HCL view that DOES contain the resource reference) instead of `planned_values`; (b) accept `null`/empty if the corresponding `references` array shows a known-after-apply binding. Pick the simpler of the two per policy. | P0 | S71-T1 |
| S71-T3 | Update affected policies. Add rego unit tests that pin the fix against synthetic `tfplan.json` fixtures (one with `known after apply`, one with a literal). | P0 | S71-T2 |
| S71-T4 | Re-run gcp-vm-network + gcp-full-stack + gcp-gke-cluster (the most VPC-dense scenarios). Verify iter counts drop (or at least don't regress). | P0 | S71-T3 |
| S71-T5 | PR + ADR amendment (or new ADR) about the rego "use configuration not planned_values for `known after apply`" rule. | P0 | S71-T4 |

### Exit criteria
- Every affected policy is fixed (or documented as deliberately strict).
- Rego unit tests pin the fix.
- Three GCP scenarios re-run with comparable or lower iter counts vs S63 baseline.

## S72 — ADR-0018 sustaining audit — AWS + Scaleway phase3 retirements

### Motivation
ADR-0018 documents the N11 retirement criteria. S60 retired one bullet on each of AWS phase3 and Scaleway phase3. Several more are likely Category-A candidates — single-attribute or simple multi-attribute rules with strong validator/policy feedback. This slice runs the audit + lands the easy retirements.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S72-T1 | Re-audit `prompts/aws/phase3_self_review.md`. Classify each remaining prescriptive rule per ADR-0018 (Category A, B, or C). | P1 | — |
| S72-T2 | Re-audit `prompts/scaleway/phase3_self_review.md`. Same classification. | P1 | — |
| S72-T3 | Pick the 1–2 most clearly Category-A candidates from each cloud. Execute the 7-step N11 protocol per candidate. | P0 | S72-T1, S72-T2 |
| S72-T4 | Combined PR with the retirements. STATUS + NEXT_SESSION update with the running prompt-rule count. | P0 | S72-T3 |

### Exit criteria
- AWS + Scaleway phase3 audits documented.
- 2–4 additional retirements landed.

## Why this order, in one paragraph

S68 first because it's the smallest concrete win and it cleans up a recurring noise source (mock-actionable seeds in pitfalls/aws.yaml after every sweep). S69 next while the recent S63–S67 N10/N13 work is fresh in the agent's memory — easier to reason about which extractor handles which shape. S70 promotes the tool while ADR-0012 is still front-of-mind. S71 (M98) is the largest unknown — leave time. S72 last because the sustain-arc audits are cheap and benefit from any S71 policy-fix iter-count drops.

## Autonomous-execution loop prompt

Paste this into a fresh Claude session to start the arc:

```
/loop until all 5 slices (S68-S72) in docs/plans/slices-68-72-plan.md are complete: every slice's exit criteria met, every PR merged with green CI, STATUS + NEXT_SESSION updated.

Read docs/NEXT_SESSION.md first for the prior arc's handoff, then docs/plans/slices-68-72-plan.md for the slice definitions. The standing rules from slices-54-62-plan.md apply (authority to merge, pitfall pollution discipline, mock rebuild discipline, STATUS+ADR+NEXT_SESSION updates, scope per PR).

Work slices in order S68 → S69 → S70 → S71 → S72. Each slice is one PR. S69 may close M96 as superseded (no code change) — that's a valid outcome; capture it in the PR body.

Authority: open + merge PRs with `gh pr merge <N> --squash --admin --delete-branch` once CI is green, in all four repos. Discard auto-learning sweep pollution in pitfalls/*.yaml with `git checkout pitfalls/` — never hand-edit.

Stop only when: (a) all 5 slices complete OR (b) you genuinely cannot proceed (regression in main blocks every scenario AND fix-forward isn't obvious — document the blocker in NEXT_SESSION + stop).

Before stopping for any reason, update docs/NEXT_SESSION.md with: which slices closed, which are blocked, what next session should do FIRST.
```

## Fresh-context checklist

The autonomous prompt assumes the agent reads, in order:
1. `AGENTS.md`
2. `docs/NEXT_SESSION.md` (the §"READ FIRST" pointer)
3. This file (`docs/plans/slices-68-72-plan.md`)
4. `STATUS.md`
5. `BACKLOG.md` (M96 + M98 + M53 are still listed there)
6. `docs/decisions/0012-dynamic-pitfalls.md` + `0015-classifier-routing.md` + `0018-n11-retirement-criteria.md`
7. `docs/plans/slices-54-62-plan.md` § "Standing rules" + the N11 7-step protocol
