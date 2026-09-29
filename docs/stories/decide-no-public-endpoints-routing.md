---
kind: operator
status: ready
---

# Decide: how the Scaleway `no_public_endpoints` criterion is routed, and whether servers may have public IPs

The user's call. Three proven Scaleway scenarios (`web-app-paris`, `mysql-ha-paris`,
`private-lb-db-paris`) name `no_public_endpoints` with `target: database`, but it reaches a
server-IP policy. Separately, the `ip_id` pitfall gives servers public IPs while `web-app-paris`
says instances must not be reachable from the internet; since mockway #31, Layer 2 sees that and
`web_app_paris` fails. The detail is in `no-public-endpoints-criterion-routing`, which this blocks.

Options: route the criterion to `no_public_database`; or scope `no_public_endpoints` to a target;
and either forbid public IPs on servers (retire the pitfall through the pipeline) or drop the
`web-app-paris` connectivity check.

**Done when:** the user's decision is written into `no-public-endpoints-criterion-routing` and this
file is deleted in the same PR.
