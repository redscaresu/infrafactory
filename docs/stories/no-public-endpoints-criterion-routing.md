---
kind: code
status: later
epic: policy-correctness
risk: high
---

# The `no_public_endpoints` criterion with `target: database` reaches a database policy

Found while scoping policy-correctness (2026-09-28). All three Scaleway scenarios that name
`no_public_endpoints` do so with `target: database` (`web-app-paris.yaml:57-58`,
`mysql-ha-paris.yaml:45-46`, `private-lb-db-paris.yaml:39-40`), but `infrafactory.yaml:239` routes
the criterion to `policies/scaleway/no_public_endpoints.rego`, a server-IP policy. Its kept
`server_id` rule also denies, on a re-plan, the pattern `pitfalls/scaleway.yaml:46` prescribes
(`ip_id = scaleway_instance_ip.web.id`): an existing ADR-0017 policy/pitfall conflict.

**Needs the user's decision before it is ready:** it changes the outcome of three proven Scaleway
scenarios. Options: route the criterion to `no_public_database`, or scope `no_public_endpoints` to
a target and resolve the pitfall conflict.

**Done when:** the three scenarios' `no_public_endpoints` criterion is evaluated by a policy that
checks the database, proven by a Rego test and an e2e whose outcome is named in the PR; the
ADR-0017 conflict with the ip_id pitfall no longer fires.
