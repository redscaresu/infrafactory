---
status: later
blocked_by: generator-writes-firewall
---

# Require a firewall on the server, not only a real one when declared

Only if the generator still declares no security group. `default_deny_ingress` checks that a
declared group denies by default; it cannot require one, and the requirement cannot come from
the holdout (ADR-0033 §5). It has to be a rule about the server.
