---
kind: lead
status: blocked
blocked_by: [decide-aws-web-live-llm-run]
epic: aws-layer3-wiring-proof
depends_on: [aws-layer3-ami-resolve-wiring, aws-layer3-stage-order-test, aws-layer3-run-checklist, decide-aws-web-live-llm-run]
touches: ["docs/epics/aws-layer3-wiring-proof.md (delete)", "docs/hld/2026-09-27-aws-web-stack.md", "STATUS.md"]
---

# The one real-LLM Layer 2 run of aws-web-live against fakeaws, and the PR that closes the epic

Lead-run, LLM cost, no real cloud: sandbox_deploy stays off, so the resolve, claim and apply
never run.

The lead does not start the run until decide-aws-web-live-llm-run records the user's go, and
tells the user the run id when it ends.

The lead runs fakeaws from a clone at the CI pin (ci.yml:224, FAKEAWS_SHA; worktrees share a
stale ../fakeaws) and `infrafactory run scenarios/training/aws-web-live.yaml` with the default
config. The gate does not run at Layer 2 (generate_command.go:824, test_command.go:700), so the
lead runs it over the run's generated snapshot from an uncommitted test in a scratch worktree, with
AMI harness.AWSLayer2AMI, awsAdmittedInputs's root, Region us-east-1 (the Layer 2 fallback and the
scenario's region_restriction; the default config has no aws.region) and UserData from
renderAWSUserData of aws-web-live.yaml's service — and pastes the code and output in the PR. No
holdout: it needs sandbox_deploy (holdout.go:347). A run ending repair_budget_exhausted or stuck on
a non-mock failure is a pipeline bug (AGENTS.md:148-152): file it as a story, do not re-run
without a new go-ahead. The closing PR deletes docs/epics/aws-layer3-wiring-proof.md, marks the
epic done in the HLD's `## Epics` (docs/hld/2026-09-27-aws-web-stack.md:478) and moves STATUS.md
Now to aws-web-live-on-real-aws.

**Done when:**
- The run's terminal reason is target_reached, with its run id in the closing PR.
- Its final iteration.json shows a state_policy stage with status pass (not skip) whose detail
  counts default_deny_ingress among the policies evaluated, and the topology stage passing
  http_probe compute:80.
- The lead captures fakeaws DescribeSecurityGroups for the run before any reset and pastes it in
  the PR: non-empty ip_permissions, only tcp/80 from 0.0.0.0/0.
- sha256 sums of the run's generated/ are recorded at run end and quoted in the PR before the
  gate runs; the AWS gate over that snapshot returns no problems.
- The closing PR deletes the epic file, marks the HLD line done, updates STATUS.md Now, and
  passes CI.
