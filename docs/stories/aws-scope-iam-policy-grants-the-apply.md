---
kind: code
status: blocked
blocked_by: [aws-web-live-layer2-llm-run]
epic: aws-web-live-on-real-aws
depends_on: [aws-web-live-layer2-llm-run, aws-interrupt-names-take-over]
touches: [docs/layer3/aws/iam-policy.json, internal/harness/aws_scope_policy_test.go, docs/operations.md, docs/layer3/coverage.md, internal/cli/aws_scope_doc_test.go]
risk: high
---

# iam-policy.json and its closed-set test grant the apply, destroy and the AL2023 public parameter, and nothing else

Today the policy grants claim, sweep and reap only. awsScopeGrants()
(internal/harness/aws_scope_policy_test.go:77-95) refuses any other action, so a JSON-only edit
fails CI. The JSON is the one list (ADR-0040 decision 14); operations.md describes it.

The candidate actions are the EC2 calls the pinned provider (5.100.0) sends to apply and destroy,
read with `TF_LOG=debug` against fakeaws at FAKEAWS_SHA over three inputs:
- internal/e2e/testdata/aws-web-step-one/web-step-one.tf;
- the generated HCL of the Layer 2 run from aws-web-live-layer2-llm-run (its run's `generated/`
  snapshot), which is what the model actually writes;
- a variant of that HCL with an `aws_eip` attached to the instance (aws_eip is allowlisted,
  infrafactory.yaml:192, so the model may emit it).
TF_LOG shows API calls, not implicit authorizations. Generation always writes `default_tags`
(internal/cli/generate_command.go:593-601), so every create sends TagSpecifications, and AWS then
also checks ec2:CreateTags. Decide it explicitly: grant ec2:CreateTags pinned by
`aws:RequestedRegion` (and by `ec2:CreateAction` to the granted create actions), or record why it is
left out. Add ssm:GetParameter on
`arn:aws:ssm:REGION::parameter/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64`
(no account segment); the resolve's DescribeImages (internal/harness/aws_ec2_reads.go) is already
inside `ec2:Describe*`. New
statements are pinned by `aws:RequestedRegion`; existing Sids are kept (the widening test mutates
them by Sid). Widen awsScopeGrants() to exactly the new set.
TestAWSScopeIAMPolicyViolationsCatchEachWidening needs a new example widening, since RunInstances
becomes legitimate, and an ec2:CreateTags-without-condition case if CreateTags is granted.

Docs, so they stay true once the grant lands:
- docs/operations.md Scope setup step 3 (:182-188) stops enumerating actions: it says what each
  statement is for, points at the JSON and drops the link to this epic's file. The simulator line
  for ec2:RunInstances (:210-211) flips to allowed, and a line for an action outside the policy stays
  implicitDeny.
- The `### Running aws-web-live` sentence "does not yet grant ssm:GetParameter ..."
  (docs/operations.md:364-366) is rewritten to say the policy grants it, still naming only
  ssm:GetParameter, so TestAWSRunChecklistNamesTheStagesAndTheReapCommand keeps passing (or is
  updated in step).
- The coverage.md aws-web-live blocker cell (docs/layer3/coverage.md:119, "cannot yet resolve the
  AMI ... grants none of the apply's IAM actions") is rewritten; the status stays
  `runnable, unrun`.
Merging this changes nothing on AWS until the user applies it (aws-layer3-iam-measured). Commit
trailer: `ADR: none — measured-candidate grants, ADR-0040's single-owner rule unchanged`.

**Done when:**
- TestAWSScopeIAMPolicyGrantsOnlyClaimSweepAndReap (renamed if its name is now false) passes.
  Adding ec2:CreateKeyPair or iam:PassRole to any statement fails it.
- The policy has no ec2:* or ssm:* wildcard beyond the existing `ec2:Describe*`. Every new
  '*'-resource statement carries StringEquals aws:RequestedRegion REGION, and the JSON has no
  account id (the configured `aws.account_id` in any form, or an ARN's account field).
- TestAWSScopeSetupRunbookNamesWhatTheCodeReads and
  TestAWSRunChecklistNamesTheStagesAndTheReapCommand pass.
  `grep -n 'epics/aws-web-live-on-real-aws' docs/operations.md` returns nothing, and step 3 names no
  IAM action except in the simulator block.
- `grep -n 'not yet grant\|cannot yet resolve' docs/operations.md docs/layer3/coverage.md` returns
  nothing.
- The PR lists the candidate actions per input (fixture, Layer 2 HCL, aws_eip variant), the TF_LOG
  method, and the ec2:CreateTags decision with its reason.
- `go test -tags noui ./internal/harness/... ./internal/cli/...` and `make doc-hygiene` pass.
