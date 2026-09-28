---
kind: code
status: blocked
blocked_by: [policy-loader-skips-test-files, rego-test-harness]
epic: policy-correctness
depends_on: [policy-loader-skips-test-files, rego-test-harness]
touches: ["policies/scaleway/default_deny_ingress_test.rego", "policies/scaleway/vpc_required_test.rego", "policies/common/naming_test.rego"]
---

# Test-only: Rego tests for scaleway default_deny_ingress, scaleway vpc_required and common naming

No rule changes. If a captured shape shows one of these rules cannot fire, stop and report to the lead; do not fix the rule here. default_deny_ingress: the plan shape, the mockway security_groups state shape, and the API-default exclusion (:66-72). vpc_required: the singleton NIC, counted NIC, inline block and wrong-reference shapes (:48-160). common/naming runs on every AWS and Scaleway plan (infrafactory.yaml:73), so it is scoped in: test its exemptions (:5-31). No run outcome or existing Go test changes.

**Done when:**
- The harness mutation check kills every deny/deny_state body in the three files
- Rego test: a declared scaleway_instance_security_group left at 'accept' in plan and in state denies, and a project_default group in state does not
- Rego test: a counted server with an inline private_network pn_id referencing scaleway_vpc_private_network.main plus count.index does not deny
