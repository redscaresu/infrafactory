---
kind: code
status: ready
epic: policy-correctness
depends_on: []
touches: ["policies/scaleway/default_deny_ingress.rego", "policies/scaleway/region_restriction.rego", "policies/scaleway/no_public_database.rego", "policies/aws/region_restriction.rego", "policies/aws/no_public_db.rego", "docs/decisions/0034-a-prohibition-is-a-specification.md", "internal/harness/policy_claims_test.go", "internal/cli/state_policy_mock_only_test.go"]
risk: high
---

# Remove the claim that deny_state checks what Layer 3 created, at all three sites, with a ratchet that rewording cannot defeat

Remove the claim; do not make it true (see declined). Criteria evaluation receives the mock deployResult even after a sandbox apply (test_command.go:846-848, :867, :951, :1382). Sites: default_deny_ingress.rego:36-38 ('Layer 2 and Layer 3'); scaleway/region_restriction.rego:30-42 ('what the provider actually created', above state_resource :53); ADR-0034:76-79 ('deny_state against deployed state'). Every aws/scaleway file that defines deny_state gets a comment saying it reads the Layer 2 mock's state: aws/no_public_db, aws/region_restriction, scaleway/default_deny_ingress, scaleway/no_public_database, scaleway/region_restriction. ADR-0034 §4 says the same, and that the holdout is the only real-state check (docs/hld/2026-09-27-aws-web-stack.md:385). The ratchet has two parts. Positive: the required phrase 'Layer 2 mock', which a rewording drops. Negative: 'Layer 3', 'real state', 'actually created', 'provider created' or 'really created' in any comment of a file that defines deny_state. vpc_required.rego legitimately cites the Layer 3 gate and defines no deny_state. Comment and ADR edits only: no run outcome or existing Go test changes.

**Done when:**
- internal/harness/policy_claims_test.go fails if a file under policies/aws or policies/scaleway defines deny_state and lacks 'Layer 2 mock' in a comment (fails today on all five files)
- The same test fails on any listed phrase in any comment of a file that defines deny_state. It fails today on default_deny_ingress.rego:37 and region_restriction.rego:32
- The same test fails unless ADR-0034 §4 contains 'Layer 2 mock' and 'holdout' (fails today)
- Claim pin (ADR-0028), not proof of removal: evaluateSupportedCriteria with sandboxApplied=true reports an accept-inbound group from the mock StateSnapshot, with Layer mock_deploy
