# Review pass 190 — S186 `run --keep`

`codex exec review --base main`, 2026-09-20.

## Findings

### [P1] Preserve kept state before running holdouts — ACCEPTED

> When `--keep` is used on a scenario that has criteria-only holdouts, this skips the
> real destroy for the training iteration, but `runRunCommand` still runs
> `runCriteriaOnlyHoldouts` afterward on the same `runtime.OutputDir()` without
> `KeepSandbox`. [...] it can destroy or corrupt the live state before
> `registerKeptRun` copies and records it.

Correct. `runCriteriaOnlyHoldouts` calls `executeTestWithScenario(ctx, runtime, sc,
runtime.OutputDir(), ...)` — the same directory, deliberately, because a criteria-only
holdout validates the training scenario's generated code. With Layer 3 on that means a
second `ensureRunProject`, a marker overwrite, an apply over the kept stack's state and
a destroy. The kept stack would be torn down and the record would name the holdout's
project.

**Fixed by skipping holdouts under `--keep`, with a stage that says so.** Not by
threading `KeepSandbox` through: the holdout does not merely destroy, it *re-applies
over this run's state in this run's directory*, and keeping a stack while re-applying
over its state cannot both happen. Suppressing only the destroy half would leave a
worse state than either.

There is direct precedent one line away — `--no-destroy` already skips holdouts for
the same incompatibility, and has since the flag existed. `--keep` now joins it in a
`switch` rather than a third `else if`, so the three outcomes are visibly exhaustive.

Cost, stated plainly: a `--keep` run does not get the holdout check. That is a real
reduction in what the run proves, which is why it is a visible skip stage and not a
silent omission — the same rule the rest of this project follows about checks that
did not run. `run` without `--keep` is unchanged.

`TestRunKeepLeavesTheStackUpAndRegistersIt` now asserts the skip and its reason;
deleting the `case controls.Keep:` arm fails it.

## Note on scope: no holdout is discovered today

`scenarios/holdout/` holds one file, `web-app-paris-pinned.yaml`, and
`DiscoverCriteriaOnlyHoldouts` never returns it: discovery requires `type: holdout`
AND `references: <training scenario path>`, and that file declares **neither**. Every
run currently logs `holdout/discovery: pass (0 holdouts)`.

So the cost of skipping holdouts under `--keep` is presently zero — it skips nothing
that runs. The guard is still right: the day someone adds those two keys to a
keepable scenario, the holdout would re-apply over the kept stack's state and destroy
it, and nothing would have warned them.

Worth recording separately that the holdout machinery is built and unwired. Passing
the training criteria proves only that the generator satisfied the checks it was
shown; the holdout is the half that tests otherwise, and it is not running.
