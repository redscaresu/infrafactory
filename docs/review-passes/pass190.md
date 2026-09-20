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

## Note on scope

`scenarios/holdout/web-app-paris-pinned.yaml` is a holdout for `web-app-paris`, not
`web-live-paris`, so the demo scenario never hit this. It would have bitten the first
person to add a holdout for a keepable scenario, and silently.
