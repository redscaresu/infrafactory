# Codex review pass 87 — S156c

**Clean.** No correctness, safety, or maintainability findings in the diff.

S156c converged in four passes (77, 85, 86, 87), and three of the four were the
**same question asked at successively wider scope**: what is the identity of a
live lesson?

- **77** — not the rule text, because a live rule states its evidence and the
  evidence grows.
- **85** — not the detail alone either, because the gate keeps `unhealthy` apart
  from `unreachable` and health apart from version drift.
- **86** — and the cloud belongs in it as well, because the corpus is per-cloud;
  filtering afterwards both discarded sufficient evidence and counted breadth
  across clouds.

Each answer was correct and incomplete, and each incompleteness was visible from
the one before it. The generalisable move, and the one that finally ended it, was
to stop **restating** the identity at the persistence layer and make it
`Candidate.Key()` — derived by the thing that defines it. The pass-86 `MkdirAll`
finding is the same shape in miniature: an obligation held by callers is an
obligation a new caller can fail to know about.
