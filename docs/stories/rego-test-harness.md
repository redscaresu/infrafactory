---
kind: code
status: ready
epic: policy-correctness
depends_on: []
touches: ["internal/harness/rego_policy_test.go", "internal/harness/testdata/regotest/", "docs/decisions/0028-factual-claims-are-tests-not-comments.md", "go.sum"]
risk: high
---

# go test runs every *_test.rego under the real policies/ tree through opa/v1/tester, and an unkilled deny/deny_state mutant fails the test

Split from the loader for size. The runner and the mutation check share module loading, so they stay together, and both halves still land in wave 1. The new internal/harness/rego_policy_test.go lives under ./internal/..., which `make test` runs (Makefile:418-419). It resolves policies/ from the repo root through the existing opaTestRepoRoot (opa_m98_test.go), not the package cwd. It loads policies plus tests through github.com/open-policy-agent/opa/v1/tester, from the same module as go.mod:12. A helper returns problems and the real test turns each into t.Errorf, so a problem fails the test. Problems: a failing or erroring test, and a _test.rego with no test_ rule. Mutation: for every policy file with a sibling <name>_test.rego, remove each deny/deny_state body in turn (v1/ast), re-run the tests, and report any body no test kills, with file:line. Opt-in is per file until policy-tests-required. Fixtures are inline Rego trimmed from a real capture: a plan from the pinned provider, or state from the mock's /mock/state. None is hand-shaped. Amend ADR-0028: a policy claim is a mutation-checked Rego test, and conftest was not adopted. No existing test changes.

**Done when:**
- The walked set is resolved from the repo root and contains policies/aws/region_restriction.rego and policies/scaleway/region_restriction.rego. The test fails if policies/aws or policies/scaleway has no non-test .rego
- Self-test: a copy of the real policies/aws in t.TempDir plus a _test.rego with one failing test_ rule makes the helper return a problem
- Self-test: a fixture policy with one deny body no test kills yields a problem naming its file:line, which the real test reports with t.Errorf. A fixture whose every deny/deny_state body is killed yields none
- Self-test: a fixture _test.rego with zero test_ rules yields a problem
- go.mod gains no require line
