# Review pass 191 — S186 `run --keep`

`codex exec review --base main`, 2026-09-20.

> The changes add --keep handling across CLI, API, and UI with appropriate validation,
> registration, teardown-skipping behavior, and focused tests. I did not find a
> discrete correctness issue introduced by the patch.

Clean. Converged after three passes: pass 188 [P2] (keep failures not reaching the
run status), pass 189 [P1] (failing iterations skipping their destroy), pass 190
[P1] (holdouts re-applying over a kept stack). All three accepted and fixed; none
were nits, and none were declined.

All three had the same shape — **the keep was correct for the path it was written
for and wrong for a path that reaches the same code.** A failing iteration, a
holdout, and a copy that failed all arrive at the keep from outside the happy path
that was in mind while writing it.
