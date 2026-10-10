---
kind: code
status: ready
depends_on: [aws-interrupt-names-take-over]
touches: ["internal/cli/reap_command.go", "internal/cli/run_command.go", "internal/cli/aws_scope_lifecycle.go", "internal/cli/aws_reap_command_test.go", "internal/cli/aws_run_failure_test.go"]
risk: high
---

# A first Ctrl-C at Layer 3 still finishes the teardown it interrupts, on aws and Scaleway, for `test` and `run`

Found by /code-review on #416 (2026-10-10). These gaps predate #416, and #416 deliberately left them out:

- **aws `test`:** a first Ctrl-C cancels the context the teardown then runs on. `awsScopeTeardown`'s
  destroy fails at once and the settle wait returns `ctx.Err()`, so the claim is kept and the
  resources stay live and billable until a manual reap. The aws branch of
  `withSandboxInterruptGuard` only prints a notice; unlike Scaleway's branch, it runs no destroy on a
  fresh context.
- **aws `run`, failure arm:** a first Ctrl-C during the arm cancels its own context, so its destroy
  aborts as well, not only the settle waits.
- **Scaleway `run`:** it has no signal handling at all. `iterate` runs on `cmd.Context()`, so a first
  Ctrl-C kills the process with real resources live and prints no `infrafactory reap` line.

Give each of these paths what Scaleway `test` already has: after an interrupt, the teardown (destroy,
then sweep and release on aws) runs on a fresh, bounded context that the first signal did not
cancel. The guard prints that it is finishing the teardown, a second Ctrl-C abandons it, and the
recovery command is printed either way (#416's first-signal notice). Commit trailer:
`ADR: none — interrupt teardown parity`.

**Done when:**
- An aws `test` interrupted mid-apply still runs destroy, sweep and release on a fresh context. A
  test with the injected-fake harness sees `destroy` and `claim delete` after the interrupt, and the
  claim is released when the sweep is clean.
- The same holds for the aws `run` failure arm interrupted during its destroy.
- A Scaleway `run` interrupted mid-apply destroys, or prints `infrafactory reap <scenario>`, exactly
  as Scaleway `test` does. A test covers it.
- A second signal during that teardown abandons it and prints the recovery command. A test covers it.
- Each new test fails with its fix removed (cp backup, break, restore with cp), recorded in the PR.
