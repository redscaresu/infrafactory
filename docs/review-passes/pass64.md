# Codex review pass 64 — S156a

One finding, accepted. **The fourth pass to land on the same change, from a new
direction: not the merge, but what reads its output.**

## [P2] Live preservation could mask an avoid-learning regression

`scripts/sweep_39.sh` parses `kept_new=N` into `AVOID_EMISSIONS`, a ratchet on
whether the avoid extractor still works — it warns when a sweep emits zero.

Passing `--keep avoid,live` made `kept_new` count both. So a sweep that preserved
three live entries and produced **zero** avoid pitfalls would report
`AVOID_EMISSIONS=3` and warn about nothing. A metric that reads healthy while the
thing it measures is broken is worse than no metric.

`pitfall-merge` now prints per-source counts (`kept_avoid=2 kept_live=1`), and the
sweep ratchets on `kept_avoid`.

## What this slice has actually been about

Five passes, five findings, all downstream of adding **one constant**:

| pass | who had to learn about `live` |
|---|---|
| 59 | the two corpus ratchets; the sweep's preservation list |
| 60 | the merge's freshness handling |
| 62 | the path guard on the new command |
| 63 | the merge's source semantics |
| 64 | the metric that reads the merge's output |

`LiveSource = "live"` is three lines. Everything since has been the places that
had to know. **A vocabulary is only as real as the things that enforce it** — and
the readers are found by enumerating them, not by waiting for a reviewer to name
them one at a time. That is now written into ADR-0019 for whoever adds the next
source value.

## Nothing declined this pass.
