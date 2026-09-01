# Codex review pass 54 — S155b `live upgrade`

One finding, accepted. **The incomplete half of pass 53's fix.**

## [P1] The marker was preferred, not required

Pass 53 made the upgrade take its project from the marker — but fell back to the
deployment record when the marker could not be read. That leaves the **editable**
half deciding where real infrastructure gets applied, which is the hole the marker
exists to close. A guard that degrades to the thing it was guarding against is not
a guard.

Required now. The distinction from `live teardown`, which *does* fall back, is
deliberate and written down:

| | teardown | upgrade |
|---|---|---|
| refusing strands something | yes — a pre-cutover record whose resources are real | no — tear down and deploy again |
| bounded by existing state | yes, destroy acts on its own workdir's state | no, an apply creates |

Neither argument for falling back holds when applying.

## The pattern in this slice

Five passes, ten findings, and the through-line is not carelessness in ten places
— it is **one fix applied incompletely, repeatedly**:

- passes 50–52: "did anything reach the cloud?" answered for the tag, then the
  address, then the workdir, then the ordering
- passes 53–54: "which project do we trust?" answered by preferring the marker,
  then by requiring it

Both converged only when the answer became a single mechanism rather than a
series of correct-looking local patches. Worth remembering: under a one-clean-pass
rule the second pass is not a formality, it is where a half-applied fix gets
caught.

## Nothing declined this pass.
