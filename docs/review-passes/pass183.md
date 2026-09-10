# Review passes 183+ — the endpoint, and the fix for it

`codex exec review --base main` on `fix/default-vpc-is-not-an-orphan`.

## Findings, both accepted

**[P2] a list failure looked like an empty project.** `ScalewayPrivateNICDetach`
swallowed list errors with a bare `continue`, so an auth failure or a changed
route returned "nothing to detach" — and the caller then logged *"no private
NICs to remove"* before walking into a destroy that was about to fail for
exactly that reason.

That is the failure mode this entire arc was about, reappearing inside the fix
for it. Silence that reads as success is what cost two days: a poweroff that
reported nothing, a stage nobody persisted, a mock that answered `7 added, 7
destroyed`. List failures are now reported as `could NOT be deleted` entries.

**[P2] the Layer 2 stage always passed.** A NIC that could not be deleted was
recorded in the detail while the stage said `pass` and the summary said
"removed N". Same false green as the Layer 3 stage had, in the copy of it.

## Not raised, and worth stating

The diff **deletes** `ScalewayInstancePowerOff`, its settle-retry and their
tests. That is the second and third wrong answers being removed along with the
machinery built on them — keeping the code would have kept the theory alive in a
form future readers would have taken for evidence.

## What the review could not have caught

Neither of the two real causes was findable by reading this diff: that v2alpha1
refuses every NIC, and that mockway does not embed `private_nics` on the server
object. Both needed the API to be asked. Six review passes across the day found
eight defects in the *code*; the defect in the *belief* took two `curl` calls.
Worth remembering when weighing another review pass against another experiment.
