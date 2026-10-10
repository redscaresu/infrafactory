---
kind: code
status: ready
depends_on: []
touches: ["internal/cli/test_command.go", "internal/cli/test_command_test.go"]
---

# An interrupted `test` does not report a false `destruction/destroy` failure

Seen on real AWS in aws-layer3-claim-legs (2026-10-10, run 20261010T103819Z): after one Ctrl-C the
real teardown passed (sandbox_deploy destroy, aws_scope_sweep and aws_scope_release), but the
summary also showed `destruction/destroy: fail` with detail "context canceled". The `destruction`
layer's destroy (appendDestroyResult, internal/cli/test_command.go) runs on the signal-cancelled
context, so it fails for the interrupt, not for anything left behind. A red stage that is not a
leftover sends the operator looking for resources that do not exist.

Run it on the same fresh teardown context as the sandbox teardown (teardownContext /
finishTeardown, #429), or, if it cannot run after an interrupt, report it as skipped with a
detail naming the interrupt, never as a failure.

**Done when:**
- A test interrupts `test` (cancelledNotify) and shows `destruction/destroy` passes or is skipped
  with an interrupt detail, never `fail` with "context canceled". It fails on today's code, shown
  by mutation.
