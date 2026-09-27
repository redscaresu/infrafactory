---
status: later
hld: 2026-09-27-aws-web-stack
depends_on: [policy-correctness, fakeaws-step-one-surfaces]
---

# The prohibition the holdout will check is a tested Rego policy that walks every AWS ingress shape, the aws-web-live scenario and its holdout exist and are discoverable, and the model is told which ingress shape to write.

**Done when:**
- policies/aws/ gains the ingress prohibition with Rego tests run through opa/v1/tester that fail when each deny and deny_state rule is removed. It walks all three shapes with no 'or': `ingress` blocks, aws_security_group_rule and aws_vpc_security_group_ingress_rule (the gate allowlist binds only when Layer 3 is enabled, internal/cli/test_command.go:611-615, while this policy runs on every AWS plan, so the allowlist cannot close a shape for it). No public-source rule admits 22, uses protocol -1 or 'all', spans 0/0, or a tcp/6 range covering 22 (0-65535 included). Public source = cidr_blocks, ipv6_cidr_blocks (::/0), cidr_ipv4, cidr_ipv6, including two-halves shapes; prefix_list_ids refused; security_groups, self, referenced_security_group_id not public; iam_instance_profile on aws_instance denied. Fixtures for each spelling.
- A testdata copy of the 2026-06-07 network.tf security group (from .infrafactory/runs/aws-instance/20260607T064123Z/generated/network.tf:25-28, which is gitignored, .gitignore:5): SSH from 10.0.0.0/16 passes the public-source rule; its 0.0.0.0/0 variant is denied.
- The deny_state half fails closed when a group in state has no ip_permissions field, tested against input captured from fakeaws's real /mock/state export (fakeaws-step-one-surfaces item 8), not a hand-written shape (undefined-is-not-false, internal/harness/policy_packages.go).
- scenarios/training/aws-web-live.yaml exists (single owner): service nginx 1.27 port 80 with ttl, compute small count 1, the ingress policy named as a criterion so its state half runs at Layer 2, region_restriction us-east-1, http_probe compute:80 reachable. A test loads it and asserts those criteria.
- scenarios/holdout/aws-web-live-unseen.yaml: connectivity public_internet -> compute port 22 blocked and port 443 blocked, mirroring scenarios/holdout/web-live-paris-unseen.yaml; a discovery test proves the holdout loader finds it for aws-web-live. The story decides whether to add connectivity to compute port 80 expect success as a real positive control, since a security group drops silently and 443 blocked proves nothing on an instance (contradiction 12); ADR-0033 rule 3 admits connectivity checks.
- prompts/aws/phase2_generate_hcl.md says which ingress shape to write (inline `ingress` blocks for step one, the shape fakeaws round-trips) and why; this is the single owner of that choice, which aws-web-stack-load-balancer inherits.
- ADR-0034 gains the third-cloud note.

**Out of scope:** The gate (aws-layer3-gate); any LLM or real-cloud run; fixing the four existing AWS policies (policy-correctness owns them).

**Constraints:** Depends on policy-correctness for the Rego test harness and inherits its record that deny_state never runs against real state: the holdout is the only real-state check. ADR-0034: a policy that walks two shapes fails open on the third. ADR-0028: a factual claim is a test. ADR-0033 rule 3: holdouts probe connectivity, http_probe and dns_resolution only.

**Built by:** agents. Swarm-buildable, wave 2, in parallel with aws-layer-neutral-hcl.

**Areas:** `policies/aws/ (new ingress policy + *_test.rego + testdata)`; `scenarios/training/aws-web-live.yaml (new)`; `scenarios/holdout/aws-web-live-unseen.yaml (new)`; `internal/scenario or internal/cli/holdout.go (discovery test)`; `prompts/aws/phase2_generate_hcl.md (ingress shape)`; `docs/decisions/0034`
