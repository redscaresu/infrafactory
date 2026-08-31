# Codex review pass 32 — S166+S167 cutover

`codex exec review --base main` on `s166-cutover` at `0d08a8d`.

Two findings, both accepted, both P1. Both are the same question the cutover
kept failing to ask: **`tofu destroy` used to do this — who does it now?**

## [P1] D6 came straight back, from the other direction

Taking the project out of Terraform moved the failure that defines this arc.

`destroySandbox` purges and retries because `tofu destroy` could not delete a
project that still contained Scaleway's auto-created "Default security group" —
`precondition failed: resource is still in use`. That purge is triggered **by
destroy failing**.

After the cutover, destroy *succeeds*: the project is not its resource. The 412
lands on `releaseRunProject`'s Account API delete instead, which had no purge
and no retry. So every run declaring compute would leak a project again — and
leak it the same way it did the first time, invisibly: nothing billable
survives, so cost checks keep reporting clean.

Fixed: `releaseRunProject` purges and retries once, reporting what it removed.
The reporting is the part that distinguishes a fix from a coincidence — the
first D6 verification passed with the purge firing silently. Covered by
`TestReleaseRunProjectPurgesTheAutoCreatedBlockerAndRetries` and
`TestReleaseRunProjectReportsTheOriginalErrorWhenNothingWasAutoCreated`.

While here, the guard moved **inside** `releaseRunProject`. Four paths reach it,
it deletes a real project over HTTP with Terraform nowhere in the loop, and
`destroy_retry.go` already states the doctrine: a check that can be forgotten
will be. The now-redundant call site guard in `live_teardown` was removed.
Covered by `TestReleaseRunProjectRefusesAProjectNoMarkerNames`.

## [P1] The `run` auto-destroy path never deleted the project

`internal/cli/run_command.go`. S165 documented this as a deliberate gap: that
path sits outside `executeTest` and the project id lived in the state file it
had no access to.

**The cutover closes it.** The id now comes from the marker via
`CaptureSweepTarget`, which this path already calls. Keeping the gap would have
been carrying a known leak past the change that removed its cause. Accepted and
fixed: `releaseRunProject` before the sweep, with the recovery-command
annotation the other failures on that path get.

## Nothing declined this pass.

Worth recording across passes 30–32: **five placements of "delete the project"
have now been wrong, and the condition was right in all five.** The cutover
removed one deletion site (Terraform) that four paths had been depending on
without naming it.
