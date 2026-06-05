# Arc: sustain under the renamed vocabulary

Status: planned (2026-06-04)
Owner: next-session claude (designed for autonomous execution)
Follows: `mock-gaps-and-rename-plan.md` (closed 2026-06-04 with S102 + S103 + S104)
Shape: goal-named variable-length arc per AGENTS.md (1 slice, ~2-3 hr wallclock; mostly sweep time)

## Big picture

S104 renamed the auto-learning vocabulary atomically: `IsMockServerBug` / `ExtractFixPitfall` / `ExtractAvoidPitfall` / `ExtractDescriptivePitfall` in code, `descriptive` / `fix` / `avoid` in the YAML `source:` enum, `AVOID_EMISSIONS=N` in the sweep summary, `bin/pitfall-merge --keep avoid` in the sweep harness. Unit + integration tests covered the mechanism, and a probe sweep ran on 2026-06-04 immediately before the rename, but the rename hasn't been exercised under live sweep conditions yet.

The risk is low (atomic refactor, tests green) but the surface area is large (every emission path, the loader, the merger, the sweep harness). A 3-sweep sustain run validates:

1. **Classifier still routes correctly.** `IsMockServerBug` matches the same signals it matched as `IsMockActionable`; nothing routes to `docs/mock-gaps.md` that should have stayed a pitfall, and vice versa.
2. **Extractors still emit with new source values.** Any organic `fix` or `avoid` entry that surfaces during a sweep gets the new enum value (not the old literal).
3. **Selective-discard still preserves `avoid`.** `bin/pitfall-merge --keep avoid` correctly preserves `avoid` entries through teardown and discards `fix` + `descriptive` as noise.
4. **Sweep summary lines emit with new names.** `AVOID_EMISSIONS=N` appears; `RETRY_TRANSPORT` + `RETRY_RECOVERED` (S101) still work.
5. **No regression on the 38-39/39 deterministic baseline.**

## Standing rules

Inherit all rules from prior arcs (slices-54-62 through `mock-gaps-and-rename-plan.md`):

- **Selective pitfall discard** (post-S104): `avoid` survives via `bin/pitfall-merge`; `fix` + `descriptive` discarded as before.
- **Transport retry**: `scripts/sweep_39.sh` retries `transport_failed` shape once before recording (S101 behavior).
- **Fix at source**: mock bugs go to fakeaws/fakegcp/mockway, never hand-edit `pitfalls/*.yaml`.
- **Mandatory close-out per Option C**: folded into the single slice.

## Slice

| Slice | Title | Effort |
|---|---|---|
| S105 | 3 consecutive `make sweep-39` runs under renamed vocab + close-out | ~2-3 hr (mostly wallclock) |

## S105 — three consecutive sustain sweeps + close-out

### Motivation

The rename was atomic and tests pass. This slice confirms the rename holds under live sweep conditions. If 3 sweeps come back clean, the rename is durable and the loop can move on. If any signal fails to emit or any scenario flaps, that's a rename regression — fix-forward in-slice.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S105-T1 | `make mocks-restart` for a clean baseline. Run `make sweep-39` three consecutive times into distinct `SWEEP_DIR`s (`/tmp/sweep-s105-1`, `-2`, `-3`). Capture per-scenario pass/fail, `AVOID_EMISSIONS=N` per run, panic counts, `RETRY_TRANSPORT=N` + `RETRY_RECOVERED=M` per run. | P0 | — |
| S105-T2 | After each sweep, grep the post-merge `pitfalls/*.yaml` for the old source-enum literals (`learned\|learned_from_diff\|learned_from_diff_avoid`). Any hit is a rename regression — the writer or loader still produces the old values. | P0 | T1 |
| S105-T3 | After each sweep, grep the master log for the old summary-line names (`N13_EMISSIONS\|learned_from_diff\* lines`). Any hit is a sweep-harness rename regression. | P0 | T1 |
| S105-T4 | Build a comparison table per-scenario across the 3 sweeps. Flag any scenario that flapped. | P0 | T1 |
| S105-T5 | If `avoid` entries surface organically: confirm they get preserved through `bin/pitfall-merge` (cross-reference pre vs post YAML diffs). If `AVOID_EMISSIONS=0` across all 3 sweeps, that's a soft watchdog — note it but don't treat as failure (S100 showed N13 can stay silent across sustain cycles). | P0 | T1 |
| S105-T6 | If any rename regression surfaces (T2 / T3 / T5): fix-forward in-slice. Re-run the failing sweep stage to confirm. | P0 | T2-T5 |
| S105-T7 | One PR. **Arc close-out folded in** (STATUS + NEXT_SESSION + ARCHIVE per Option C). | P0 | T1-T6 |

