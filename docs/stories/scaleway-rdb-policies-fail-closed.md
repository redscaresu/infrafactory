---
kind: code
status: ready
epic: policy-correctness
depends_on: [policy-loader-skips-test-files, rego-test-harness, deny-state-layer3-claim-removed]
touches: ["policies/scaleway/encryption_at_rest.rego", "policies/scaleway/encryption_at_rest_test.rego", "policies/scaleway/no_public_database.rego", "policies/scaleway/no_public_database_test.rego"]
risk: high
---

# Behaviour change: Scaleway encryption_at_rest and no_public_database deny on null and absent values, not only on false

In Rego, null and [] are not false, so `not x` passes them. encryption_at_rest.rego:8: an omitted optional bool renders null. Unverified: the captured 2.83.0 plan decides, and if it renders false the rule stays and the test pins it. no_public_database.rego:9: an absent block renders []. deny_state :20: mockway's public endpoint carries "private_network": nil (../mockway/repository/repository.go:2475-2482), so it never denies against the mock. Fix: deny unless encryption_at_rest == true, unless private_network has an element, and unless the endpoint's private_network is an object. encryption_at_rest stays plan-only: holdout_test.go:562-668 depends on it. Unchanged Go tests: state_policy_test.go:94-110 (hand-shaped false, still 1 denial) and state_policy_params_test.go:100-102 (deny_state still defined). Outcome at Layer 1, on every Scaleway plan: full-stack-paris, incremental-project-paris, mysql-ha-paris, private-lb-db-paris and web-app-paris deny an RDB that omits either. No criterion names no_public_database, and the e2e stubs set both (full_stack_paris, web_app_paris, scaleway_services) and still pass.

**Done when:**
- Rego test: captured scaleway_rdb_instance with encryption_at_rest omitted denies
- Rego test: captured scaleway_rdb_instance with no private_network block denies (fails today)
- Rego test: a mockway rdb instance whose endpoint has private_network null denies via deny_state (fails today). One whose private_network is an object does not
- The harness mutation check kills every body in both files
