---
status: active
---

# Policies are unit-tested, and say only what they actually check

**Done when:**
- every policy under `policies/aws/` and `policies/scaleway/` has Rego unit tests (`*_test.rego`,
  `test_*` rules) run by `go test` through the OPA library infrafactory already uses
  (`opa/v1/tester`), and each `deny`/`deny_state` rule has a test that fails if the rule is removed;
- AWS `region_restriction` honours the criterion's `params.region`, proven by a Rego test that
  fails on today's policy;
- the claim that `deny_state` checks what Layer 3 really created is either made true or removed
  everywhere it is made (ADR-0034, `policies/scaleway/default_deny_ingress.rego`);
- ADR-0017's policy-conflict detection receives the denying policy's name, proven by a test that
  fails today.

**Out of scope:** adopting conftest; GCP and Genesys policies (GCP is paused).

**Constraints:** ADR-0028 (a factual claim is a test), ADR-0017, ADR-0034, and the
undefined-is-not-false lesson in `internal/harness/policy_packages.go`. No new binary or
dependency: the OPA Go library is already in `go.mod`.

**Evidence, from the 2026-09-27 conftest assessment** (`.swarm/research/conftest-suitability.md`,
local): conftest would replace a ~315-line evaluator that is not the problem, and cost `deny_state`,
params, per-cloud routing and repair-loop feedback to rebuild — so it was not adopted. What is
missing is policy tests: two five-line Rego tests found two false-coverage defects —
- GCP `region_restriction` ignores `params.region` (it reads a `data.region_allowlist` that is never
  supplied, then falls back to three hard-coded regions); **AWS uses the same pattern, untested**.
- Scaleway's plan-time `region_restriction` does not compare a zonal resource's `zone` with
  `params.region`; whether it bites depends on whether real planned values carry `region`, which
  has not been checked against a real plan.

It also found that Layer 3 never evaluates policies against real state — `deny_state` there sees
the mock's snapshot — and that ADR-0017's detection is dead in production: `toFeedbackFailures`
drops `Policy` (`internal/cli/run_command.go`), so the detector always receives an empty name.
