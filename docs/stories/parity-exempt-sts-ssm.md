---
kind: code
status: ready
epic: fakeaws-step-one-surfaces
depends_on: []
touches: ["internal/e2e/cross_repo_parity_test.go"]
---

# Item 12 (first half): cross-repo parity exempts sts and ssm, proven by a test CI runs

Add sts and ssm, each with a non-empty reason, to the fakeaws exempt map (internal/e2e/cross_repo_parity_test.go:117-123). Move the per-fake loop body (:67-88) into a function taking (spec, landed, scenariosDir) so a table test can feed it landed lists without a sibling checkout. Today the test skips in CI because no sibling is checked out (survey-constraints facts 12-13).

**Done when:**
- A new table test in the required test job, with no sibling needed, feeds today's fakeaws LandedServices plus sts and ssm and gets nothing missing; with either exemption removed, or given an empty reason, it fails naming that service
- The existing TestCrossRepoParity_EveryLandedServiceHasScenario behaves the same (it calls the extracted function)
- The diff touches only internal/e2e/cross_repo_parity_test.go; pitfalls/aws.yaml is unchanged
