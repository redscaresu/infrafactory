---
status: active
hld: 2026-09-27-aws-web-stack
depends_on: [aws-layer3-seal-and-dispatch, aws-layer3-claim-sweep-reap, aws-layer-neutral-hcl, aws-ingress-policy-and-holdout, aws-layer3-gate]
---

# The whole AWS Layer 3 path runs in order against fakes before any real money: STS, AMI resolve, generate, gate, STS, stamp, default VPC, claim, apply, account check, user-data check, http_probe, holdout, destroy, sweep, release, each failure short-circuiting; and one real-LLM Layer 2 run of aws-web-live reaches its target.

**Done when:**
- `generate`, `run` and `test` at Layer 3 aws resolve the AMI and its root once per command,
  after STS and before generation or the gate, and a failed resolve ends the command before any
  model call or claim. No new flag: sandbox_deploy.enabled is the switch, off in infrafactory.yaml.
- A CI test in internal/cli with injected fakes (the awsLifecycle doer for STS/SSM/EC2,
  fakeSandboxDeployHarness for the apply, a fake RealProbe for http_probe and holdout) drives
  `run --holdout` for `cloud: aws` with sandbox_deploy enabled and the real gate, and asserts the
  order: STS GetCallerIdentity; the AMI resolve (SSM read of the AL2023 public parameter, then EC2
  DescribeImages of that id: once per run, before the first generation, ADR-0039 decision 7);
  generation; gate; STS again; stamp and default-VPC checks; claim taken (ADR-0040); apply;
  account-id check; user-data compare; http_probe; holdout; destroy; sweep; claim released. For
  each forward stage, a fake that fails it runs no later forward stage; once the apply has
  started, destroy and the sweep still run, the reap command is printed, and the claim is kept
  when the sweep is not clean. infrafactory-user-data.sh is in the output dir and equal to the
  render when the apply starts: generation writes it and the gate checks it; nothing copies it.
- An interrupt during the AMI resolve leaves no claim and names no reap; an interrupt after the
  claim leaves the claim and names `infrafactory reap` (already: aws_run_failure_test.go:256-290);
  reap against the fakes releases it (already: e2e/aws_reap_fakeaws_test.go:70).
- One operator-approved `infrafactory run aws-web-live` at Layer 2 (LLM, fakeaws at the CI pin)
  ends target_reached; the AWS gate, run by the lead over the run's generated snapshot with the
  Layer 2 AMI and the AL2023 root fixture, reports no problems; the policy's state half is
  evaluated against ip_permissions; http_probe compute:80 is derived true. Run id recorded in the
  epic's closing PR. This is the only LLM run before real cloud.
- docs/operations.md § Layer 3 (AWS) gains the AWS run checklist (what to source, what refuses
  and in what order, what a failure prints, and that the resolve is denied until
  aws-web-live-on-real-aws grants ssm:GetParameter on the AL2023 public path), held to the code's
  stage names by a test; coverage.md stops calling the AWS gate unbuilt.

**Checks before every story's PR:** `go test -tags noui ./internal/cli/... ./internal/harness/...`
passes with these guards unweakened: TestNoProductionCodeSetsTheLayer3HCLGate,
TestTheAWSClaimIsReleasedOnlyAfterACleanSweep, TestADR0023AmendmentRecordsTheGateLift,
TestAWSScopeSetupRunbookNamesWhatTheCodeReads, aws_sdk_audit_test.go and
aws_scope_policy_test.go; `make doc-hygiene` passes; no diff to docs/layer3/aws/iam-policy.json or
pitfalls/.

**Out of scope:** Any real-cloud call; new gate or policy rules (file them against their epics);
an e2e Layer 3 dry run against fakeaws (fakeaws cannot serve the sealed apply, and its
DescribeImages has no root data — a later need is a fakeaws story with a FAKEAWS_SHA bump).

**Constraints:** Verify against the thing itself: this proves wiring and order, not AWS
behaviour; it is the last check before aws-web-live-on-real-aws. LLM cost is one run,
operator-invoked.

**Built by:** agents. Swarm-buildable, wave 4, except the one LLM run which the lead triggers.
Stories: aws-layer3-ami-resolve-wiring; then aws-layer3-stage-order-test and
aws-layer3-run-checklist; then aws-web-live-layer2-llm-run, whose PR records the run id and
closes the epic.

**Areas:** `internal/cli/ (the AMI resolve stage in generate, run and test; the stage-order test
with injected fakes)`; `docs/operations.md (AWS run checklist)`; `docs/layer3/coverage.md`
