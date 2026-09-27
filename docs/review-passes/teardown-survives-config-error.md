# Review — a failed apply must not leak a real stack when destroy cannot evaluate the config

Named for the story rather than numbered: pass numbers are a shared counter, and
parallel agents picking the next one collide.

One finding declined across the loop; the rest were accepted and are in the PR body.

## Declined: [P2] "Rerun the auto-created purge after the config-free destroy"

The claim: the purge in `destroySandbox` runs before the config-free destroy removes
the Terraform-owned resources, so Scaleway's auto-created default security group may
still be in use then; returning success straight after `RunWithoutConfig` leaves it
behind and the project delete fails.

It is already handled one step later, by design. Every teardown path that calls
`destroySandbox` (`test`, `run` auto-destroy, `live teardown`, `reap`, the interrupt
path) calls `releaseRunProject` next, and `releaseRunProject` purges auto-created
resources and retries whenever the project delete fails ("PURGE FIRST, always",
`run_project_lifecycle.go`). That purge is the one that exists for exactly this case:
under ADR-0025 the ordinary destroy no longer deletes the project, so the 412 from
the default security group lands on the project delete, not on the destroy. A second
purge inside `destroySandbox` would duplicate it, running before the delete whose
failure proves it is needed.
