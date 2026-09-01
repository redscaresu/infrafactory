# Codex review pass 86 — S156c

Two findings, both accepted.

## [P2] Promotion grouped across clouds, and the filter came too late

`live learn` ran the gate over **every** deployment and then refused any
candidate whose deployments disagreed on a cloud. That reads as caution and is
the opposite:

- A Scaleway deployment that met the threshold **on its own** was dropped because
  a GCP deployment happened to observe the same words. Sufficient evidence
  discarded by unrelated evidence.
- Worse in the other direction: `--deployments 2` counted breadth **across**
  clouds, so one observation on each of two clouds promoted — a coincidence of
  wording that is a fact about neither of them.

The corpus is per-cloud. The cloud is therefore part of what makes an observation
*the same observation*, exactly as its status is, so the partition belongs before
the gate rather than after it. Promotion now runs per cloud and `agreedValue` on
clouds is gone.

`TestLearnRefusesWhenDeploymentsDisagreeOnTheCloud` **encoded the bug**: it
seeded one observation on each of two clouds and relied on cross-cloud breadth to
promote so it could then watch the filter reject it. It now asserts the rule that
should have been there — one apiece is not reproduction — alongside a new test
that a cloud reproducing on its own is never suppressed by another.

Records that name no cloud are reported by count rather than dropped.

## [P2] The first live write could not create its own corpus

A reader treats a missing pitfalls file as an empty corpus, so writing the first
lesson into a fresh corpus is a normal thing to do. `AppendLivePitfall` went
straight to `writePitfallsFile`, whose `os.CreateTemp(pitfallsDir, …)` fails with
ENOENT if the directory is not there.

`AppendPitfall` had its own `MkdirAll`, which is exactly why this was missed: the
obligation lived in the callers, so a new caller could simply not know about it.
Moved into `writePitfallsFile`, which is the function that actually needs the
directory — the caller's copy is gone rather than duplicated.

## Nothing declined this pass.