### Exit criteria

- Three consecutive sweeps complete (any combination of 38/39 or 39/39 deterministic, transport tail allowed).
- Zero hits for old source-enum literals in post-merge `pitfalls/*.yaml`.
- Zero hits for old summary-line names in any sweep master log.
- Per-scenario stability documented; flapping scenarios investigated (if any).
- ARCHIVE close-out for the arc lands.

## Why this shape

Single slice because the work is one logical action (3 sweeps under a fixed configuration) and the verification is mechanical (grep for old literals; cross-reference YAML diffs). Splitting into two slices would force an artificial boundary mid-validation. If a regression surfaces, fix-forward IN the slice — the slice's PR carries both the sweep evidence AND the fix together, which is the right granularity for the auditor.

If the work expands beyond fix-forward (e.g. a deeper structural problem with the rename surfaces and needs a re-architect), spawn a separate arc and close this one out as "validated except X".

## Autonomous-execution loop prompt

```
/loop until S105 in docs/plans/sustain-under-renamed-vocab-plan.md is complete: exit criteria met, PR merged with green CI, STATUS + NEXT_SESSION updated.

Read docs/NEXT_SESSION.md first for the prior arc's handoff, then docs/plans/sustain-under-renamed-vocab-plan.md for the slice definition. All prior standing rules apply (slices-54-62 through mock-gaps-and-rename). Standing rule (post-S104): `avoid` survives via bin/pitfall-merge; `fix` + `descriptive` discarded as sweep noise.

S105 folds the mandatory ARCHIVE + NEXT_SESSION close-out per the Option C arc shape — no separate close-out slice.

Authority: open + merge PRs with `gh pr merge <N> --squash --admin --delete-branch` once CI is green.

Exit decision matters: if any of the three sweeps shows an old source-enum literal (`learned`, `learned_from_diff`, `learned_from_diff_avoid`) in post-merge pitfalls/*.yaml, or an old summary-line name (`N13_EMISSIONS`, `learned_from_diff* lines`) in the master log, that's a rename regression — fix-forward in-slice before closing out. Otherwise proceed to close-out directly.

Stop only when: (a) S105 complete OR (b) you genuinely cannot proceed (document the blocker in NEXT_SESSION + stop).

Before stopping for any reason, update docs/NEXT_SESSION.md with: what landed, what's blocked, what next session should do FIRST.
```

## Fresh-context checklist

The autonomous prompt assumes the agent reads, in order:
1. `AGENTS.md` (Option C goal-named arcs; sweep-protocol bullet — already in the renamed vocabulary)
2. `docs/NEXT_SESSION.md`
3. This file (`docs/plans/sustain-under-renamed-vocab-plan.md`)
4. `STATUS.md`
5. `docs/status/ARCHIVE.md` § "2026-06-04 mock-gaps drain + learning-system rename" (the rename specifics S105 validates)
6. `docs/decisions/0019-learning-system-vocabulary.md` (the ADR that codifies what the rename was)
7. `scripts/sweep_39.sh` (the file S105 invokes; verifies `--keep avoid` + `AVOID_EMISSIONS=N` text)
8. `docs/auto-learning-loop.md` (the explainer; written in the renamed vocabulary)
