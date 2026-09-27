---
kind: code
status: blocked
blocked_by: [parity-exempt-sts-ssm, fakeaws-provider-exact-pin, fakeaws-sts-caller-identity, fakeaws-refuse-unknown-ami]
epic: fakeaws-step-one-surfaces
repo: fakeaws
depends_on: [parity-exempt-sts-ssm, fakeaws-provider-exact-pin, fakeaws-sts-caller-identity, fakeaws-refuse-unknown-ami]
touches: ["handlers/ssm.go (new)", "handlers/ssm_test.go (new)", "repository/ssm.go (new)", "repository/ssm_test.go (new)", "handlers/handlers.go", "handlers/admin.go", "handlers/regression_manifest.go", "coverage_matrix.yaml", "examples/working/ssm_parameter/ (new)", "examples/misconfigured/ssm_parameter_duplicate/ (new)", "AGENTS.md", "README.md", "CHANGELOG.md"]
risk: high
---

# Item 9 + item 12 (ssm): SSM Parameter Store with an atomic no-overwrite CAS and the AL2023 public parameter; ssm lands

handlers/ssm.go: JSON 1.1 (X-Amz-Target AmazonSSM.*) at /ssm/region/{region} (secretsmanager.go:19 pattern), the path smokeEnv already sets. PutParameter with Overwrite=false on an existing name returns 400 ParameterAlreadyExists atomically, and Overwrite=true bumps Version. GetParameter and DeleteParameter return ParameterNotFound when missing. Also model what aws_ssm_parameter calls at 5.100.0 (TF_LOG=DEBUG, AGENTS.md:102); everything else answers 501. Timestamps are epoch numbers (AGENTS.md:104-106). repository/ssm.go registers its table with prependResetTables (repository.go:230-237). The read-only public parameter /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64 returns the AL2023 fixture constant. gatherSSMStateReal goes into collectState (admin.go:130-140) and lists user parameters only (name, type, version, no values). The same PR adds "ssm" to LandedServices, the RegisterRoutes call, and a coverage_matrix aws_ssm_parameter row (scenario_resource_type omitted). The contract pair lives in ssm_test.go. Update AGENTS.md:53-56 and the README.md table. The examples carry explicit endpoints { ssm = "http://127.0.0.1:8082/ssm/region/us-east-1" }.

**Done when:**
- Contract test: a second PutParameter with Overwrite=false returns ParameterAlreadyExists with the value unchanged; 20 concurrent Overwrite=false puts on one name give exactly one 200; after DeleteParameter, GetParameter returns ParameterNotFound
- Tests: /mock/reset clears parameters; Snapshot/Restore round-trips one
- GetParameter on the AL2023 path returns the id that DescribeImages knows under an al2023 name
- In the provider-smoke job, working/ssm_parameter passes and misconfigured/ssm_parameter_duplicate passes (expected.txt ParameterAlreadyExists)
- /mock/state ssm lists the user parameter and not the public one; regression-seed-audit, coverage-audit and smokeEnv's LandedServices endpoint test are green
