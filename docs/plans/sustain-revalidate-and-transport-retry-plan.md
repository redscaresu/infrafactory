# Arc: sustain re-validation + LLM-transport retry

Status: planned (2026-06-03)
Owner: next-session claude (designed for autonomous execution)
Follows: `post-sustain-tightening-plan.md` (closed 2026-06-03 with fakeaws#7 + #78/#79/#80)
Shape: goal-named variable-length arc per AGENTS.md "Planning a New Arc" (2 slices, ~4–5 hr)

## Big picture

Two complementary follow-ups from the post-sustain tightening arc:

1. **Sustain re-validation** (S100). The post-sustain arc landed three behavioural changes — fakeaws Route 53 fix (S96), transport-failure classifier (S97), GCP phase3 rule #13 retirement (S98). None has been exercised on a real multi-sweep run. Three consecutive `make sweep-39` runs confirm: (a) the aws-route53 fix is durable; (b) the rule #13 retirement doesn't regress GCP scenarios; (c) the transport classifier behaves correctly on real data (S97 was dry-run only against archived sweep-s95-3).

2. **LLM-transport retry** (S101). S97 classifies pre-iter-1 transport failures as `transport_failed` but doesn't retry them. The classification is half the work — the natural follow-up is one retry on the detected shape before recording as failed. Closes the transport-noise loop so future sustain runs converge to a single deterministic pass-count rather than "X/39 deterministic + Y/39 transport — go re-run".

Order: sustain first because it generates real transport-failure data S101 can validate against. If sustain happens to produce no transport failures, S101 still ships (the retry-shape pattern is what we're encoding; it just won't have a live trigger).

S101 owns the mandatory ARCHIVE close-out per Option C — no separate close-out slice.

## Slices

| Slice | Title | Effort |
|---|---|---|
| S100 | Three consecutive `make sweep-39` sustain runs | ~2-3 hr (mostly wallclock) |
| S101 | LLM-transport retry in `scripts/sweep_39.sh` + arc close-out | ~2 hr |

**Total**: ~4–5 hr.

## Standing rules

Inherit all rules from prior arcs (slices-54-62 through `post-sustain-tightening-plan.md`). Specifically:

- **Selective pitfall discard (S94)**: `learned_from_diff_avoid` survives sweep teardown via `bin/pitfall-merge`; `learned` + `learned_from_diff` discarded as before.
- **Transport classifier (S97)**: `sweep_39.sh` emits `TRANSPORT_FAILED=N` and rewrites `summary.tsv` rows to distinguish transport from convergence failures.
- **Mandatory close-out per Option C**: folded into S101.

## S100 — three consecutive sustain sweeps

### Motivation

Post-sustain tightening landed four behavioural changes since the last sustain-validation arc:

| Change | Slice | Validation status |
|---|---|---|
| fakeaws Route 53 list-sort + `ChangeTagsForResource` | S96 | End-to-end on aws-route53 (1 scenario, 1 run) |
| Transport-failure classifier in `sweep_39.sh` | S97 | Dry-run on archived sweep-s95-3 data only |
| GCP phase3 rule #13 retirement | S98 | End-to-end on gcp-cloud-run (1 scenario, 1 run) |
| OPA-dup ratchet for prompts | S99 | CI-only |

Each was validated narrowly. A 3-sweep sustain run validates them collectively + answers:

- Does aws-route53 hold across multiple invocations, or is the route53 fix scenario-specific?
- Does the rule #13 retirement regress any GCP scenario the spot-check missed?
- How often does Claude CLI rate-limit fire on this LLM credential, given S97's classifier as the observability?

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S100-T1 | `make mocks-restart` for a clean baseline. Run `make sweep-39` three consecutive times into distinct `SWEEP_DIR`s (`/tmp/sweep-s100-1`, `-2`, `-3`). Capture per-scenario pass/fail, N13 emissions, panic counts, AND `TRANSPORT_FAILED=N` per run. | P0 | — |
| S100-T2 | Build a comparison table per-scenario across the 3 sweeps. Flag any scenario that flapped (passed in some, failed in others). | P0 | S100-T1 |
| S100-T3 | If aws-route53 fails again: that's a regression on the fakeaws#7 fix — investigate (could be a sweep ordering issue, could be a fix that didn't cover a different LLM-generated shape). If it passes in all three: S96 confirmed durable. | P0 | S100-T2 |
| S100-T4 | If transport failures occur: validate the S97 classifier did its job (reclassified to `transport_failed`, not `repair_budget_exhausted`). Count them; if pattern is consistent across sweeps, that's the live data S101 needs. | P0 | S100-T2 |
| S100-T5 | If GCP scenarios regress on `region_restriction` policy: that's an S98 regression — the prompt rule was load-bearing after all. Restore rule #13. | P0 | S100-T2 |

