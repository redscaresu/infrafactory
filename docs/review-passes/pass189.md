# Review pass 189 — S186 `run --keep`

`codex exec review --base main`, 2026-09-20.

## Findings

### [P1] Destroy failed keep iterations before retrying — ACCEPTED

> When `run --keep` has a failing iteration after the sandbox apply succeeded (for
> example a real probe or criteria failure), this branch skips the real destroy even
> though the run has not reached `target_reached` yet. The repair loop can then start
> another iteration, and the generation path removes/regenerates the same output
> directory, so the state for the still-running resources from the failed iteration
> can be overwritten before `registerKeptRun` ever records anything.

Correct, and worse than "untracked": `writeGeneratedFiles` does `os.RemoveAll(outputDir)`
on a clean run, which takes the live state **and** the run-project marker. The
resources from that iteration would have had no state to destroy them from and no
project id naming them — strictly worse than the `--no-destroy` leak `--keep` was
added to replace.

**Both halves of the claim were verified in the code before the fix**, not inferred
from the description: that criteria failures land in `failures` before the destroy
decision (`test_command.go`, the `evaluateSupportedCriteria` append), and that
generation wipes the directory (`generate_command.go`, the `default:` arm of
`writeGeneratedFiles`). This project has shipped three confident fixes for a
misdiagnosed cause in one day before.

Fixed by making `--keep` mean what it says — keep a *successful* stack.
`keepingSandbox = opts.KeepSandbox && len(failures) == 0`, read as late as possible
so it accounts for the criteria checks and the mock destroy. That lines up with the
run loop by construction rather than coincidence: the loop registers only on
`target_reached`, which is exactly an iteration that ended with no failures, so the
iteration that skips its destroy is the iteration that gets registered.

The run-project switch reads the same variable, so the project and the stack cannot
disagree about whether this run kept anything.

`TestKeepStillDestroysAnIterationThatFailed` drives an apply that succeeds and a
probe that fails, and asserts the sandbox destroy still ran once. Mutating the gate
back to `opts.KeepSandbox` fails it.

## Note on the previous pass

Pass 188's [P2] (keep failures not reaching the run status) is fixed and covered;
this pass raised no repeat of it.
