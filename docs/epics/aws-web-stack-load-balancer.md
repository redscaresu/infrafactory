---
status: later
hld: 2026-09-27-aws-web-stack
depends_on: [aws-web-live-on-real-aws]
---

# Step two: the same stack behind an ALB, proving the floor grows (Goal 4; Rollout 3), with fakeaws's ELBv2 and securityGroupRuleSet first.

**Done when:**
- fakeaws: ELBv2 (CreateLoadBalancer, target groups, listeners, RegisterTargets, Describe*, all into /mock/state) and Authorize* returning securityGroupRuleSet with rule ids, or the allowlist keeps refusing aws_vpc_security_group_*_rule at Layer 3 (the model-facing shape choice is aws-ingress-policy-and-holdout's; today the provider crashes on the rule resource, .swarm/moto apply-web-fakeaws-false.log:186); each with validation errors, coverage_matrix rows, LandedServices + cross_repo_parity_test.go mapping to the new scenario; the moto 17-resource ALB stack (.swarm/moto/tf) applies on fakeaws with the exact pin.
- infrafactory: a scenario with a public load balancer on 80 reaches target at Layer 2 with http_probe load_balancer:80 derived by deriveTopologyAWS; the gate admits aws_lb, aws_lb_target_group, aws_lb_listener, aws_lb_target_group_attachment with cost bounds (one LB, one listener on 80); the sweep list gains load balancers, target groups and listeners with reap deletes in dependency order; the holdout dials the LB on 22 and 443 blocked; the probe window is re-measured for an ALB health check.
- On real AWS: target reached, holdout passes, sweep empty; IAM user and SCP widened to elasticloadbalancing and recorded in docs/operations.md; docs/layer3-coverage.md row with cost (ALB ~$0.0225/h + LCU).

**Out of scope:** HTTPS/ACM, DNS, autoscaling, ECS/EKS, the live path, GCP.

**Constraints:** Same gate, policy and sweep rules as step one (S156d: new paths inherit unwritten rules). fakeaws constraints as fakeaws-step-one-surfaces. Real-cloud constraints as aws-web-live-on-real-aws. Scoped in detail only after step one has run: the HLD gives step two two paragraphs.

**Built by:** the lead (real account or real cloud) — not for swarm builders. Unchanged from v1 except the ingress-shape clause now defers to aws-ingress-policy-and-holdout. The fakeaws ELBv2 and gate/Layer 2 stories are swarm-buildable and may start after fakeaws-step-one-surfaces; the real-cloud Done and IAM/SCP widening are the user's. Split at /plan-epic time if ELBv2 should start before step one has run.

**Areas:** `../fakeaws/handlers/elbv2.go (new), ../fakeaws/handlers/ec2.go (Authorize*)`; `../fakeaws/coverage_matrix.yaml, ../fakeaws/handlers/regression_manifest.go`; `internal/e2e/cross_repo_parity_test.go`; `internal/harness/topology_derive.go`; `internal/cli/layer3_hcl_shape.go, layer3_hcl_shape_test.go`; `internal/harness/ (aws sweep collections)`; `internal/config/config.go, infrafactory.yaml, docs/layer3-coverage.md`; `scenarios/training/, scenarios/holdout/`; `prompts/aws/`; `docs/operations.md`
