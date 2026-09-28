---
kind: code
status: ready
epic: policy-correctness
depends_on: [policy-loader-skips-test-files, rego-test-harness, deny-state-layer3-claim-removed]
touches: ["policies/aws/region_restriction.rego", "policies/aws/region_restriction_test.rego"]
risk: high
---

# AWS region_restriction compares the provider's region and zones to params.region, and fails closed on a region it cannot read

Delete data.region_allowlist and its fallback (:15-19). Also delete the resource-values.region rule (:21-32): on 5.100.0 no generated type carries a plan-time region (aws_s3_bucket's is computed-only). Plan rules, active only when params.region is set, since they run on every AWS plan: check each configuration.provider_config entry named aws. A missing region key, a non-constant expression, or a constant other than params.region denies, and aws_ resources with no aws entry at all deny. Keep the availability_zone rule, comparing with startswith. Why deny instead of resolving var.X: the model's own provider block survives when cfg.Fakeaws.URL is empty (generate_command.go:356-359), and its region may come from AWS_REGION, which the plan cannot see. State: walk every collection for region and availability_zone. The comment must say that the Layer 2 region is the endpoint-path region infrafactory injects (generate_command.go:548-555; fakeaws handlers/sqs.go:28, ec2.go:35), so only availability_zone reflects the model. The comment keeps 'Layer 2 mock'. Outcome: all 11 AWS scenarios name it with us-east-1. Placements in eu-west-1, which the old fallback allowed, now deny. The injected block is literal us-east-1, so current runs pass. The aws_full_stack e2e stub (us-east-1, us-east-1a/b) does not change, and no Go test pins the allowlist.

**Done when:**
- Rego test: provider region "eu-west-1" with params.region us-east-1 denies (fails today)
- Rego test: provider region = var.region denies (fails today)
- Rego test: aws_ resources with an aws provider entry lacking region deny, and so do aws_ resources with no aws entry (both fail today)
- Rego test: literal us-east-1 with params us-east-1 and availability_zone us-east-1a does not deny
- Rego test: availability_zone eu-west-1a with params.region us-east-1 denies (fails today)
- Rego test: fakeaws-shaped state (subnet with availability_zone eu-west-1a; sqs queue with region eu-west-1) denies via deny_state (fails today)
- Rego test: with no params.region, deny and deny_state return nothing, matching the Scaleway shape (state_policy_params_test.go:83-93)
- The harness mutation check kills every deny/deny_state body
