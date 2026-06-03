# Arc: post-sustain tightening (aws-route53 + transport classification + OPA-dup follow-through)

Status: planned (2026-06-03)
Owner: next-session claude (designed for autonomous execution)
Follows: `sustain-and-n13-durability-plan.md` (closed 2026-06-03 with 2/3 sweeps at 39/39 + 1 sweep with aws-route53 flake + transport tail)
Shape: goal-named variable-length arc per AGENTS.md "Planning a New Arc" (4 slices, ~3.5–5.5 hr)

## Big picture

The sustain-and-n13-durability arc validated 39/39 across two clean sweeps but surfaced two distinct flake classes in the third:

1. **One genuine LLM convergence flake**: `aws-route53` `repair_budget_exhausted` after 5 iters / 471s. Reproduces (sweep 1 passed, sweep 3 failed). Bounded scope.
2. **One transport-failure cluster**: 6 scenarios failing at `iteration_1_generate` with 5–9s durations (Claude CLI rate-limit or quota blip). Currently shows up as `repair_budget_exhausted` in `summary.tsv` — same bucket as a genuine LLM convergence failure — which muddies flake-budget characterization on future sweeps.

This arc tightens the remaining loose ends so the next sustain runs cleaner:

- **S96** investigates and fixes aws-route53.
- **S97** classifies transport failures separately in `summary.tsv` so they don't conflate with LLM flakes (no retry yet — that's bigger; just classify).
- **S98** lands an outstanding retirement: GCP phase3 self-review rule #13 explicitly cites `region_restriction` OPA policy. Identified mid-arc but never executed. Same audit walks AWS/Scaleway phase3 for the same shape.
- **S99** extends `TestPitfallsNoOPADuplication` to scan `prompts/*.md` too — S82's ratchet only covered `pitfalls/*.yaml`, which is why rule #13 slipped through. Future duplications get caught automatically.

S99 enforces what S98 retires — natural pairing. Mandatory ARCHIVE close-out folded into S99 per Option C.

## Slices

| Slice | Title | Effort |
|---|---|---|
| S96 | aws-route53 flake fix (pitfall or fakeaws) | ~1-2 hr |
| S97 | Transport-failure classification in sweep_39.sh | ~1 hr |
| S98 | Retire GCP phase3 rule #13 + AWS/Scaleway phase3 OPA-dup audit | ~1-2 hr |
| S99 | Extend OPA-dup ratchet to `prompts/*.md` + arc close-out | ~30 min + close-out |

**Total**: ~3.5–5.5 focused hours.

## Standing rules

Inherit all rules from `slices-54-62-plan.md`, `slices-74-78-plan.md`, `slices-79-83-plan.md`, `slices-84-88-plan.md`, `slices-89-93-plan.md`, and `sustain-and-n13-durability-plan.md`. Same merge authority, scope per PR, sweep protocol (selective discard preserving `learned_from_diff_avoid` via `bin/pitfall-merge`).

Specifically for this arc:

- **S96**: investigate before committing to a fix path. Read `.infrafactory/runs/aws-route53/<latest-failed>/iterations/{1..5}/generated/*.tf` AND the failure detail. Two possible fix paths — pitfall (LLM-side) OR fakeaws Route 53 handler (mock-side). Don't predict; investigate then commit. Could also fall into "Category C — load-bearing prompt rule" outcome.
- **S98**: rule #13 was identified as Category B per ADR-0018 mid-arc-89-93. Don't re-audit from scratch — just execute the N11 retirement protocol on it, plus walk AWS phase3 + Scaleway phase3 self-review files for the same "rule explicitly cites OPA policy name" shape.
- **S99 owns the arc close-out** (per Option C; no separate slice).

## S96 — aws-route53 flake fix

### Motivation

