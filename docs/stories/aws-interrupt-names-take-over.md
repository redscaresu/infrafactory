---
kind: code
status: ready
epic: aws-web-live-on-real-aws
depends_on: []
touches: [internal/cli/reap_command.go, internal/cli/aws_scope_lifecycle.go, internal/cli/run_command.go, internal/cli/test_command.go, internal/cli/aws_reap_command_test.go, internal/cli/aws_scope_lifecycle_test.go, internal/cli/aws_run_failure_test.go, internal/cli/reap_command_test.go, docs/operations.md, internal/cli/aws_scope_doc_test.go]
---

# Every aws print that leaves the claim held names `reap --take-over <holder>`, the command that works

When an aws run keeps the claim, the command it tells the operator to run refuses. Plain
`infrafactory reap` reads the claim and refuses a held one, naming `reap --take-over <holder>`
instead (claimAWSScopeForReap, internal/cli/reap_command.go:206-218; pinned by
TestAWSReapRefusesAHeldClaimUnlessTakeOverNamesItsHolder, internal/cli/aws_reap_command_test.go:184).
Yet two prints name plain reap:
- the Ctrl-C print in withSandboxInterruptGuard (internal/cli/reap_command.go:311-315), which the
  epic asks to see from both `run` and `test`;
- every `aws_scope_claim_kept` detail from awsScopeTeardown and awsScopeClaimKept
  (internal/cli/aws_scope_lifecycle.go:76-126), which builds `reap` with reapCommand while
  `opts.AWSClaimHolder` is in hand.

Fix it where the command is built. A kept claim names awsTakeOverCommand(runtime, holder)
(reap_command.go:221), as reap's own kept-claim path already does (:264). The interrupt guard does
not know the holder today: pass it in from `run` (controls.AWSClaimHolder, run_command.go:102) and
`test` (awsClaimHolderFor, aws_scope_lifecycle.go:25). The interrupt print fires whether or not the
run's teardown released the claim, and `--take-over` refuses when no claim is held
(deleteAWSClaimHeldBy, internal/harness/aws_scope.go:181). So the print names both: the take-over
command if the claim is still held, plain reap if it is not. The `### Running aws-web-live`
paragraph "What a failure prints" in docs/operations.md says the same, and
TestAWSRunChecklistNamesTheStagesAndTheReapCommand pins the take-over form. Commit trailer:
`ADR: none — the recovery hint names the command that accepts a held claim`.

**Done when:**
- TestInterruptedAWSTestKeepsTheClaimAndPrintsTheReapCommand, and a `run` twin, assert the output
  contains `reapCommand(...) + " --take-over " + <the run's holder>` and the plain command for the
  released case. Every existing aws claim-kept assertion in aws_scope_lifecycle_test.go and
  aws_run_failure_test.go asserts the `--take-over <holder>` suffix, not only the reapCommand prefix.
- Reverting awsScopeClaimKept to reapCommand (cp backup, restore by cp) fails at least one of them;
  the PR records it.
- Scaleway's interrupt print is unchanged (reap_command_test.go passes unedited apart from the
  guard's new argument).
- `go test -tags noui ./internal/cli/...` and `make doc-hygiene` pass.
