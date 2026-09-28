---
kind: code
status: blocked
blocked_by: [aws-scope-claim, aws-scope-sweep]
epic: aws-layer3-claim-sweep-reap
depends_on: [aws-scope-claim, aws-scope-sweep]
touches: ["internal/cli/aws_scope_lifecycle.go (new)", "internal/cli/aws_scope_lifecycle_test.go (new)", "internal/cli/run_project_lifecycle.go", "internal/cli/test_command.go", "internal/cli/generate_command.go", "internal/cli/runtime.go", "internal/cli/run_command.go", "internal/cli/layer3_teardown_cloud_test.go"]
risk: high
---

# The test path's AWS arm. The claim is taken after the checks that can refuse, every exit after it funnels into one teardown, the release happens only after a clean sweep, and it is proven through runTestCommand with the gate stubbed

(1) Gate seam. Add Deps.Layer3HCLGate. nil means the real layer3PreflightHCLForCloud; production never sets it. test_command.go:690 and generate_command.go:837 call through it, so tests can drive the production callers past the aws refusal (layer3_cloud.go:35-41), which today returns at test_command.go:718-736. (2) Holder. run mints NewAWSClaimHolder(runID) once (run_command.go:85-86) and passes it in testExecutionOptions (:1106); test mints its own. One holder per process. The claim is taken and released per test execution. (3) Take. The ensureRunProject aws arm (run_project_lifecycle.go:25-32) runs after the STS preflight (test_command.go:830): stamp, then default VPC, then TakeAWSClaim. It returns aws.account_id, as awsCommandEnvForAccount requires (aws_preflight.go:41-49). ErrAWSClaimOutcomeUnknown counts as held. (4) Teardown. There is one awsScopeTeardown call after every branch, the ADR-0025 lesson 'released in one place' (0025:181-227). For aws, it replaces the destroy block's release and sweep (test_command.go:977-996) and the final switch (:1047-1080). It destroys if state exists and destruction is wanted. It then calls awsReleaseAfterCleanSweep, the single release function: it sweeps itself, and releases only when the sweep is clean. When the claim is kept it adds StageAWSScopeClaimKept, an exported constant, and names `infrafactory reap <scenario>`. releaseRunProject's and appendOrphanSweepResult's aws arms stay refused. (5) Deps.AWSSSM and Deps.AWSEC2 default the way AWSSTS does (runtime.go:450-451). The preflight pass detail says the SCP is not asserted.

**Done when:**
- Driven through runTestCommand, cloud aws, the gate stubbed, fake SandboxDeploy and Destroy, and fake AWS doers. A missing stamp or a default VPC fails with zero PutParameter and zero SandboxDeploy calls. A held claim fails naming the holder, with zero SandboxDeploy and zero DeleteParameter calls.
- On the success path the call log shows TakeAWSClaim before SandboxDeploy.Run, then SandboxDestroy.Run, then the sweep's last Describe, then the claim's DeleteParameter.
- Each exit after the take runs the sweep, releases only when it is clean, and otherwise keeps the claim, adds StageAWSScopeClaimKept and names the reap command. The exits are: env build failure; SandboxDeploy failing with no state written; SandboxDestroy failing; a cancelled ctx; a sweep 403; a settle timeout. None of them prints 'delete it by hand' or 'holds nothing but could not be deleted'.
- --no-destroy and Destruction.Enabled=false each keep the claim with zero Destroy and zero DeleteParameter calls, add StageAWSScopeClaimKept, and name the reap command.
- ErrAWSClaimOutcomeUnknown gives zero SandboxDeploy calls, keeps the claim, and names the reap command.
- Source tests: harness.ReleaseAWSClaim has one non-test caller, awsReleaseAfterCleanSweep, and it calls SweepAWSScope before ReleaseAWSClaim. awsScopeTeardown has one call site in test_command.go. No non-test file assigns Deps.Layer3HCLGate.
- In TestEveryTeardownSeamRefusesAnotherCloud only the aws ensureRunProject row changes; the other rows and every Scaleway test pass unmodified.
