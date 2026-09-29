# Codex review pass 33 — S166+S167 cutover

`codex exec review --base main` on `s166-cutover` at `bc7f1bd`.

Two findings, both accepted, both P1. Both are the cutover reaching a path that
was written when the project came out of Terraform.

## [P1] Teardown destroyed with the wrong provider default project

`live_teardown.go` built its environment with `sandboxCommandEnv(runtime)`,
which passes an empty project, so `tofu destroy` ran with the **shared
fallback** as the provider default while the apply that created those resources
ran with the run's own project. A destroy is the inverse of an apply and must
run in the same provider context; anything the provider resolves by project
rather than by id refreshes against the wrong one.

`reap` had the same call, and it already had the marker's project id in hand.

Fixed: both use `sandboxCommandEnvForProject(runtime, <project>)`.

## [P1] The interrupt guard never deleted the project

`withSandboxInterruptGuard` destroys on Ctrl-C and then exits. Before the
cutover `tofu destroy` took the project with it. Now it does not, so every
interrupted Layer 3 run left an empty project — and an interrupt is the **one
exit with no stage summary**, so unlike the other paths it could not even report
what it kept.

The same review of that function surfaced a second half the earlier passes
missed: it bailed on a missing state file with "nothing to clean up". Since the
project is created before the apply, an interrupt between those two points
leaves a real project and no state — precisely the shape the message denied.

Fixed: the guard reads the marker, and

- state present → destroy, then delete the project;
- no state but a marker → skip the destroy, delete the project, and say so;
- neither → unchanged, genuinely nothing to clean up.

Covered by `TestInterruptGuardDeletesTheProjectWhenNothingWasApplied` and an
assertion added to `TestInterruptGuardDestroysLiveResources`.

## Nothing declined this pass.

Running total across passes 30–33: **six placements of "delete the project"
have been wrong and none of the conditions have been.** Removing a deletion
Terraform used to perform invalidated every path that had been depending on it
implicitly, and the paths surfaced one review at a time rather than from any
one reading of the diff.
