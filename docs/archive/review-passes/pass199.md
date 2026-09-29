# Review passes 199–203 — S192 live holdouts

`codex exec review --base main`, 2026-09-20. Five findings, all accepted, converged clean.

## Findings

**[P2] Keep holdout references compatible with the schema.** Matching moved to the
scenario name; `scenario.schema.json` still documented `references` as a path. A holdout
authored from the schema would be silently ignored. Schema corrected. (A first attempt
round-tripped the JSON through Python and reformatted all 401 lines — reverted and done
as a surgical string edit.)

**[P2] Don't skip holdouts when destroy is skipped.** `--holdout` combined with
`--no-destroy` never ran, because the block sat inside the destroy branch — while the
flag was still accepted and the run still reported `target_reached`. Extracted
`holdoutAfterCriteria` and called it from **both** criteria paths, so the two cannot
drift.

**[P2] Don't mark empty holdout sets as passed.** A scenario with no holdout files
produced a clean discovery and no failures, and `deploy` recorded `HoldoutPass` — the
estate page would say "unseen checks passed" for a deployment nothing probed. `runHoldouts`
now reports whether a check actually ran, counting CHECKS rather than files.

**[P2] Probe holdout DNS with the training scenario name.** `RealProbeHarness` substitutes
`{{scenario_name}}`, and the records belong to the stack the training scenario built, so
passing the holdout's own name resolved `web-live-paris-unseen.example.com` for a record
called `web-live-paris.example.com`.

**[P2] Don't silently ignore `--holdout` without Layer 3.** Refused before generation now,
like `--keep`, rather than accepted and skipped.

## Three of five were one bug class

*A holdout that did not run being indistinguishable from one that passed.* Recording a
pass for an empty set; keying the estate column off the request rather than the result;
accepting the flag without Layer 3 and skipping. I designed a three-state badge
specifically to prevent that reading, then reintroduced it three times in the plumbing
underneath it.

## What the tests caught that review did not

A botched edit **deleted the Layer 2 mock destroy**. Three tests failed immediately.

## A false-clean I reported and had to retract

An early mutation run showed "6 failing / 7 failing" and read as good coverage. Those were
the three broken tests, not the mutants. Re-run against an honest base:

| mutant | first (dirty base) | honest |
|---|---|---|
| empty holdout set reports as run | "6 failing" | **survived** |
| holdout runs against a non-serving stack | — | **survived** |
| discovery counted as a check | "7 failing" | caught |

Both survivors needed new tests. **A mutation count is only evidence if the base is
green** — and separately I read a `grep -c` returning `1` as clean and said so out loud.

A third mutant also survived at first for a different reason: the test called
`holdoutAfterCriteria` directly, so deleting a CALL SITE could not fail it. Rewritten to
drive `executeTest`.
