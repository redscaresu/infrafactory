---
kind: code
status: blocked
blocked_by: [aws-layer3-ami-resolve-wiring]
epic: aws-layer3-wiring-proof
depends_on: [aws-layer3-ami-resolve-wiring]
touches: ["internal/cli/aws_layer3_order_test.go (new)", "internal/cli/aws_run_failure_test.go"]
---

# One injected-fake `run --holdout` proves the whole AWS Layer 3 stage order and that each failure ends it there

Tests only, in internal/cli. Holdout runs only through `run --holdout` with sandbox_deploy on
(holdout.go:347, run_command.go:75,1164), so drive `run`, not `test`. Extend runAWSRun
(aws_run_failure_test.go:54) rather than a new harness. Fakes:

- the awsLifecycle doer for STS/SSM/EC2 (aws_scope_lifecycle_test.go:47-104);
- fakeSandboxDeployHarness for the apply (the apply env is sealed to real AWS,
  aws_sealed_env.go:38-54, so fakeaws cannot serve it);
- a fake Deps.RealProbe for http_probe and holdout (runtime.go:92-93,123; there is no dialer seam);
- a seed generator returning the raw aws-web-step-one fixture (awsStepOneFixtures,
  e2e/testdata/aws-web-step-one) with the AMI id the doer served and no user-data file —
  not awsAdmittedStack, which already contains infrafactory-user-data.sh and which
  placeAWSUserData refuses (aws_user_data.go:62-63);
- the region awsAdmittedRegion (us-east-1), not runAWSRun's eu-west-2 (aws_run_failure_test.go:65);
- the real gate, recorded by a wrapper that logs an event then calls layer3PreflightHCLForCloud
  (with Layer3HCLGate nil nothing else records it);
- a scenario with an http_probe compute:80 criterion and a holdout file under
  `<paths.scenarios>/holdout` (holdout.go:59-78), run with `--holdout` and repairs: 1.

Existing interrupt and reap coverage is cited, not duplicated: aws_run_failure_test.go:256-290
(interrupt inside the apply names reap), e2e/aws_reap_fakeaws_test.go:70, aws_reap_command_test.go.
Rows that fail before the claim (the resolve) stay with aws-layer3-ami-resolve-wiring. A
production defect this finds is reported to the lead as a new story, not fixed here. Commit
trailer: `ADR: none — tests only`.

**Done when:**
- One ordered log (the doer's calls plus generator, gate, apply, probe and destroy events)
  contains, in this order: sts GetCallerIdentity; ssm GetParameter(AL2023 parameter); ec2
  DescribeImages; generate; gate (generation, generate_command.go:843); gate (test,
  test_command.go:710); sts GetCallerIdentity; stamp read; default-VPC describe; claim put; apply;
  account_check; user_data_check (DescribeInstanceAttribute); http_probe; holdout; destroy; sweep
  (EC2 and SSM Describe*, incl. ssm:DescribeParameters); claim get before claim DeleteParameter.
  The run ends target_reached.
- The deploy fake's onRunDir asserts infrafactory-user-data.sh in the apply dir is a regular file
  equal to renderAWSUserData of the scenario's service.
- A table with one row per forward stage from the gate on, each failed by its fake, pinned to one
  iteration (repairs: 1): no later forward stage appears in the log. A gate failure ends `run` at
  generation with an error (generate_command.go:843). From the apply on, destroy and the sweep
  still run, the output names `infrafactory reap`, the claim is deleted after a clean sweep and
  still held after a dirty one.
- Moving the resolve call after the claim, or the post-apply checks after http_probe, makes the
  test fail; the builder shows this with a local mutation (cp backup and restore, never git
  checkout) and records it in the PR.
