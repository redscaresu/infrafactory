---
kind: code
status: blocked
blocked_by: [policy-loader-skips-test-files, rego-test-harness, deny-state-layer3-claim-removed]
epic: policy-correctness
depends_on: [policy-loader-skips-test-files, rego-test-harness, deny-state-layer3-claim-removed]
touches: ["policies/aws/no_public_db.rego", "policies/aws/no_public_db_test.rego", "internal/harness/aws_no_public_db_test.go"]
---

# AWS no_public_db loses a deny_state that reads keys fakeaws never exports

deny_state (:16-22) reads rds.instances[].publicly_accessible. fakeaws exports rds.db_instances[] without that field (../fakeaws/handlers/rds.go:843-861), so the rule can never fire. Delete it, and a criterion naming the policy then reports an honest skip (policy_packages.go:97-115). Test the plan rule. Outcome: none, since no AWS scenario names no_public_endpoints or no_public_database. No existing Go test covers this file.

**Done when:**
- internal/harness/aws_no_public_db_test.go fails if PolicyFileDefinesRule(policies/aws/no_public_db.rego, "deny_state") is true (fails today)
- Rego test: publicly_accessible = true denies, and the harness mutation check kills the deny body
