# Review passes 173–179 — the teardown fix, and four things around it

`codex exec review --base main` on `fix/poweroff-before-destroy`, seven passes.
**Eight findings: seven accepted, one declined.** Three were bugs the change
itself introduced.

**No final confirming clean pass.** Codex hit its usage limit before the last
run completed, so the pass after the final fix (skipping plural allowlist
refusals) never returned a verdict. Every finding raised has been addressed and
the suite is green, but a clean pass is *not* claimed. Re-running it is
outstanding work, recorded here rather than glossed.

## Accepted

**[P1] the poweroff authorised against the wrong directory.** It re-checked
provenance with `runtime.OutputDir()` while `destroySandbox` had been handed a
`workDir`. `live teardown` and `live reap` pass the *deployment's* directory and
need not have loaded a scenario at all, so the guard would have read an empty or
unrelated state file and silently skipped the poweroff — on exactly the paths
that tear down long-lived infrastructure.

**[P2] the gate taught a refuted remedy.** Recording refusals verbatim is only
safe if the refusal is true, and the NIC message still ended *"...which is
removed with the server"* — the half ADR-0029's refutation killed. Fixed at the
source, which is the argument for verbatim recording working in both directions:
correct the enforcing code and every derived pitfall corrects with it.

**[P2] the allowlist refusal was not recognised.** There are two Layer 3
refusal prefixes and the extractor matched one. `layer 3 refuses to apply
resource type(s) …` is the *more* common class — reaching for a type nobody
budgeted for is the easiest mistake to make — and it was being dropped whole.

**[P2] the success path never learned.** Gate extraction was wired only into the
terminal stuck/budget harvest. A refusal in iteration 1 that iteration 2 fixes
reaches `target_reached`, skips the harvest, and discards the best signal in the
run *precisely when the loop worked*. Mirrored into the self-correction hook.

**[P2] a false green in my own stage.** A server reported as `did NOT reach
stopped` was rendered inside a **passing** stage headed "powered off N
instance(s)" — asserting the opposite of the truth, in the case where the
destroy is about to fail. The stage now fails, and the marker is a shared
constant so the two sides cannot drift.

**[P2] refusals that name no resource type cannot be filed.** Several named only
the block label ("web sets no server_id"). Now every resource-specific refusal
names the type.

**[P2] plural allowlist refusals.** `A, B: not in allow_resource_types` would
file one rule, naming both, under whichever came first. Skipped rather than
mis-filed — the same rule already applied to refusals naming no type: a pitfall
offered under the wrong key is worse than one not recorded.

## Declined, with the reason

**[P2] a failed poweroff stage does not fail the command.** Correct, and
intended. Whether the account is clean is decided by the destroy and then by
`ScalewayOrphanSweep`, which fails closed — the division the purge already uses.
A server that would not stop while the destroy nonetheless succeeded means the
teardown worked and the poweroff was moot; failing there would be a false
negative. This path exists to remove false statements in *both* directions, not
to trade one for the other. Written into the code, not just declined here.

## An audit that was removed

A source-scanning test for "every refusal names a resource type" flagged correct
code: `name` means the block label in some functions and an *attribute* name in
others (`pn_id indexes …`). Replaced with concrete message assertions. A check
that fires on correct code is a check somebody deletes.

## Mutation checks

| mutation | result |
|---|---|
| poweroff skips the wait for `stopped` | `TestInstancePowerOffWaitsForStopped` fails |
| poweroff trusts the project query parameter | `…IgnoresServersOutsideTheProject` fails |
| guard reads `runtime.OutputDir()` again | `TestPowerOffAuthorisesAgainstTheCallersWorkDir` fails |
| `IsStuck` back to previous-iteration only | oscillation test fails |
| harvest skips `ExtractGatePitfall` | `TestRunCommandLearnsAGatePitfallFromARefusal` fails |
| poweroff stage always passes | `TestPowerOffStageFailsWhenAServerNeverStopped` fails |

In mockway (#27), both directions: dropping the state check fails the contract
test, and refusing unconditionally fails it too.
