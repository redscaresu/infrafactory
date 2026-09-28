---
kind: code
status: ready
repo: mockway
touches: ["handlers/", "repository/"]
risk: high
---

# mockway returns an instance IP's binding as `server: {id, name}`, as the Scaleway API does

Found by #321 (2026-09-28): `GET /instance/v1/zones/{zone}/ips/{id}` returns a flat `server_id`.
The real API (scaleway-sdk-go `IP.Server *ServerSummary`) returns `server: {id, name}`, so the
provider (2.83.0) reads every IP's binding as `""` against mockway: a re-plan after apply shows
`server_id = ""` whether or not the IP is bound, and `policies/scaleway/no_public_endpoints.rego`
denies any IP from prior state. Fix at the source (sweep protocol); never hand-edit pitfalls.

**Done when:**
- A handler test: an IP attached to a server returns `server: {id, name}` naming that server on
  GET and list; an unattached IP returns `server: null`. The test fails on mockway `main`.
- A provider check (scaleway/scaleway 2.83.0, local mockway, fake credentials): apply a server with
  `ip_id = scaleway_instance_ip.web.id`, then a re-plan shows the IP's `server_id` equal to the
  server's id and plans no change. Recorded in a mockway example or contract test CI runs.
- Every place that builds an IP response (create, get, list, attach, detach, server update) returns
  the same shape; the flat `server_id` is gone unless the real API also returns it.
