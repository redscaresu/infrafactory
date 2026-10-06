---
status: later
hld: 2026-09-27-aws-web-stack
depends_on: [aws-layer3-seal-and-dispatch, aws-layer3-claim-sweep-reap, aws-layer-neutral-hcl, aws-ingress-policy-and-holdout, aws-layer3-gate]
---

# The whole AWS Layer 3 path runs in order against fakes before any real money: preflight, claim, AMI resolve, generate, gate, apply, account check, user-data check, http_probe, holdout, destroy, sweep, release, each failure short-circuiting; and one real-LLM Layer 2 run of aws-web-live reaches its target.

**Done when:**
- A CI test with injected fakes (fake doer for STS/SSM/EC2 sweep, fakeaws for the apply, a fake probe dialer) runs `cloud: aws` with sandbox_deploy enabled through test_command.go and asserts the stage order: preflight GetCallerIdentity, stamp and default-VPC checks, claim taken (ADR-0040), AMI resolved, generation, gate, apply, account-id check, user-data compare, http_probe, holdout, destroy, sweep, claim released. The 'AMI resolved' stage calls ResolveAWSAMIFromSSM (aws-layer-neutral-hcl, ADR-0039 decision 7) exactly once per run, after preflight and before generation. Because deploy copies only .tf and .hcl into the workdir (deploy_command.go:27), this stage also writes infrafactory-user-data.sh into the workdir explicitly — it is not carried across by the copy step. For each stage, a fake that fails it ends the run at that stage with the recovery command where resources may exist, and the claim is kept when the sweep is not clean.
- The interrupt path is exercised with a fake signal: the claim is left and `infrafactory reap` is named; reap against the fakes releases it.
- One operator-run `infrafactory run aws-web-live` at Layer 2 (LLM, fakeaws) ends target_reached with the generated HCL passing the gate unchanged, the policy's state half evaluated against ip_permissions, and http_probe compute:80 derived true. Run id recorded in the epic's closing PR. This is the only LLM run before real cloud.
- docs/operations.md gains the AWS run checklist (what to source, what preflight will refuse, what a failure prints) so the first real run is a checklist, not an exploration.

**Out of scope:** Any real-cloud call; new gate or policy rules (file them against their epics).

**Constraints:** Verify against the thing itself: this proves wiring and order, not AWS behaviour; it is the last check before aws-web-live-on-real-aws. LLM cost is one run, operator-invoked.

**Built by:** agents. Swarm-buildable, wave 4, except the one LLM run which the lead triggers. Depends on aws-layer3-claim-sweep-reap's code stories, not on its real-cloud proof.

**Areas:** `internal/cli/ (Layer 3 path wiring test with injected fakes)`; `internal/e2e/ (AWS Layer 3 dry run against fakeaws)`; `docs/operations.md (AWS run checklist)`
