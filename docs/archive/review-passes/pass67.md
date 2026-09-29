# Codex review pass 67 — S156a

**Clean.** No findings.

> The reviewed changes add live pitfall retention, CLI wiring, and sweep
> preservation without introducing a correctness issue I could identify.

Converged. Eight passes, eight findings, seven accepted and one declined.

## What the slice was actually about

`LiveSource = "live"` is one constant. Everything else was the places that had to
know:

| pass | who had to learn about `live` |
|---|---|
| 59 | the two corpus ratchets, and the sweep's preservation list |
| 60 | the merge's handling of freshness |
| 61 | the runtime wiring (maintenance must not need the LLM) |
| 62 | the path guard on the new command |
| 63 | the merge's source semantics |
| 64 | the metric reading the merge's output |
| 65 | *(declined)* the sweep's cloud coverage |
| 66 | the merge's identity for entries carrying state |

Four of those were mechanisms that **already existed in this repository** —
ratchets, preservation, path validation, dedup identity. None was found by
reading the new code; each was found by a reviewer asking who else cares.

**A vocabulary is only as real as the things that enforce it.** ADR-0019 now says
so, and says the next value added here should start by enumerating the readers
rather than by adding the constant.

## Passes 63 and 66 are worth reading together

63 fixed a bug by preserving historical behaviour; 66 showed that the historical
behaviour was itself wrong for the new case. Both were right on the evidence
available at the time. The general lesson is not "think harder" — it is that
*"keep the old behaviour"* is a decision, not a default, and deserves the same
justification as changing it.
