---
status: later
hld: 2026-09-27-aws-web-stack
depends_on: [aws-layer3-wiring-proof, aws-layer3-claim-sweep-reap]
---

# A generated aws-web-live applies to the real member account, serves the pinned image at the address the holdout then dials, the holdout finds 22 and 443 closed and would have failed had 22 been open, the sweep finds the scope empty, and a real sandbox_deploy failure teaches pitfalls/aws.yaml (Goals 1, 2, 5; Design § Step one is run and test).

**Done when:**
- One `infrafactory run aws-web-live` with sandbox_deploy enabled ends target_reached on real AWS, and the run evidence shows: the instance's ImageId equals the id the resolver read from the real public parameter; the account-id check passed; DescribeInstanceAttribute userData equals the rendered script; http_probe returned HTTP 200 from aws_instance.public_ip as read from terraform-live.tfstate; the holdout dialed THAT SAME address for 22 and 443 before destroy and found both blocked; the sweep proved the account empty; the claim was released; the run's cost is under the €5 cap. Run id recorded.
- A mutation run with the security group opening 22 to 0.0.0.0/0 (fixture, not model): the holdout fails, the run ends without repair (ADR-0033 § 5), destroy and sweep still run, the scope is empty afterwards.
- Goal 5: a sandbox_deploy-stage failure on real AWS yields a learned aws_ entry in pitfalls/aws.yaml through the existing extractors. Naturally occurring in the first runs, or induced by a fixture whose fault both the mock and the gate pass (for example a subnet CIDR outside the VPC block, if fakeaws does not check it; the story picks one and records why the mock passes it). A run ending repair_budget_exhausted or stuck without a learned entry is a pipeline bug and does not close this; a filed bug does not close this.
- The legs the planted-leak proof did not reach (aws-layer3-gate-lift): `test` and `run` take and release the claim on real AWS; a second `test` is refused naming the holder; the failed-apply sweep runs; the interrupt prints from `run` and `test` are seen; `tofu destroy`'s IAM actions are sent and none refused.
- The IAM user's action list is fixed by running apply, destroy, sweep and reap, and written ONCE in docs/operations.md § Layer 3 (AWS) 'IAM user policy', including ssm:GetParameter on the AL2023 public path. Single owner of that list.
- The AWS probe window is measured (first HTTP 200 after apply across the runs) and set in the per-cloud field aws-layer-neutral-hcl added; the value and the measurement are recorded.
- docs/layer3/coverage.md flips the aws-web-live row from 'runnable, unrun' to **runnable** with its run id and cost (the doc's S152 contract; the row and structure are aws-layer3-gate's). AGENTS.md § Layer 3 (AGENTS.md:158) and CONCEPT.md (:73-75, 465-479, 610-616) describe AWS Layer 3: credential contract, scope, probe contract.

**Out of scope:** The load balancer; the live path; GCP or Genesys Layer 3; scheduled or CI runs; account creation or closure; any code change other than the measured window value and the docs (code defects found here are filed against their epics).

**Constraints:** Real money: €5 cap, serial runs, run from a throwaway worktree (docs/operations.md:47-50). Never hand-edit pitfalls. Verify against the thing itself: a green fakeaws run is not evidence. AGENTS.md:158: read operations.md § Layer 3 first. Preceded by aws-layer3-wiring-proof.

**Built by:** the lead (real account or real cloud) — not for swarm builders. NOT for swarm builders: every Done is a real-cloud observation, an IAM edit or a doc that describes the observed run.

**Areas:** `docs/operations.md (AWS 'IAM user policy' subsection)`; `docs/layer3/coverage.md (one status cell)`; `AGENTS.md`; `CONCEPT.md`; `infrafactory.yaml (aws probe window value)`; `pitfalls/aws.yaml (pipeline-written only)`; `.infrafactory/runs/aws-web-live/ (evidence)`
