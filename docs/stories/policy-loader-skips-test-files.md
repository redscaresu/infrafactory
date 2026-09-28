---
kind: code
status: ready
epic: policy-correctness
depends_on: []
touches: ["internal/harness/policy_packages.go", "internal/harness/opa.go", "internal/harness/state_policy.go", "internal/harness/policy_loader_test.go", "internal/harness/testdata/policy-loader/"]
risk: high
---

# Production policy evaluation never loads a *_test.rego

Layer 1 loads whole cloud dirs (infrafactory.yaml:73) through the v0-compat opa/rego (opa.go:9). discoverPolicyPackages walks every .rego file (policy_packages.go:26-34), and rego.Load has a nil filter (opa.go:58, state_policy.go:59). So a colocated v1-only test file would break every plan for its cloud. Skip *_test.rego in discovery, and pass rego.Load a filter that drops them. PolicyFileDefinesRule is unchanged. No run outcome or existing Go test changes: no *_test.rego exists today. aws-ingress-policy-and-holdout needs this story and rego-test-harness.

**Done when:**
- Point EvaluatePlanPoliciesWithParams and EvaluateStatePoliciesWithInput at a testdata dir holding a policy plus a _test.rego that does not parse under v0. Both return no error, and the test package is never queried (fails today)
- discoverPolicyPackages over that dir omits the test file's package (fails today)
