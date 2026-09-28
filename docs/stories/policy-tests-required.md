---
kind: code
status: ready
epic: policy-correctness
depends_on: [policy-loader-skips-test-files, rego-test-harness, aws-region-restriction-honours-params, aws-encryption-sse-binds-its-bucket, aws-vpc-required-fails-closed, aws-no-public-db-drops-dead-state-rule, scaleway-region-restriction-zonal-plan-rule, rego-tests-for-unchanged-policies, scaleway-rdb-policies-fail-closed, scaleway-no-public-endpoints-drops-dead-rules]
touches: ["internal/harness/rego_policy_test.go", "internal/harness/testdata/regotest/"]
---

# Every policy under policies/aws, policies/scaleway and policies/common must have a mutation-checked _test.rego

The harness's per-file opt-in becomes a requirement for aws/, scaleway/ and common/. GCP and Genesys stay out, per the epic. This closes the epic's first done-when. No run outcome or existing Go test changes.

**Done when:**
- Self-test: a fixture dir missing one _test.rego yields a problem
- Deleting any one _test.rego under policies/aws, policies/scaleway or policies/common fails go test ./internal/harness
- Over the real tree, removing any deny/deny_state body in those dirs fails go test ./internal/harness
