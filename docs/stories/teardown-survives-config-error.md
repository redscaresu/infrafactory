---
kind: code
risk: high
status: ready
touches: [internal/cli/test_command.go, internal/harness, internal/cli/run_project_lifecycle.go]
---

# A failed apply must not leak a real stack when destroy cannot evaluate the config

Run `20260927T171147Z` (generated `web-live-paris`, iteration 1): the apply created a full stack —
server, IP, volume, security group, load balancer and IP, private network, VPC — then failed on a
config error (`server_ips = [try(...private_ips[0].address, "")]`, an empty IP). `tofu destroy`
evaluated the same config and failed the same way, so the run project and everything in it
survived. The next iteration reused the output directory, so no state referenced the stack.
`live/stray_run_projects` reported it, correctly, but nothing removed it; the operator tore it
down through the API.

**Done when:** when destroy fails after an apply created resources, teardown still removes them —
for example by deleting what the run project contains through the API, since the run owns that
project (ADR-0025) — and the orphan sweep proves the project empty before deleting it. A test
reproduces "apply creates, then fails on config; destroy fails on the same config" and fails
without the fix. Any API-side deletion is scoped to the run's own stamped project and refuses
any other (ADR-0023). No real-cloud runs; fakes only. The lead canaries it afterwards.
