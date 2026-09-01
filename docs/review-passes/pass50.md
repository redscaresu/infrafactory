# Codex review pass 50 — S155b `live upgrade`

Two findings, both accepted.

## [P1] The tag advanced even when the apply never ran

I moved the record onto the new tag on every path, reasoning that a half-finished
apply is running *something* and a record naming the old version would send the
next observation looking for the wrong thing. That reasoning is right for a
failure **during** apply and wrong for a failure at init or plan, where nothing
reached the cloud at all — and advancing there makes the record claim a version
that was never deployed, which is the exact falsehood S155a exists to prevent.

Split on `applyRan(err)`, which reads the `SandboxDeployError.Stage`. Anything
unrecognised counts as "it ran": assuming nothing happened is the answer that
loses infrastructure.

## [P2] The address could go stale across an upgrade

Replacement HCL can recreate the load balancer. `d.Address` was captured at first
deploy and never refreshed, so `upgrade_verify` would probe infrastructure this
deployment no longer owns — and every later `live observe` would keep probing it.

Now re-read from state after the apply, reported when it moves. When it cannot be
re-read the old one is kept **and said out loud**, because everything after that
point probes an address nothing just confirmed.

## What the test fake taught

`TestUpgradeRefreshesTheAddressWhenTheEndpointMoves` failed at first because
`fakeSandboxDeployHarness.Run` writes its own minimal state, and my `onRun` hook
fired *before* it — so the fake overwrote the fixture the test had just staged.
The hook now runs last. Worth noting because the failure looked like the
production code ignoring the new address, and was not.

## Nothing declined this pass.
