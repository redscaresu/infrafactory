---
kind: code
status: ready
epic: aws-layer3-wiring-proof
depends_on: []
touches: ["internal/cli/aws_ami_resolve.go (new)", "internal/cli/aws_ami_resolve_test.go (new)", "internal/cli/generate_command.go", "internal/cli/run_command.go", "internal/cli/test_command.go", "internal/cli/aws_preflight.go", "internal/cli/aws_scope_lifecycle.go", "internal/cli/aws_scope_lifecycle_test.go", "internal/cli/aws_run_failure_test.go", "internal/cli/aws_post_apply_test.go", "internal/cli/aws_user_data_test.go", "internal/cli/layer3_cloud_test.go", "internal/cli/layer3_teardown_cloud_test.go"]
risk: high
---

# Resolve the AL2023 AMI and its root once per command at Layer 3 aws, before generation and before the gate

Nothing in production sets CommandRuntime.AWSLayer3AMI or AWSLayer3AMIRoot (runtime.go:157-163),
so at Layer 3 aws generation refuses (generate_command.go:719-721) and the gate refuses a zero
root (layer3_aws_user_data_gate.go:104). Add one resolve stage, in new
`internal/cli/aws_ami_resolve.go`: STS with the command context (a ctx-aware variant of
assertAWSCredentials, or harness.VerifyAWSIdentity(ctx, ...) directly — aws_preflight.go:25-36
uses context.Background() today), then harness.ResolveAWSAMIFromSSM (aws_ami.go:48) and
harness.DescribeAWSAMIRoot (aws_ec2_reads.go:109) with the sealed env (awsLayer3Env),
runtime.Deps.AWSSSM/AWSEC2 and endpoint "", setting both fields. The stage name is a constant
beside StageAWSScopeClaimKept (aws_scope_lifecycle.go:15); its pass detail carries the AMI id and
the root, so iteration.json and test's JSON both record them.

It runs when cloud is aws and sandbox_deploy is enabled, at three entry points:

- `generate`: runGenerateCommand, before generation (generate_command.go:36).
- `run`: runRunWithNotify, once before the iteration loop (run_command.go:101-104 area), outside
  the interrupt guard (which wraps only the loop, run_command.go:450).
- `test`: runTestWithNotify, after awsClaimHolderFor (test_command.go:53) and before
  withSandboxInterruptGuard (:62), with cmd.Context(). Not inside the guard: for aws it prints
  the reap command on any cancelled context (reap_command.go:296-316). Not in
  executeTestWithScenario, so run's per-iteration test does not resolve again. The pass stage
  reaches executeTest through a testExecutionOptions field and is prepended before `allowlist`;
  on failure runTestWithNotify writes an OutputResult carrying the failed resolve stage.

It takes and releases no claim (ADR-0040 decisions 5-6). No new flag: the switch is the existing
sandbox_deploy.enabled, false in infrafactory.yaml:81; turning it on belongs to
aws-web-live-on-real-aws. Test fixtures that assign the fields before the command
(aws_scope_lifecycle_test.go:355, aws_run_failure_test.go:100, layer3_teardown_cloud_test.go:123,
aws_post_apply_test.go:178) move to the awsLifecycle doer serving GetParameter for
AWSAL2023AMIParameter and DescribeImages with the root. In TestAWSGateRefusesBeforeAnyTofu the
'empty AMI root' row (:178) serves a root DescribeAWSAMIRoot accepts but the gate refuses (gp2,
or DeleteOnTermination false; aws_ec2_reads.go:130-145 vs layer3_aws_user_data_gate.go:104-117),
and the no-ec2-call assertion (:190) excludes ec2:DescribeImages.
TestTestCommandRefusesANonScalewayCloudAtTheGate (layer3_cloud_test.go:115) is updated to what aws
now does. Commit trailer: `ADR: none — implements ADR-0039 decision 7`.

**Done when:**
- On the awsLifecycle call log, `generate`, `run` and `test` with cloud aws and sandbox_deploy
  enabled each send the resolve's sts:GetCallerIdentity before ssm:GetParameter for
  harness.AWSAL2023AMIParameter, then ec2:DescribeImages for the returned id, before the first
  generator call (generate, run) or the gate (test). ssm:GetParameter(AWSAL2023AMIParameter) and
  ec2:DescribeImages each appear exactly once per command, including across a two-iteration run.
  (Later STS calls from the claim preflight are expected and not counted.)
- A table test fails the resolve at each step — STS account mismatch, SSM denied or a non-AMI
  value, DescribeImages with a non-ebs root or no root mapping — and with a context cancelled
  during STS, during SSM and during EC2: the command ends with an error naming the resolve stage,
  the generator is called zero times, the gate is not reached, the claim parameter is never
  written, and no reap command is printed.
- With sandbox_deploy disabled, or a non-aws cloud, none of those three calls appears in the log
  and Layer 2 generation still writes harness.AWSLayer2AMI.
- `test`'s JSON output carries the resolve stage before `allowlist`: pass with a detail naming
  the id and root, or fail with the error (written on failure too). A `run`'s iteration.json
  carries the same pass stage.
- runAWSTestWith(gated) and runAWSRun no longer assign AWSLayer3AMI or AWSLayer3AMIRoot; the gated
  tests pass with values served by the fake doer, and removing the resolve call from any one
  entry point makes at least one test fail.
- Every aws_ami_resolve test injects the awsLifecycle doer, and a guard fails any internal/cli
  test whose resolve reaches the default (real) client because Deps.AWSSSM or AWSEC2 was left nil
  (runtime.go:468-472).
- An AST test beside TestNoProductionCodeSetsTheLayer3HCLGate fails if any non-test function
  other than the resolve stage assigns CommandRuntime.AWSLayer3AMI or AWSLayer3AMIRoot.
