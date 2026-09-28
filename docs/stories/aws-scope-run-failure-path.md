---
kind: code
status: blocked
blocked_by: [aws-scope-test-lifecycle, aws-scope-reap-command]
epic: aws-layer3-claim-sweep-reap
depends_on: [aws-scope-test-lifecycle, aws-scope-reap-command]
touches: ["internal/cli/run_command.go", "internal/cli/aws_run_failure_test.go (new)", "internal/cli/layer3_teardown_cloud_test.go"]
risk: high
---

# run acts on the claim only when it holds it, ends the run whenever an iteration kept the claim, and prints the reap command on an interrupt

Replace run_command.go:779-792's aws 'teardown is not built' arm. If no iteration recorded a claim take, the arm makes zero AWS calls. Otherwise it calls ReadAWSClaimHolder: absent or another holder means zero writes, reported as not held by this run. Only when the stored holder is the run's own does it act: the env comes from aws.account_id through awsCommandEnvForAccount, never from the Scaleway marker (run_command.go:798-803); it destroys if state exists, then calls awsReleaseAfterCleanSweep. The run ends after any iteration whose stages carry StageAWSScopeClaimKept, whatever the cause: dirty, 403, settle timeout, --no-destroy, destruction disabled, a failed release, or an unknown outcome. The terminal reason is new and lives only in run_command.go, never repair_budget_exhausted or stuck, so learning does not fire (run_command.go:442). For cloud aws, run's loop is wrapped in withSandboxInterruptGuard, which test already uses (test_command.go:57), so an interrupt prints the reap command.

**Done when:**
- A gate-refused aws run makes zero AWS calls. The aws row of TestFailedRunOnAnotherCloudTearsNothingDown is rewritten to assert exactly that; its non-aws rows are unchanged.
- Through runRunCommand with the gate stubbed, a fake LLM and fake doers. When the iteration released the claim, the arm sends zero DeleteParameter calls and reports no ParameterNotFound failure. When the claim is held by another holder, the arm makes zero writes and zero EC2 calls.
- When the iteration kept the claim for a dirty sweep, the run ends with the fake LLM's generate count at one. The arm destroys, sweeps and releases only on a clean sweep, and otherwise names the reap command.
- An aws run with --no-destroy and two repair iterations available ends after one, with generate called exactly once.
- An interrupted aws run, via a fake notify, prints the reap command and makes zero Scaleway calls. Every Scaleway run test passes unmodified.