Sweep 1 (2026-06-03): aws-route53 passed iter 1 / 123s. Sweep 3: same scenario, `repair_budget_exhausted` after 5 iters / 471s. Reproduces. The flake budget is therefore not zero — characterizing it is what makes 39/39 a real claim instead of a single-sweep one.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S96-T1 | Locate the most-recent failed aws-route53 run (`ls -td .infrafactory/runs/aws-route53/*/`). Read iter 1, 3, 5 `generated/*.tf` files — look for oscillation (attribute name flipping, resource added/removed across iters). Check `run.json` for the failure detail. | P0 | — |
| S96-T2 | Categorize: (a) LLM oscillates between two HCL shapes → pitfall fix (typically `source: learned` describing the wrong shape to avoid); (b) fakeaws Route 53 handler returns wrong shape on Read → fakeaws PR; (c) genuine LLM-side ambiguity with no clear fix → Category C prompt rule add. | P0 | S96-T1 |
| S96-T3 | Execute the chosen fix. End-to-end re-run aws-route53 alone (`./bin/infrafactory run scenarios/training/aws-route53.yaml`); confirm `target_reached`. If fakeaws-side: PR in `../fakeaws`, rebuild via `make fakeaws-restart`. If pitfall-side: hand-add to `pitfalls/aws.yaml` as M91-permitted `source: learned` (same path as S85's SNC pitfall). | P0 | S96-T2 |
| S96-T4 | Single-PR commit with the fix + reference to the investigation findings in the PR description. | P0 | S96-T3 |

### Exit criteria

- aws-route53 converges target_reached on a fresh end-to-end run.
- Fix is durable enough to land — meaning the next sweep (S97-T3 or later) should pass aws-route53.

## S97 — Transport-failure classification

### Motivation

Sweep 3's tail showed 6 scenarios fail with shapes like:

```
$scenario	repair_budget_exhausted	failed	2	5
$scenario	repair_budget_exhausted	failed	2	6
$scenario	repair_budget_exhausted	failed	2	9
```

Two iterations, durations 5-9 seconds, both iterations failing at `iteration_1_generate` (the Claude CLI invocation itself). That's a transport-failure cluster, not LLM convergence — the LLM never even produced HCL. But `summary.tsv` calls it `repair_budget_exhausted`, indistinguishable from "the LLM iterated 5 times and couldn't converge" (genuine flake).

Result: any future "X/39" claim conflates the two. We can't tell from `summary.tsv` whether 32/39 is "32 deterministic + 7 real flakes" or "32 deterministic + 6 transport + 1 real flake" — sustain validation gets noisier with each transport-blip.

The fix is to detect the pattern and report it separately:

| Existing summary column | New behavior |
|---|---|
| `terminal_reason` | Now also `transport_failed` for the detected shape |
| `status` | Already `failed` — unchanged |
| `iters` | Already low (1-2) — unchanged |
| `dur_s` | Already < 30s typically — unchanged |

Detection rule: `dur_s < 30 AND iter_count ≤ 2 AND every iteration failed at the generate stage (not test or validate)`. Approximate — false positives surface as `transport_failed` rows that the operator can re-classify manually; false negatives stay as `repair_budget_exhausted` (the existing behavior).

No retry. That's a bigger arc (LLM-transport robustness). S97 just classifies.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S97-T1 | Modify `scripts/sweep_39.sh`: after the existing per-scenario loop, for each row in `summary.tsv` where `terminal_reason=repair_budget_exhausted`, examine the matching `.log` file. If every iteration's failed stage is `iteration_*_generate` (no `_test` or `_validate` failures) AND `dur_s < 30`, rewrite the row's `terminal_reason` to `transport_failed`. | P0 | — |
| S97-T2 | At sweep end, emit an additional `TRANSPORT_FAILED=N` summary line alongside the existing `PASS=X / TOTAL=Y` and `PANIC_LINES=N` and `N13_EMISSIONS=N` lines. | P0 | S97-T1 |
| S97-T3 | Add the classifier to the panic-gate region of the script (same shape — non-fatal warning, doesn't fail exit code). Document the heuristic inline. | P0 | S97-T1 |
| S97-T4 | Run one sweep to validate the classifier on real data. If no transport failures occur this sweep, do a dry-run unit-test with a hand-crafted summary + log. | P0 | S97-T3 |
| S97-T5 | Single PR. Updated AGENTS.md sweep-protocol bullet (the four-ratchet set is unchanged, but `summary.tsv` shape changes — note the new `transport_failed` value). | P0 | S97-T4 |

### Exit criteria

- `scripts/sweep_39.sh` reclassifies transport failures and emits `TRANSPORT_FAILED=N`.
- Documented heuristic inline.
- Validated against real data OR hand-crafted fixture.

## S98 — Retire GCP phase3 rule #13 + AWS/Scaleway phase3 OPA-dup audit

### Motivation

Mid-arc-89-93, the user highlighted GCP phase3 self-review rule #13 ("Region restriction: ... The `region_restriction` OPA policy enforces this"). The rule explicitly cites the OPA policy that already enforces it — textbook Category B per ADR-0018 (load-bearing but a replacement carrier exists). I categorized it and recommended retirement, but never executed.

This slice closes that loop. Same scan applied to AWS phase3 + Scaleway phase3 self-review files for additional rules of the same shape (prompt rule names an OPA policy as the enforcement mechanism).

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S98-T1 | Open `prompts/gcp/phase3_self_review.md`. Delete rule #13 entirely (the entire "Region restriction" block). Re-number subsequent rules if needed. | P0 | — |
| S98-T2 | Audit `prompts/aws/phase3_self_review.md` + `prompts/scaleway/phase3_self_review.md` for rules that explicitly cite an OPA policy name. Document each in a slice-internal table — retire same-shape Category B candidates; document Category C with rationale. | P0 | S98-T1 |
| S98-T3 | For each retirement, execute the N11 7-step protocol — re-run impacted scenarios to confirm OPA enforcement covers the dropped rule (region_restriction.rego is the immediate test). | P0 | S98-T2 |
| S98-T4 | Single PR. Document in PR body + STATUS.md. | P0 | S98-T3 |

### Exit criteria

- GCP phase3 rule #13 retired.
- AWS + Scaleway phase3 self-review files audited for same shape.
- All retirements validated by N11 protocol.

## S99 — Extend OPA-dup ratchet to `prompts/*.md` + arc close-out

### Motivation

S82 landed `TestPitfallsNoOPADuplication` to catch verbatim OPA-msg duplication in `pitfalls/<cloud>.yaml`. Rule #13 slipped past it because the ratchet only scanned pitfall YAMLs, not prompt markdown. Extending the scan to `prompts/**/*.md` catches future cases of the same shape automatically.

S99 also folds the mandatory arc close-out (STATUS + NEXT_SESSION + ARCHIVE) per Option C.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S99-T1 | Extend `TestPitfallsNoOPADuplication` in `internal/generator/pitfalls_opa_dedup_test.go` to also walk `prompts/<cloud>/*.md` and apply the same `msg := sprintf(...)` literal-substring check. Or split into a sibling test (`TestPromptsNoOPADuplication`) for cleaner failure messages. | P0 | S98 |
| S99-T2 | Verify the new test fires correctly: stash the S98 retirement temporarily and confirm the ratchet would have caught rule #13. Restore the retirement after verification. | P0 | S99-T1 |
| S99-T3 | Single PR with the ratchet extension + arc close-out: STATUS + NEXT_SESSION update; `docs/status/ARCHIVE.md` § "post-sustain tightening" with per-slice narrative. | P0 | S99-T2 |

### Exit criteria

- New ratchet test catches the rule-#13 shape.
- Arc close-out narrative committed.
- Suggested next arc noted in `docs/NEXT_SESSION.md` (or "no obvious next arc — sustain again to confirm post-fix state").

## Why this order, in one paragraph

S96 first because it's the user-visible delta (39/39 sustained). S97 second because it enhances the sustain-validation tooling that S96 just exercised — natural pairing. S98 third because the retirement has been pending for two arcs; we owe it. S99 last because it ratchets what S98 retired (future-proofing) and naturally absorbs the close-out (no padding).

## Autonomous-execution loop prompt

```
/loop until all 4 slices (S96, S97, S98, S99) in docs/plans/post-sustain-tightening-plan.md are complete: every slice's exit criteria met, every PR merged with green CI, STATUS + NEXT_SESSION updated.

Read docs/NEXT_SESSION.md first for the prior arc's handoff, then docs/plans/post-sustain-tightening-plan.md for the slice definitions. The standing rules from slices-54-62-plan.md, slices-74-78-plan.md, slices-79-83-plan.md, slices-84-88-plan.md, slices-89-93-plan.md, AND sustain-and-n13-durability-plan.md apply. The S94 selective-discard rule still applies: `learned_from_diff_avoid` survives sweep teardown via `bin/pitfall-merge`; `learned` + `learned_from_diff` discarded as before.

Work slices in order S96 → S97 → S98 → S99. S99 folds the mandatory ARCHIVE + NEXT_SESSION close-out per the Option C arc shape — no separate close-out slice.

Authority: open + merge PRs with `gh pr merge <N> --squash --admin --delete-branch` once CI is green, in all four repos. S96 may be either a fakeaws PR (mock-side) or an infrafactory pitfall add (LLM-side) — investigate before committing to the fix path.

Stop only when: (a) all 4 slices complete OR (b) you genuinely cannot proceed (regression in main blocks every scenario AND fix-forward isn't obvious — document the blocker in NEXT_SESSION + stop).

Before stopping for any reason, update docs/NEXT_SESSION.md with: which slices closed, which are blocked, what next session should do FIRST.
```

## Fresh-context checklist

The autonomous prompt assumes the agent reads, in order:
1. `AGENTS.md` (Option C goal-named variable-length arcs)
2. `docs/NEXT_SESSION.md`
3. This file (`docs/plans/post-sustain-tightening-plan.md`)
4. `STATUS.md`
5. `docs/status/ARCHIVE.md` § "2026-06-03 sustain + N13 durability" (for the sweep-3 transport-tail data + aws-route53 reproduction signal)
6. `docs/decisions/0018-n11-retirement-criteria.md` (for the Category A/B/C protocol S98 executes)
7. `internal/generator/pitfalls_opa_dedup_test.go` (the existing S82 ratchet that S99 extends)
8. `scripts/sweep_39.sh` (the file S97 modifies)
