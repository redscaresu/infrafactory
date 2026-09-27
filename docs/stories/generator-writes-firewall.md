---
kind: lead
epic: firewall-end-to-end
status: blocked
blocked_by: generator-self-review-hang
---

# Does the generator now write a locked-down security group?

ADR-0034 lifted the pitfall that forbade a security group, but no visible criterion in
`web-live-paris` asks for one.

**Half proven, 2026-09-27.** A seeded `test` run against real Scaleway, with a declared group
defaulting to drop, showed port 22 and 443 blocked and HTTP served through the load balancer,
so the allowlist, `default_deny_ingress` and the probes all hold when the group is written.

**Remaining:** whether the generator *chooses* to write one. Run `run --holdout` on
`web-live-paris`.

**Done when:** a generated run passes the holdout, or fails it and `server-firewall-rule` becomes
`ready`.
