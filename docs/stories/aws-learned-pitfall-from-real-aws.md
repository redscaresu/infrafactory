---
kind: lead
status: blocked
blocked_by: [aws-web-live-holdout-mutation, aws-learned-pitfall-account-id-scrub, aws-layer2-seeds-resolved-ami, aws-web-live-layer2-llm-run]
epic: aws-web-live-on-real-aws
depends_on: [aws-web-live-holdout-mutation, aws-learned-pitfall-account-id-scrub, aws-layer2-seeds-resolved-ami, aws-web-live-layer2-llm-run]
touches: ["pitfalls/aws.yaml (pipeline-written only)", "docs/layer3/real-vs-mock-deltas.md (only if the fault is a fakeaws/real gap)"]
risk: high
---

# A real sandbox_deploy failure teaches pitfalls/aws.yaml an aws_ entry through the existing extractors, and the failed-apply sweep is seen

The epic's third bullet (Goal 5), plus the failed-apply sweep from the fourth. The epic's **Checks
before every stage** apply.

**You:** say go before the run, in the lead's session (none is needed if the saved diff below
qualifies). The lead does not start without it, and tells you the run id and the cost bound when it
ends.

If aws-web-live-real-run saved a qualifying learned entry, commit that diff and skip the run.
Otherwise induce one. A stub `agent.claude.command` serves iteration 1 a fixture whose fault fakeaws
and the gate both pass (for example a subnet CIDR outside the VPC block; check fakeaws at
FAKEAWS_SHA first and record why it passes), and iteration 2 the clean fixture. The stub is
stateless per call, so it tells the iterations apart by a counter file in the throwaway worktree (or
another stated method); the PR body records which. Learning runs on any non-mock failure detail that
names an aws_ resource (internal/cli/run_command.go:290-309, 551-576;
internal/generator/pitfalls_learn.go:526-535), so the entry alone proves nothing: an entry learned
from AccessDenied or UnauthorizedOperation is a permissions fault, a transient error is not a
lesson, and a mock-gap classification belongs to the mock. A run ending stuck or
repair_budget_exhausted with no entry is a pipeline bug: file it and stop; do not re-run without a
new go. Commit exactly the pipeline-written hunk from the throwaway worktree, never edited. Since
the chosen fault is one fakeaws passes and real AWS refuses, record it in
docs/layer3/real-vs-mock-deltas.md in the same PR (or file a fakeaws story and link it there).

**Done when:**
- Before the go (if a run is needed): the cumulative cost bound of the epic's real runs so far plus
  this run's worst case is written down, dated, and is under €5.
- iteration 1's iteration.json fails at layer sandbox_deploy, followed by aws_scope_sweep and
  aws_scope_release passing. The run ends target_reached, and app.log has a pitfall_emitted (or
  prescriptive_*_learned) event for an aws_ resource.
- The PR quotes the failing sandbox_deploy detail, states that it is not an authorization
  (AccessDenied, UnauthorizedOperation), transient or mock-gap failure, and says why it is a
  deterministic HCL lesson.
- The PR's only pitfalls diff is one added entry in pitfalls/aws.yaml with resource aws_*,
  learned_layer sandbox_deploy and source learned (or descriptive). It has no account id (the configured `aws.account_id` in any form, or an ARN's account field), and
  `git diff` of it equals the worktree's diff byte for byte.
- The fakeaws/real gap is in docs/layer3/real-vs-mock-deltas.md or filed as a fakeaws story.
- pitfalls_source_ratchet_test.go (1000-byte cap) and TestPitfallsSourceEnum pass. The PR body states
  the run id and the dated cost bound, with no account id (the configured `aws.account_id` in any form, or an ARN's account field).
