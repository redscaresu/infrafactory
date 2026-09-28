---
kind: code
status: blocked
blocked_by: [policy-loader-skips-test-files, rego-test-harness, deny-state-layer3-claim-removed]
epic: policy-correctness
depends_on: [policy-loader-skips-test-files, rego-test-harness, deny-state-layer3-claim-removed]
touches: ["policies/scaleway/region_restriction.rego", "policies/scaleway/region_restriction_test.rego"]
risk: high
---

# Scaleway region_restriction checks a zonal resource's zone against params.region at plan time

On 2.83.0, scaleway_instance_server has zone and no region. The plan region rule (:5-15) never sees zonal resources, and the zone rule (:17-28) needs a params.zone that no scenario sets. Add a plan rule, zone startswith params.region, mirroring deny_state :76-86. Absent params stays silent: TestRegionRestrictionFindsNothingWithoutParams pins that (see declined). Keep 'Layer 2 mock'. Outcome: all 18 Scaleway scenarios name it with fr-par. A zonal resource outside fr-par now fails at Layer 1 instead of Layer 2. Existing Go tests do not change: opa_test.go:128-136 still denies on its region fixture, and holdout_test.go:568 is state-only. The e2e stubs use fr-par-1.

**Done when:**
- Rego test: scaleway_instance_server with zone nl-ams-1 and no region attribute, and params.region fr-par, denies at plan (fails today)
- Rego test: zone fr-par-2 with params.region fr-par does not deny
- The harness mutation check kills every deny/deny_state body
