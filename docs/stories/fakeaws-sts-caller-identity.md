---
kind: code
status: ready
epic: fakeaws-step-one-surfaces
repo: fakeaws
depends_on: [parity-exempt-sts-ssm, fakeaws-provider-exact-pin]
touches: ["handlers/sts.go (new)", "handlers/sts_test.go (new)", "handlers/handlers.go", "handlers/regression_manifest.go", "coverage_matrix.yaml", "examples/working/env_endpoints/main.tf", "AGENTS.md", "README.md", "CHANGELOG.md"]
risk: high
---

# Item 3 + item 12 (sts): STS GetCallerIdentity returns account 000000000000; sts lands

handlers/sts.go: Query-RPC at POST /sts (the flat pattern of iam.go:25-27). GetCallerIdentity returns Account=awsproto.FakeAccountID (awsproto.go:33), a fixed UserId and a documented, fixed Arn arn:aws:iam::000000000000:user/<name>. Other STS actions answer 501 UNIMPLEMENTED. The same PR appends "sts" to LandedServices (regression_audit_test.go:50-56) and adds registerSTSRoutes (handlers.go:86-102). It adds a coverage_matrix row for aws_caller_identity: scenario_resource_type omitted, working_dir_name env_endpoints, misconfigured and updates exempt with reasons. The contract pair lives in sts_test.go. env_endpoints drops its skip_* flags and gains allowed_account_ids = ["000000000000"] and data.aws_caller_identity. Update the AGENTS.md:53-56 wire-format line and the README.md service table.

**Done when:**
- The contract test asserts Account == awsproto.FakeAccountID, a non-empty UserId and an Arn containing :000000000000:, and fails if sts.go uses another literal
- AssumeRole returns 501 with an UNIMPLEMENTED log line
- In the provider-smoke job, env_endpoints passes with no skip_* flags; its allowed_account_ids makes the apply fail if STS answers any other account
- regression-seed-audit and coverage-audit are green; smokeEnv's LandedServices endpoint test passes with sts landed
