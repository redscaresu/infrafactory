---
kind: lead
status: blocked
blocked_by: [aws-scope-iam-policy-grants-the-apply, aws-layer3-run-evidence, aws-layer2-seeds-resolved-ami, aws-web-live-layer2-llm-run]
epic: aws-web-live-on-real-aws
depends_on: [aws-scope-iam-policy-grants-the-apply, aws-layer3-run-evidence, aws-layer2-seeds-resolved-ami, aws-web-live-layer2-llm-run]
touches: ["docs/layer3/aws/iam-policy.json (only if an action was refused)", "internal/harness/aws_scope_policy_test.go (only if an action was refused)"]
risk: high
---

# The first real `test` over a generated step-one fixture measures the IAM list on real AWS, with no action refused

First real-AWS spend. No LLM: `infrafactory test` over a staged step-one fixture, so nothing is
learned (`test` never calls AppendPitfall; only run_command.go does). The epic's **Checks before
every stage** apply before each run.

**You:** apply the policy (`aws iam put-user-policy ... --profile infrafactory-admin`,
docs/operations.md Scope setup step 3) after aws-scope-iam-policy-grants-the-apply merges, and again
if this story's PR widens it; tell the lead when it is applied. Then say go for each real run in the
lead's session. The lead does not start a run without that go, and tells you the run id and the
cost bound when it ends.

Staging the fixture. A hand-copied web-step-one.tf is refused by the gate before any spend: the gate
needs a terraform block with the aws pin and exactly one provider "aws"
(internal/cli/layer3_aws_provider.go:37-60), and a user-data file byte-equal to renderAWSUserData of
the scenario's service (internal/cli/aws_post_apply.go:28). Only generation writes those. So, from a
throwaway worktree with a local `--config` (the aws block, sandbox_deploy on; nothing committed from
it), run `infrafactory generate internal/e2e/testdata/aws-web-step-one/web-step-one.yaml` with a stub
`agent.claude.command` that prints web-step-one.tf. Generation resolves the AMI on real SSM and puts
it in the HCL (awsAMIForGeneration, internal/cli/generate_command.go:716-728), writes providers.tf
with default_tags and the rendered user-data file into the output dir `test` reads. The stub and its
output go in the evidence.

The run: `infrafactory test` on the same scenario and config. Observe the AMI resolve, the Layer 2
deploy on fakeaws with the seeded id (aws-layer2-seeds-resolved-ami), the gate, the claim, the
apply, account_check, user_data_check, real_probe, destroy, the sweep and the release. Then
CloudTrail `lookup-events` for `Username=infrafactory-layer3` over the run window; events take 5-25
minutes to appear (docs/operations.md:330-332). If the stack is slow to serve, raise the probe
window in the local config and record the value for aws-probe-window-measured and the later runs.

A refused action, or an apply failing on AccessDenied or UnauthorizedOperation: stop, run
`reap --take-over <holder>` if the claim is held, open a PR adding exactly that action to
iam-policy.json and awsScopeGrants(), ask the user to apply it and for a new go. Otherwise there is
no PR, and the evidence (copied out of the worktree before it is removed) goes to the closing PR body.

**Done when:**
- Before each go: the cumulative cost bound of the epic's real runs so far plus this run's worst
  case (instance and address hours at list price for the probe window and teardown) is written down,
  dated, and is under €5.
- A `test` on real AWS ends with aws_ami_resolve, apply, account_check, user_data_check and
  real_probe passing, then aws_scope_sweep and aws_scope_release passing. Afterwards `aws ssm
  get-parameter` on harness.AWSClaimParameter (infrafactory-layer3 profile) returns
  ParameterNotFound, and `infrafactory reap --dry-run` sweeps empty.
- CloudTrail for every run in this story shows no event by infrafactory-layer3 with errorCode
  AccessDenied, UnauthorizedOperation or Client.UnauthorizedOperation. It includes RunInstances,
  TerminateInstances and ssm GetParameter on the AL2023 path; the action list is quoted in the
  evidence.
- The evidence records the dated cost bound for this story's runs, computed from instance and
  address hours (CloudTrail RunInstances to TerminateInstances) at list price, and no 12-digit run.
