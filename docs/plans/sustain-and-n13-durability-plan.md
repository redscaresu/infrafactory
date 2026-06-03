# Arc: 39/39 sustain validation + N13 durability

Status: planned (2026-06-03)
Owner: next-session claude (designed for autonomous execution)
Follows: `slices-89-93-plan.md` (closed 2026-06-03 with 🎯 39/39 first deterministic + Option C scaffold decision)
Shape: goal-named variable-length arc per AGENTS.md "Planning a New Arc" (2 slices, ~3–5 hours)

## Big picture

S90 hit 39/39 once. That proves it CAN happen, not that it RELIABLY happens. We also have a latent design tension: N13 (`ExtractPrescriptiveAvoid`) writes `learned_from_diff_avoid` entries to `pitfalls/<cloud>.yaml` when a scenario converges via a deletion-as-fix — but the sweep protocol discards all pitfall additions at sweep end (`git checkout pitfalls/`), so N13's high-signal output never lands durably. The result: N13 has been running invisibly across multiple arcs.

This arc fixes the durability gap first, then runs three consecutive sweeps to (a) validate 39/39 sustains and (b) give N13 a chance to emit and stick.

The order matters: do the protocol change first so the sustain sweeps exercise both questions at once.

## Slices

| Slice | Title | Effort |
|---|---|---|
| S94 | N13 durability — selective discard in sweep protocol + watchdog | ~1.5-2 hr |
| S95 | 3 consecutive sustain sweeps + arc close-out | ~2-3 hr |

**Total**: ~3.5–5 focused hours.

## Standing rules

Inherit all rules from `slices-54-62-plan.md`, `slices-74-78-plan.md`, `slices-79-83-plan.md`, `slices-84-88-plan.md`, and `slices-89-93-plan.md`. Same merge authority, mock rebuild discipline, scope per PR. **Exception**: the "discard pitfall additions" rule changes in S94. Until S94 lands, the old rule applies.

The Option C arc shape: **mandatory close-out folded into S95** (no separate close-out slice, per AGENTS.md "no padding").

## S94 — N13 durability + watchdog

### Motivation

Three observations across the past 4 arcs:

1. N13 fires on `target_reached` after a deletion-as-fix and writes `source: learned_from_diff_avoid` to `pitfalls/<cloud>.yaml`. The S81 sweep emitted one (`google_storage_bucket` from gcp-storage); the S85 validation run emitted one (`google_storage_bucket` from gcp-full-stack). Both were discarded.
2. The S81 carve-out validation in pitfalls/gcp.yaml IS `source: learned` (from `ExtractLearnedPitfall`, not N13) — the carve-out routes the failure through stuck termination, not through organic deletion-as-fix.
3. `feedback_sweep_protocol.md` says "never hand-edit pitfalls" and "discard sweep pollution." The discard was designed against `learned` (which can be wrong about *what* the LLM should change) and `learned_from_diff` (which is a guess about what addition cleared the failure). **N13 is different**: it fires only on a confirmed deletion that DID clear the failure on the next iteration. The output is grounded in a successful run, not a guess.

The fix: keep `learned_from_diff_avoid` entries when their parent scenario converged; discard `learned` and `learned_from_diff` entries as today.

