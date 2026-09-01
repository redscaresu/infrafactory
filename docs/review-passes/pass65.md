# Codex review pass 65 — S156a

One finding, **declined** — the first decline in this slice.

## [P2] "Preserve live pitfalls for Genesys sweeps" — declined

The finding is that `scripts/sweep_39.sh` loops `aws gcp scaleway` and omits
genesys, so live entries for a supported cloud could be lost.

Checked rather than assumed:

- `pitfalls/genesys.yaml` exists and holds **19 entries** (18 descriptive, 1 fix).
- The sweep omits genesys from its **snapshot** (line 40–42), its **merge** (line
  237), and `m93_resweep.sh` does the same.
- All three corpus ratchets — `TestPitfallsSourceEnum`,
  `TestPitfallsNoHumanSeeding`, and the third in that file — also loop
  `aws / gcp / scaleway` only.

So genesys is excluded from pitfall sweep handling **consistently across the
repository**, and has been since before this slice. `LiveSource` did not create
that gap and does not widen it: in the normal path genesys entries are not merged
and therefore not touched, and in the fallback path (`bin/pitfall-merge` missing)
the blanket `git checkout pitfalls/` discards *everything* for every cloud —
avoid entries included — exactly as it always has.

**Declined as out of scope.** Bringing genesys into the sweep's pitfall handling
means also deciding whether it joins the ratchets, whether the no-human-seeding
policy applies to a corpus that predates it, and what `AVOID_EMISSIONS` means for
a cloud with no avoid entries. That is a decision with its own scope, and folding
it into a slice about retirement is how a small slice becomes S155b.

**Recorded, not dismissed**: noted in the S156 plan as a prerequisite for S156c,
which is the slice that will actually produce live entries and therefore the first
point at which this costs anything.

## Where the slice stands

Six passes, six findings, five accepted and one declined — all six downstream of
adding one constant.
