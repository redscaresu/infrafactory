---
kind: lead
status: blocked
blocked_by: [aws-gate-assembled, aws-ec2-reads, aws-layer3-destroy-arm, operator:user-approval]
epic: aws-layer3-gate
depends_on: [aws-gate-assembled, aws-ec2-reads, aws-layer3-destroy-arm, operator:user-approval]
touches: ["internal/cli/layer3_cloud.go", "internal/cli/test_command.go", "internal/cli/generate_command.go", "internal/cli/deploy_command.go", "internal/cli/live_upgrade.go", "internal/cli/runtime.go", "internal/cli/aws_post_apply.go (new)", "internal/cli/aws_post_apply_test.go (new)", "internal/cli/layer3_cloud_test.go", "docs/decisions/0023-layer3-sealed-environment-and-orphan-verification.md", "docs/epics/aws-layer3-gate.md (delete)", "docs/stories/aws-layer3-gate-lift.md (delete)"]
risk: high
---

# The last refusal this epic owns is lifted. The aws arm runs the AWS gate at all four call sites, the account and the instance's API user data are checked after apply on the Layer 3 path, and ADR-0023 records the file() exception. It merges only on the user's written approval, after claim-sweep-reap's real-AWS planted-leak proof

This lifts the gate refusal (layer3_cloud.go:35-41), and only that one; refusal_lift assigns every other refusal to its owner. The lead dispatches this story only when refusal_lift_rule holds.

- layer3PreflightHCLForCloud(cloud, outputDir, layer3GateInputs) takes the allowlist plus the AWS inputs, and aws calls validateAWSLayer3HCLShape. The signature change goes through Deps.Layer3HCLGate (runtime.go:142), whose type changes with it.
  - test_command.go:690 passes cfg.AWS.Region (not awsRegion's default), {runtime.AWSLayer3AMI, runtime.AWSLayer3AMIRoot}, and renderAWSUserData(*sc.Service). A nil service or a render error refuses.
  - generate_command.go:837 passes the userData and AMI it already holds (:751-761).
  - deploy_command.go:110 and live_upgrade.go:95 pass only the allowlist; aws never reaches them.
- runtime.go gains AWSLayer3AMIRoot harness.AWSAMIRoot beside AWSLayer3AMI (:145-148). aws-layer3-wiring-proof's resolve stage sets both. Until it does, both are empty, and the gate refuses.
- New file internal/cli/aws_post_apply.go adds appendAWSPostApplyChecks. When cloud is aws, it runs in executeTestWithScenario right after a successful SandboxDeploy.Run (test_command.go:901-904) and before criteria.
  - Stage sandbox_deploy/account_check calls harness.UnplacedAWSResources(outputDir, cfg.AWS.AccountID).
  - Stage sandbox_deploy/user_data_check calls AWSStateInstanceID and AWSInstanceUserData with the sealed env and Deps.AWSEC2, then bytes.Equal against the rendered script.
  - A mismatch or an error adds a fail stage and a FailureSummary, and sets sandboxApplied to false, so no real probe runs. Teardown still runs, through destroySandbox's aws arm and claim-sweep-reap's sweep and release.
  - Reuse Deps.AWSEC2, which the claim lifecycle already defaults (runtime.go:468). A nil AWSEC2 fails the check and never skips it.
- ADR-0023 gains a dated amendment. It describes only controls that take effect in this PR: the file() exception and its exact AST; the pre-tofu file check; the AMI root bound; and the account_check and user_data_check stages. It also records that file() stays off layer3PureFunctions (layer3_hcl_shape.go:809-833).
- One existing test changes: the 'aws is refused' row of TestGenerationGateIsKeyedOnTheScenarioCloud (layer3_cloud_test.go:167) now expects the AWS gate's refusal of the Scaleway stack.
- The PR deletes docs/epics/aws-layer3-gate.md, and its body links claim-sweep-reap's planted-leak proof.

Dependency notes: the claim, sweep, reap and the planted-leak proof (2026-09-29, #399; ADR-0040) have landed. The gate refuses aws before the claim (test_command.go:690 vs :836), so the first real run is the first to prove these legs, none of which the proof reached:
- `test` and `run` taking and releasing the claim on real AWS;
- a second `test` refused, naming the holder;
- the failed-apply sweep;
- the interrupt prints from `run` and `test`;
- `tofu destroy`'s IAM actions.

**Done when:**
- runTestCommand with cloud aws, sandbox enabled, AWSLayer3AMI and AWSLayer3AMIRoot set, the assembled web-step-one directory, fake STS, claim-sweep-reap's fakes, and a fake SandboxDeploy that writes terraform-live.tfstate. With every resource in the account and a fake EC2 that returns base64 of the rendered script, appendAWSPostApplyChecks passes both account_check and user_data_check. SandboxDestroy.Run is then called once, through destroySandbox's aws arm, and then the sweep
- The same run fails at account_check, recorded by appendAWSPostApplyChecks from UnplacedAWSResources and naming the resource, in two cases: one resource's arn names another account; an aws_route names a route_table_id that is not in the state. Real-probe calls are zero, and SandboxDestroy.Run and the sweep are still each called once
- The same run fails at user_data_check, recorded by appendAWSPostApplyChecks, with zero real-probe calls and teardown still called, in each of these cases: the fake EC2 returns the script with one byte changed, the state's SHA1, an empty value, or a 403; Deps.AWSEC2 is nil
- The same run fails at sandbox_deploy/allowlist, refused by validateAWSLayer3HCLShape, with MockDeploy, SandboxDeploy and the EC2 fake at zero calls, in each of these cases: a tampered infrafactory-user-data.sh; a second provider "aws"; an empty AWSLayer3AMIRoot; an empty aws.region
- generateAndWriteFilesWithResult (aws, sandbox enabled, AMI and root set) passes the gate for web-step-one.tf. validateAWSLayer3HCLShape refuses the same stack plus an aws_launch_template
- A test reads ADR-0023 and requires the new amendment to quote generator.AWSUserDataLine verbatim, and to name the stages account_check and user_data_check that the runTestCommand tests observe
- These pass unmodified: TestTestCommandRefusesANonScalewayCloudAtTheGate, TestRunKeepRefusesANonScalewayCloudBeforeCredentials, TestDeployRefusesANonScalewayCloudBeforeCredentials, TestLiveUpgradeTakesTheCloudFromTheRecord, TestNoCallSitePassesAHardcodedLayer3Cloud, layer3_hcl_shape_test.go, and the allowlist-to-collection test
- Merge gate, not a CI test: the user has written an explicit approval of this lift as their own comment on the PR, and the lead has linked it from the PR description. The lead never writes, paraphrases or infers the approval, and does not merge without it, --admin included
