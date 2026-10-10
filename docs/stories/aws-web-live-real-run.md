---
kind: lead
status: blocked
blocked_by: [aws-layer3-claim-legs, aws-layer3-iam-measured, aws-learned-pitfall-account-id-scrub, aws-layer3-run-evidence, aws-layer2-seeds-resolved-ami, aws-web-live-layer2-llm-run]
epic: aws-web-live-on-real-aws
depends_on: [aws-layer3-claim-legs, aws-layer3-iam-measured, aws-learned-pitfall-account-id-scrub, aws-layer3-run-evidence, aws-layer2-seeds-resolved-ami, aws-web-live-layer2-llm-run]
touches: [docs/layer3/coverage.md, "docs/layer3/real-vs-mock-deltas.md (only if a delta is seen)"]
risk: high
---

# One LLM `run aws-web-live --holdout` reaches target on real AWS, and coverage.md flips the row to runnable

The epic's first bullet, plus `run`'s claim legs from the fourth. From a throwaway worktree, with
fakeaws at FAKEAWS_SHA for Layer 2 and a local `--config` using the probe window committed in
infrafactory.yaml, or the one aws-layer3-iam-measured recorded if it raised it. The epic's
**Checks before every stage** apply.

**You:** say go before the run, in the lead's session. The lead does not start without it, and
tells you the run id and the cost bound when it ends.

Evidence comes from the run's iteration.json (aws-layer3-run-evidence). ImageId comes from `aws ec2
describe-instances --instance-ids <id from user_data_check>` within the hour a terminated instance
stays visible, and from the CloudTrail RunInstances event; compare both with the aws_ami_resolve
stage's id. Copy `.infrafactory/runs/aws-web-live/<run-id>/` out before the worktree is removed.

A refused action, or an apply failing on AccessDenied or UnauthorizedOperation: stop. Run
`reap --take-over <holder>` if the claim is held, discard the worktree's pitfalls diff (a
permissions fault is not an HCL lesson), widen exactly that action through an
aws-layer3-iam-measured-style PR, and ask the user for a new go. If the run learns an aws_ entry
from any other sandbox_deploy failure, save the worktree's pitfalls diff for
aws-learned-pitfall-from-real-aws and do not commit it here.

The PR flips the coverage.md aws-web-live row to **runnable**, puts `— (run YYYY-MM-DD, <run id>)`
in Blocked on, and updates the totals line and the note. No euro figure in coverage.md (its "No
euro figures here on purpose", docs/layer3/coverage.md:160-165); the cost bound goes in the PR body.
Any real-vs-mock delta seen is added to docs/layer3/real-vs-mock-deltas.md in the same PR.

**Done when:**
- Before the go: the cumulative cost bound of the epic's real runs so far plus this run's worst case
  is written down, dated, and is under €5.
- The run's terminal reason is target_reached. Its final iteration.json has aws_ami_resolve,
  account_check, user_data_check and real_probe (status 200, URL host = the instance's public IP
  from terraform-live.tfstate) passing, and holdout records at that same host with 22 and 443
  blocked and the port-80 success control passing. It also has aws_scope_sweep and
  aws_scope_release passing.
- describe-instances (or the CloudTrail RunInstances event) ImageId equals the aws_ami_resolve
  stage's id. CloudTrail for the run window shows no refused event by infrafactory-layer3.
- The probe window the run used equals the committed one, or the one aws-layer3-iam-measured
  recorded; the PR names which.
- TestLayer3CoverageDocTotalsMatchItsTable passes with the totals at 4 have run and 1 ungated but
  unrun. `grep -c '€\|EUR' docs/layer3/coverage.md` is unchanged.
- The PR body states the run id and a dated cost bound, and the epic's cumulative bound is under
  €5. No account id (the configured `aws.account_id` in any form, or an ARN's account field) appears in the PR body or in any committed file.
