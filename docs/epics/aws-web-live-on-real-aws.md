---
status: active
hld: 2026-09-27-aws-web-stack
depends_on: [aws-layer3-wiring-proof, aws-layer3-claim-sweep-reap]
---

# A generated aws-web-live applies to the real member account, serves the pinned image at the address the holdout then dials, the holdout finds 22 and 443 closed and would have failed had 22 been open, the sweep finds the scope empty, and a real sandbox_deploy failure teaches pitfalls/aws.yaml (Goals 1, 2, 5; Design § Step one is run and test).

**Done when:**
- One `infrafactory run aws-web-live --holdout` with sandbox_deploy enabled ends target_reached on
  real AWS, and its iteration.json shows: the instance's ImageId equals the id aws_ami_resolve read
  from the real public parameter; account_check passed; user_data_check passed, naming the
  instance; the `real_probe` record has HTTP 200 from aws_instance.public_ip as read from
  terraform-live.tfstate; the holdout's records dial THAT SAME address and find 22 and 443 blocked
  and 80 open; aws_scope_sweep proved the account empty; aws_scope_release passed. Run id and a
  dated cost bound in the PR body.
- A mutation run whose security group opens 22 to 0.0.0.0/0 (a stub agent serving a fixture, not
  the model; default_deny_ingress.rego bypassed narrowly in a throwaway config, recorded): the
  holdout fails at that IP's :22, the run ends without repair (ADR-0033 § 5), destroy and sweep still
  run, and the scope is empty afterwards.
- Goal 5: a sandbox_deploy-stage failure on real AWS yields a learned aws_ entry in
  pitfalls/aws.yaml through the existing extractors. Naturally occurring in the first runs, or
  induced by a fixture whose fault both the mock and the gate pass; the story records why the mock
  passes it. An entry learned from an authorization, transient or mock-gap failure does not count.
  A run ending repair_budget_exhausted or stuck without a learned entry is a pipeline bug and does
  not close this; a filed bug does not close this.
- The legs the planted-leak proof did not reach (gate-lift, #405): `test` and `run` take and
  release the claim on real AWS; a second `test` is refused naming the holder; the failed-apply
  sweep runs; the interrupt prints from `run` and `test` are seen and name
  `reap --take-over <holder>`, which then releases; `tofu destroy`'s IAM actions are sent and none
  refused.
- The IAM user's action list is fixed by running apply, destroy, sweep and reap. It lives in
  docs/layer3/aws/iam-policy.json, held by its closed-set test (ADR-0040 decision 14), including
  ssm:GetParameter on the AL2023 public path; docs/operations.md describes it and points at it
  without enumerating.
- The AWS probe window is measured (first success after apply across the runs) against the one
  shared real_probes value; that value is widened only if AWS needs it, and the measurement is
  recorded, dated, beside it. No per-cloud field.
- docs/layer3/coverage.md flips the aws-web-live row from 'runnable, unrun' to **runnable** with its
  run id and date (no euro figure: coverage.md keeps none on purpose). AGENTS.md § Layer 3 and
  CONCEPT.md (§ Real Scaleway Deploy, § 11) describe AWS Layer 3: credential contract, scope, probe
  contract.

**Checks before every stage:** before each real run: a throwaway worktree off origin/main; nothing
held or left (`aws ssm get-parameter` on the claim parameter shows none, and
`infrafactory reap --dry-run` sweeps empty); the cumulative cost bound plus this run's worst case,
dated, under €5; the user's go recorded. After each run: aws_scope_sweep and aws_scope_release
pass (else `reap --take-over <holder>` before anything else); a CloudTrail errorCode check over the
run window; the run dir copied out of the worktree. Before each PR: no account id (the configured `aws.account_id` in any form, or an ARN's account field) in the diff or
the PR body.

**Out of scope:** The load balancer; the live path; GCP or Genesys Layer 3; scheduled or CI runs;
account creation or closure; a per-cloud probe window field; any code change other than the code
stories listed under Built by (other defects found here are filed against their epics). There is no
layer3 GitHub environment approval: its workflow was deleted in #240, and each real run's **You:**
go is the approval.

**Constraints:** Real money: €5 cap, serial runs, run from a throwaway worktree (docs/operations.md
§ Layer 3). No tool measures cost: each real run's cost is a dated upper bound from instance and
address hours at list price, kept in PR bodies. Never hand-edit pitfalls. Verify against the thing
itself: a green fakeaws run is not evidence. Read operations.md § Layer 3 (AWS) first. Run evidence
under `.infrafactory/runs/` is gitignored and lost with the worktree, so it is copied out and quoted
in PR bodies. Preceded by aws-layer3-wiring-proof.

**Built by:** agent code stories first (no real cloud), then the lead's real-AWS stories, strictly
serial; aws-web-live-on-real-aws-close's PR closes the epic.
- Wave 1, agents: aws-learned-pitfall-account-id-scrub, aws-interrupt-names-take-over,
  fakeaws-admin-seed-image (repo fakeaws), then aws-layer2-seeds-resolved-ami; and
  aws-layer3-run-evidence once aws-layer3-stage-order-test merges. Stories sharing
  internal/cli/test_command.go run one at a time.
- Wave 2, agent: aws-scope-iam-policy-grants-the-apply, once aws-web-live-layer2-llm-run gives it
  the Layer 2 HCL.
- Wave 3, lead + You: aws-layer3-iam-measured, then aws-layer3-claim-legs.
- Wave 4, lead + You, serial: aws-web-live-real-run, aws-web-live-holdout-mutation,
  aws-learned-pitfall-from-real-aws.
- Wave 5, lead: aws-probe-window-measured, then aws-web-live-on-real-aws-close.

**Areas:** `docs/layer3/aws/iam-policy.json` and `internal/harness/aws_scope_policy_test.go`;
`docs/operations.md`; `docs/layer3/coverage.md (one row, totals)`; `docs/layer3/real-vs-mock-deltas.md`;
`internal/cli/` and `internal/harness/` (the code stories); `internal/generator/` (the scrub);
fakeaws `handlers/admin.go`; `AGENTS.md`; `CONCEPT.md`; `infrafactory.yaml (probe window line)`;
`pitfalls/aws.yaml (pipeline-written only)`.
