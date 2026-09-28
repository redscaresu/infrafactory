---
kind: code
status: ready
epic: policy-correctness
depends_on: []
touches: ["internal/cli/run_command.go", "internal/cli/run_command_policy_gap_test.go", "docs/decisions/0017-policy-pitfall-conflict.md"]
---

# toFeedbackFailures carries Policy, so ADR-0017's DetectPolicyConflict receives the denying policy's name

toFeedbackFailures (run_command.go:1414-1424) drops Policy, so f.Policy is always empty at :642. Copy every field FailureSummary has: Layer, Stage, Check, Policy, Command, Resource, Detail. There is no Status, because FailureSummary has none (output_contract.go:32-39). Oscillation signatures key on Check/Resource/Detail (feedback/oscillation.go:19) and do not change. Add a dated line to ADR-0017's amendment (:69-79). Public output: FailureSummary and run JSON do not change. On stuck or repair_budget_exhausted runs (run_command.go:441), `run` now appends docs/policy-gaps.md and logs policy_gap_recorded (:641-665); no pitfall learning is suppressed. No scenario's pass/fail changes, and no existing Go test changes.

**Done when:**
- A table test fails unless toFeedbackFailures preserves Policy, Layer, Stage and Command (fails today)
- A stuck run-loop test with fake deps drives a policy failure whose HCL matches a same-resource prescriptive pitfall. It fails unless a temp docs/policy-gaps.md entry names that policy (fails today)
