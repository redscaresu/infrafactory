# S165 review pass 26 — two findings, both acted on

### [P2] `--no-destroy` would have deleted a project whose resources are live

A regression introduced by pass 25's own fix. Lifting the cleanup out of the
destroy branch lost the context that destruction had actually *run*, so
`len(failures) == 0` on a `--no-destroy` run — where the apply succeeded and the
resources are deliberately still up — matched the delete case. That would either
fail on resources in use or remove the handle to a run the operator asked to
keep.

**"No failures" is not "nothing is left."** Deletion now requires that no state
was ever written, or that destruction genuinely ran and the account came back
clean.

### [P2] A cancelled run skipped the cleanup entirely

`releaseRunProject` passed the run's context straight to `Delete`. If the run was
cancelled — Ctrl-C, a timeout — that context is already done, so the API call
never happened and the project survived on exactly the runs that most need
cleaning up.

It now uses a fresh bounded context (`context.WithoutCancel` plus its own
timeout), the same reasoning as the interrupt guard's destroy: the whole point is
to do work *after* cancellation. Covered by
`TestReleaseRunProjectSurvivesACancelledRunContext`.
