---
kind: code
status: ready
epic: policy-correctness
depends_on: [policy-loader-skips-test-files, rego-test-harness, aws-vpc-required-fails-closed]
touches: ["policies/scaleway/no_public_endpoints.rego", "policies/scaleway/no_public_endpoints_test.rego", "internal/harness/opa_test.go", "internal/e2e/full_stack_paris_test.go"]
---

# Scaleway no_public_endpoints drops two rules that read attributes the provider never had, and states what the third checks

Behaviour-neutral by design; see contradictions for the lead's decision. Deleted: server values.public_ip (:5-15) and ip values.server (:17-26). Neither attribute exists in the 2.76.0 or 2.83.0 schema, so neither rule ever fired. Kept: ip values.server_id != null (:28-37). It fires only when a plan carries a bound IP from prior state; a fresh plan's server_id is computed and unknown. The comment states what the policy does not check: a fresh ip_id (configuration only), ip_ids, and enable_dynamic_ip. opa_test.go:330-339 fed values.server, an attribute the provider never had. Rewrite it to values.server_id, still expecting 1. full_stack_paris_test.go:208-211 becomes 'an IP whose server_id is known in the plan, after a prior apply bound it'. Outcome: none. web_app_paris_test.go:147,155 (ip_id stubs) and pitfalls/scaleway.yaml:46 are unaffected. Wave 3 because aws-vpc-required-fails-closed edits opa_test.go in wave 2.

**Done when:**
- Rego test: a re-plan shape with scaleway_instance_ip values.server_id set denies
- Rego test: a fresh plan with ip_id = scaleway_instance_ip.web.id, present only in configuration expressions.ip_id.references and absent from planned_values, does not deny (the pinned limit)
- Rego test: ip_ids = [scaleway_instance_ip.web.id] does not deny, and enable_dynamic_ip = true does not deny (pinned)
- opa_test.go 'no public endpoints' case passes with values.server_id and 1 failure
- The harness mutation check kills the remaining deny body
