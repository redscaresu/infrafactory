---
kind: code
status: ready
epic: fakeaws-step-one-surfaces
repo: fakeaws
depends_on: [fakeaws-provider-exact-pin, fakeaws-sts-caller-identity, fakeaws-sg-ip-permissions, fakeaws-instance-eni-and-ips, fakeaws-subnet-and-instance-attributes, fakeaws-refuse-unknown-ami, fakeaws-ssm-parameter-store, fakeaws-sg-ingress-validation, fakeaws-ec2-tags]
touches: ["examples/working/web_step_one/ (new)", "examples/web_step_one_state_test.go (new)", "AGENTS.md", "CHANGELOG.md"]
---

# Item 2 + item 3 (state): web_step_one applies env-only and exactly pinned; state carries the fake account

examples/working/web_step_one as epic bullet 2 specifies. The instance uses the AL2023 fixture AMI. The provider block has region, s3_use_path_style, default_tags and allowed_account_ids = ["000000000000"] (a safety deviation, see contradictions), with no keys, skip_* or endpoints. New gated examples/web_step_one_state_test.go uses smokeEnv (never a bare exec env) to apply, read `tofu show -json`, and destroy. Gaps it surfaces are fixed at source with a test; this story is alone in its wave. Refresh the AGENTS.md:146 contract list.

**Done when:**
- In the provider-smoke job, TestProviderSmokeWorking/web_step_one passes (apply, plan -detailed-exitcode = 0, destroy)
- In the provider-smoke job, the state test fails unless aws_instance.public_ip is non-empty and every arn and owner_id is non-empty and contains 000000000000
- grep for access_key, secret_key, skip_ and endpoints in web_step_one/main.tf finds nothing
- knownRed holds only eks_cluster and s3_bucket (owner: follow-up); coverage-audit and contract_audit_test are green; the AGENTS.md contract list equals `grep -rho 'CRITICAL\[[^]]*\]' handlers/*.go | sort -u`
