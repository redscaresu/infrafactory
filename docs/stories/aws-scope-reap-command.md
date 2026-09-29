---
kind: code
status: ready
epic: aws-layer3-claim-sweep-reap
depends_on: [aws-scope-reap-body, aws-scope-test-lifecycle]
touches: ["internal/cli/reap_command.go", "internal/cli/aws_reap_command_test.go (new)", "internal/cli/layer3_teardown_cloud_test.go"]
risk: high
---

# `infrafactory reap` gets a state-free AWS path that refuses a held claim unless --take-over names its holder, and an interrupted AWS test leaves the claim and prints the reap command

runReapCommand's aws arm replaces the refusal at reap_command.go:50-53. It needs no state and no marker. Its order: STS preflight, then AssertAWSScopeStamp, then the claim. If the claim is absent, reap takes it with TakeAWSClaim(its holder). If it is held and there is no --take-over, reap refuses, naming the holder and printing the exact `infrafactory reap <scenario> --take-over <holder>`. With --take-over H it calls TakeOverAWSClaim(H, own). Then destroySandbox's aws arm runs if state exists. Then come the sweep, ReapAWSScope and awsReleaseAfterCleanSweep. --dry-run sweeps, names what it finds, exits non-zero on any stray, and writes nothing. withSandboxInterruptGuard's aws arm (reap_command.go:181-184) never prints 'nothing to clean up' (:191-194) and never reads the Scaleway marker. It prints that the claim is kept, with the shell-quoted reapCommand (run_command.go:1589-1595); reap names the holder.

**Done when:**
- reap for aws with no state and no marker: the call log shows GetCallerIdentity, the stamp, TakeAWSClaim, the sweep, the deletes, the verdict sweep, then DeleteParameter. It exits 0.
- A wrong account or a bad stamp gives zero SSM writes and zero EC2 mutating calls.
- A held claim without --take-over refuses naming the holder and the exact command, with zero writes. --take-over with the wrong holder refuses naming the actual one.
- A dirty verdict fails naming each stray, keeps the claim and exits non-zero.
- --dry-run with a stray names it, exits non-zero, and sends zero Put, Delete, Terminate, Release or Revoke calls.
- An interrupted aws test, driven through runTestCommand with the gate stubbed and a fake notify, prints the reap command, not 'nothing to clean up'. It makes zero Scaleway calls and zero DeleteParameter calls.
- TestReapOnAnotherCloudRefusesBeforeReadingTheMarker and TestInterruptedTestOnAnotherCloudTearsNothingDown keep their non-aws rows unchanged. Every Scaleway reap and interrupt test passes unmodified.