The watchdog: warn (don't fail) if the sweep emits zero `learned_from_diff_avoid` adds across 3+ consecutive sweeps. Catches both "N13 is broken in code" and "the LLM has stopped making recoverable-by-deletion mistakes" — both worth knowing, neither necessarily a regression.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S94-T1 | Modify `scripts/sweep_39.sh`: replace the blanket `git checkout pitfalls/` with a selective restore that KEEPS lines added with `source: learned_from_diff_avoid` and discards `learned` + `learned_from_diff` additions. Trickier than it looks — YAML is structured, can't just grep. Approach: pre-sweep snapshot, parse pre-sweep YAML, parse post-sweep YAML, write a merged YAML that's pre-sweep PLUS only the new `learned_from_diff_avoid` entries. | P0 | — |
| S94-T2 | Add a static ratchet test (`internal/generator/pitfalls_n13_visibility_test.go`) that documents the expected presence of `learned_from_diff_avoid` entries. **Initially passes with zero such entries** (correct — none committed yet); the test's purpose is to flag if a contributor accidentally deletes one in the future once they exist. Schema-level check: every entry's `source` field must be in the allowed enum (`learned`, `learned_from_diff`, `learned_from_diff_avoid`). | P0 | — |
| S94-T3 | Sweep-side watchdog: in `scripts/sweep_39.sh`, after the sweep, count `learned_from_diff_avoid` additions. Emit a "N13_EMISSIONS=N" line to stdout + summary. If 0, print a warning but DO NOT fail the script. (Pattern-match the existing PANIC_LINES output for parity.) | P1 | S94-T1 |
| S94-T4 | Update `feedback_sweep_protocol.md` memory + `AGENTS.md` operational caveat: document the selective-discard rule. Explicit: `learned_from_diff_avoid` survives sweeps; the other two sources are still discarded. | P0 | S94-T1 |
| S94-T5 | One PR. Tests + script change + memory update. | P0 | S94-T1, T2, T3, T4 |

### Exit criteria

- `scripts/sweep_39.sh` selectively preserves `learned_from_diff_avoid` entries.
- Static ratchet test lands (initially zero-N13-entries-OK).
- Sweep-side watchdog emits N13_EMISSIONS=N.
- Sweep-protocol memory + AGENTS.md updated.

## S95 — 3 consecutive sustain sweeps + arc close-out

### Motivation

Three sweeps in a row gives signal on stability + N13 emergence. Expected outcomes:

| Pass count pattern | Interpretation |
|---|---|
| 39 / 39 / 39 | Deterministic baseline holds. Done. |
| 39 / 38 / 39 (or similar single-flake) | One scenario flakes; characterize which + how often. Flake budget defined. |
| 38 / 37 / 38 (multiple shifts) | More fragile than S90 suggested; the 39/39 was probably luck. New floor characterized. |
| Any consistent regression from S90 | Mock/code change since S90 broke something. Bisect. |

Plus a separate signal on N13: if even one sweep emits a `learned_from_diff_avoid` entry and S94's protocol change keeps it, the durability claim is validated end-to-end.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S95-T1 | `make mocks-restart` (clean baseline) → run `make sweep-39` three consecutive times. Each into a distinct `SWEEP_DIR` (`/tmp/sweep-s95-1`, `-2`, `-3`). Capture pass counts, panic counts, and N13 emission counts per run. | P0 | S94 |
| S95-T2 | If N13 emitted at least once: verify the entry is durably in `pitfalls/<cloud>.yaml` after sweep teardown (i.e. S94's selective-discard worked). Add it to the commit. | P0 | S95-T1 |
| S95-T3 | Build a comparison table: per-scenario pass/fail across the 3 sweeps. Flag any scenario that flapped. | P0 | S95-T1 |
| S95-T4 | If any scenario flapped or regressed from S90: investigate root cause. Decide: (a) re-baseline at the new floor, (b) fix the flake at source if quick, (c) document as a known-flake. | P1 | S95-T3 |
| S95-T5 | **Arc close-out (mandatory per Option C)**: STATUS + NEXT_SESSION update. `docs/status/ARCHIVE.md` § "2026-06-03 sustain + N13 durability arc" with: per-sweep pass counts, N13 emergence summary, flake characterization (if any). Commit any durably-learned N13 pitfalls. | P0 | S95-T2, T3, T4 |

### Exit criteria

- 3 consecutive sweeps complete; pass-count pattern documented.
- Sustainability claim made explicit: either "39/39 holds" or "stable floor is N/39 with these flakes."
- N13 durability validated (entry observed → committed) OR documented as "didn't fire this arc; watchdog active."
- ARCHIVE close-out narrative committed.

## Why this order

S94 first because if we run the sustain sweeps under the OLD protocol, any N13 emissions get discarded and we lose the chance to observe the durability claim. S95 second so its sweeps exercise the new protocol end-to-end.

## Autonomous-execution loop prompt

```
/loop until both slices (S94, S95) in docs/plans/sustain-and-n13-durability-plan.md are complete: every slice's exit criteria met, every PR merged with green CI, STATUS + NEXT_SESSION updated.

Read docs/NEXT_SESSION.md first for the prior arc's handoff, then docs/plans/sustain-and-n13-durability-plan.md for the slice definitions. The standing rules from slices-54-62-plan.md, slices-74-78-plan.md, slices-79-83-plan.md, slices-84-88-plan.md, AND slices-89-93-plan.md apply, EXCEPT the new selective-discard rule from S94 supersedes the blanket pitfall discard once S94 lands.

Work slices in order S94 → S95. S95 folds the mandatory ARCHIVE + NEXT_SESSION close-out per the Option C arc shape — no separate close-out slice.

Authority: open + merge PRs with `gh pr merge <N> --squash --admin --delete-branch` once CI is green. Discard `learned` + `learned_from_diff` sweep pollution as before; KEEP `learned_from_diff_avoid` per S94's protocol change.

Stop only when: (a) both slices complete OR (b) you genuinely cannot proceed (regression in main blocks every scenario AND fix-forward isn't obvious — document the blocker in NEXT_SESSION + stop).

Before stopping for any reason, update docs/NEXT_SESSION.md with: which slices closed, which are blocked, what next session should do FIRST.
```

## Fresh-context checklist

The autonomous prompt assumes the agent reads, in order:
1. `AGENTS.md` (note: now reflects Option C goal-named variable-length arcs)
2. `docs/NEXT_SESSION.md`
3. This file (`docs/plans/sustain-and-n13-durability-plan.md`)
4. `STATUS.md`
5. `docs/status/ARCHIVE.md` § "2026-06-03 S89–S93" (for the 39/39 baseline that S95 validates)
6. `internal/generator/prescriptive_extractor.go` (the N13 implementation)
7. `feedback_sweep_protocol.md` memory (the rule that S94 amends)
8. `scripts/sweep_39.sh` (the file S94 modifies)
