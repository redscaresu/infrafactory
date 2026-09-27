---
kind: lead
epic: firewall-end-to-end
status: blocked
blocked_by: repair-loop-loses-earlier-fix, layer3-allows-strcontains
---

# Does the generator now write a locked-down security group?

**Answered at the HCL level, 2026-09-27.** In run `20260927T163652Z` the generator wrote
`scaleway_instance_security_group` with `inbound_default_policy = "drop"`, an inbound rule for the
service port only, and `security_group_id` on the server — in both iterations that produced HCL,
with no criterion asking for it. Earlier the same day a seeded `test` run proved that shape holds
against real Scaleway: port 22 and 443 blocked, HTTP served.

**Remaining:** a generated `run --holdout` that converges and applies. That run ended `stuck`
before Layer 3: the generator left out the private network (iterations 1 and 3) and used
`strcontains()` (iteration 2). Both are now stories.

**Done when:** a generated `run --holdout` of `web-live-paris` passes the holdout against real
Scaleway. The lead runs it: real cloud.