### Exit criteria

- Three consecutive sweeps complete.
- Per-scenario stability documented.
- Decision recorded: aws-route53 stable / S98 regression / transport rate.

## S101 — LLM-transport retry + arc close-out

### Motivation

S97 classifies transport failures but doesn't retry. That leaves operators with the same "is X/39 a real result?" question every sustain — they have to mentally subtract the transport count. The cleaner shape is: when the sweep detects a `transport_failed` shape mid-run, retry the scenario ONCE before recording the row in `summary.tsv`. The retry is single-shot — not a loop. If the retry also transport-fails, the row stays `transport_failed`. If it converges, the row becomes `target_reached`.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S101-T1 | Modify `scripts/sweep_39.sh`'s per-scenario loop: after the initial `infrafactory run`, before writing to `summary.tsv`, inspect the scenario log. If the result matches the transport-failed shape (`terminal=repair_budget_exhausted AND dur_s<30 AND only _generate stages failed`), re-invoke `infrafactory run` once. Use the retry's result instead. | P0 | S100 (to validate against real data) |
| S101-T2 | Emit a `RETRY_TRANSPORT=N` summary line counting how many scenarios needed retries; emit `RETRY_RECOVERED=M` counting how many converged after retry. | P0 | S101-T1 |
| S101-T3 | Update AGENTS.md sweep-protocol bullet to describe the retry behaviour. Update `feedback_sweep_protocol.md` memory similarly. | P0 | S101-T1 |
| S101-T4 | One PR. **Arc close-out folded in** (STATUS + NEXT_SESSION + ARCHIVE per Option C). | P0 | S101-T1, T2, T3 |

### Exit criteria

- Sweep script retries transport-failed scenarios once.
- New `RETRY_TRANSPORT` + `RETRY_RECOVERED` summary lines.
- AGENTS.md + memory updated.
- ARCHIVE close-out for the arc lands.

## Why this order, in one paragraph

S100 first because: (a) sustain validates the four behavioural changes from the prior arc — without this, we don't know whether the recent work actually holds; (b) it generates real transport-failure data that S101 can implement against, which is much better than implementing a retry against synthetic data and hoping the shape matches reality. S101 second because the retry only matters once we know transport recurs (S100 settles that), and it's the natural closing move on the transport-noise loop S97 opened.

## Autonomous-execution loop prompt

```
/loop until both slices (S100, S101) in docs/plans/sustain-revalidate-and-transport-retry-plan.md are complete: every slice's exit criteria met, every PR merged with green CI, STATUS + NEXT_SESSION updated.

Read docs/NEXT_SESSION.md first for the prior arc's handoff, then docs/plans/sustain-revalidate-and-transport-retry-plan.md for the slice definitions. All prior standing rules apply (slices-54-62, slices-74-78, slices-79-83, slices-84-88, slices-89-93, sustain-and-n13-durability, post-sustain-tightening). S94 selective-discard rule still applies: learned_from_diff_avoid survives via bin/pitfall-merge; learned + learned_from_diff discarded.

Work slices in order S100 → S101. S101 folds the mandatory ARCHIVE + NEXT_SESSION close-out per the Option C arc shape — no separate close-out slice.

Authority: open + merge PRs with `gh pr merge <N> --squash --admin --delete-branch` once CI is green, in all four repos.

S100 exit decision matters: if sustain reveals aws-route53 still flakes, S96 was incomplete — investigate before S101. If GCP scenarios regress, S98 was wrong — restore rule #13 before S101. Otherwise proceed to S101 directly.

Stop only when: (a) both slices complete OR (b) you genuinely cannot proceed (regression in main blocks every scenario AND fix-forward isn't obvious — document the blocker in NEXT_SESSION + stop).

Before stopping for any reason, update docs/NEXT_SESSION.md with: which slices closed, which are blocked, what next session should do FIRST.
```

## Fresh-context checklist

The autonomous prompt assumes the agent reads, in order:
1. `AGENTS.md` (Option C goal-named arcs; sweep-protocol bullet covers four ratchets + transport classifier)
2. `docs/NEXT_SESSION.md`
3. This file (`docs/plans/sustain-revalidate-and-transport-retry-plan.md`)
4. `STATUS.md`
5. `docs/status/ARCHIVE.md` § "2026-06-03 post-sustain tightening" (for the four behavioural changes S100 re-validates)
6. `scripts/sweep_39.sh` (the file S101 modifies; classifier section starts at "transport classifier")
7. `docs/decisions/0015-classifier-routing.md` (only if S101 needs to amend it for the retry)
