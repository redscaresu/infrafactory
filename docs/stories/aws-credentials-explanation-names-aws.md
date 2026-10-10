---
kind: code
status: ready
depends_on: []
touches: ["internal/cli/output_contract.go", "internal/cli/output_contract_test.go"]
---

# A failed aws preflight is explained in aws terms, not Scaleway's

Seen on real AWS in aws-layer3-claim-legs (2026-10-10): the second `test`, refused because another
run held the claim, printed the explanation "Layer 3 real Scaleway deploy is enabled but
credentials are unavailable" and "set SCW_ACCESS_KEY and SCW_SECRET_KEY before enabling
sandbox_deploy" for `sandbox_deploy/preflight check=credentials`. `explainFailure`
(internal/cli/output_contract.go, case `failure.Check == "credentials"`) has one Scaleway-only
answer for every cloud. On aws that check also fails when the run cannot confirm it holds the claim
(docs/operations.md § Running aws-web-live), where the advice is wrong twice over.

Make the explanation follow the scenario's cloud. For aws, name the key file and the `aws` config
block, and say that a `preflight` failure next to an `aws_scope_claim` failure is that refusal
(read `aws_scope_claim` first), as operations.md does.

**Done when:**
- An aws `credentials` failure explains with no SCW_ variable; a Scaleway one is unchanged
  (golden files updated). The test fails on today's code, shown by mutation.
