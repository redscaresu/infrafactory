---
kind: code
status: blocked
blocked_by: [fakeaws-smoke-in-ci]
epic: fakeaws-step-one-surfaces
repo: fakeaws
depends_on: [fakeaws-smoke-in-ci]
touches: ["examples/*/*/main.tf (existing only)", "examples/working/env_endpoints/main.tf (new)", "examples/smoke_env_test.go (new)", "examples/provider_smoke_test.go", "concepts.md", "coverage_matrix.yaml", "README.md", "examples/README.md", "CHANGELOG.md"]
risk: high
---

# Item 1: pin hashicorp/aws 5.100.0; the harness runs tofu in a scrubbed env that fails closed; the env endpoint form is proven

Pin version = "5.100.0" in every existing examples/*/*/main.tf. It is the only binary the bake-off ran (.swarm/moto/plugin-cache/.../aws/5.100.0) and what ~> 5.70 resolves to today. SAFETY: this is the first story with env-only endpoints, so it adds examples/smoke_env_test.go::smokeEnv, used by every tofu call. It starts from os.Environ() with every AWS_* var removed, then adds fake keys, AWS_REGION=us-east-1, AWS_EC2_METADATA_DISABLED=true, and AWS_CONFIG_FILE and AWS_SHARED_CREDENTIALS_FILE pointing at non-existent paths under t.TempDir(). It sets AWS_ENDPOINT_URL_<SVC> for every LandedServices id, plus STS at /sts and SSM at /ssm/region/us-east-1 ahead of their handlers (secretsmanager.go:19 pattern), at the URLs the examples' endpoints blocks use. apply, plan and destroy also get HTTPS_PROXY=HTTP_PROXY=http://127.0.0.1:9 and NO_PROXY=127.0.0.1,localhost; init gets no proxy, since it must reach registry.opentofu.org. New examples/working/env_endpoints: region, access_key/secret_key "fake", skip_credentials_validation, skip_requesting_account_id, no endpoints block, one aws_vpc. An ad-hoc run outside the harness then authenticates as nobody on real AWS. Record the pin in README.md:119-123, examples/README.md:173, concepts.md:494,508 and coverage_matrix.yaml:30-34, saying the infrafactory side (prompts, e2e harness) lands in aws-layer-neutral-hcl.

**Done when:**
- Ungated unit test: with AWS_PROFILE, AWS_ACCESS_KEY_ID, AWS_SESSION_TOKEN and AWS_ENDPOINT_URL set via t.Setenv, smokeEnv's output contains none of their values; it has fake keys, AWS_EC2_METADATA_DISABLED=true and both file vars at paths that do not exist; the apply/plan/destroy env carries the dead proxy and the init env does not
- Ungated unit test: every handlers.LandedServices id has an AWS_ENDPOINT_URL_<SVC> in smokeEnv (fails naming any id without one); STS and SSM entries exist
- Gated test in the provider-smoke job: env_endpoints with AWS_ENDPOINT_URL_EC2 removed and AWS_MAX_ATTEMPTS=1 fails within a 2-minute exec.CommandContext, with output naming 127.0.0.1:9; with the full env, TestProviderSmokeWorking/env_endpoints passes
- Every examples/*/*/main.tf says version = "5.100.0"; `grep -rn '~> 5\.70' --exclude-dir=.git` hits only concepts.md's history sentence
- The provider-smoke job is green with knownRed unchanged
