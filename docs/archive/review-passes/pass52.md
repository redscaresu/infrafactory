# Codex review pass 52 — S155b `live upgrade`

Two findings, both accepted, both P1.

## [P1] The real-deploy opt-in was not required

`deploy` refuses unless `validation.layers.sandbox_deploy.enabled`. `upgrade`
did not check it at all — so an operator with credentials and an allowlist but
Layer 3 deliberately **off** could still apply to the deployment's real project.

A gate that guards one entry point into real infrastructure and not the other
guards nothing. Added, and checked before anything touches the workdir.

## [P1] An environment failure left unapplied configuration in the workdir

`sandboxCommandEnvForProject` ran *after* the destructive swap, so an env problem
returned early with the new HCL in place and nothing applied — the same shape
pass 51 fixed for init/plan failures, on a path pass 51 did not cover.

**Fixed by reordering rather than by another rollback.** Every fallible step that
does not touch the workdir now happens first, which removes the failure mode
instead of compensating for it. The swap itself also restores on partial failure,
because that one genuinely cannot be ordered away.

## Three passes, one question

Passes 50, 51 and 52 all found the same thing in different places: *what happens
when nothing reached the cloud?* The tag, the address, the workdir contents, and
now the ordering of setup. Each was individually right and collectively
inconsistent.

The durable fix is not the fourth patch — it is that `applyRan` is now the single
predicate, and that everything which can fail without touching the workdir runs
before the part that does. **Ordering beats compensating**: one rollback path
instead of one per early return.

## Nothing declined this pass.
