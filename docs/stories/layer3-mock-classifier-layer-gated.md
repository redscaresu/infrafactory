---
kind: code
status: ready
depends_on: []
touches: [internal/generator/pitfalls_learn.go, internal/generator/pitfalls_learn_test.go, internal/cli/run_command.go, internal/cli/run_command_test.go]
risk: high
---

# The mock-bug classifier applies only to failures from a mock layer

Found in #506's review. `IsMockServerBug` matches broad signals (for example
`resourcenotfoundexception`) that real AWS also returns, and all three learner paths apply it with no
layer gate. On a Layer 3 run (sandbox_deploy), a real-cloud failure that matches one of those signals
is written to docs/mock-gaps.md instead of teaching a pitfall, so the auto-learning pipeline silently
skips a real lesson.

Gate the classifier on the failure's layer: a failure recorded at layer sandbox_deploy (or Origin
saying real cloud, per #459) is never classified as a mock bug, whatever its text. Mock-layer
failures keep today's behaviour.

**Done when:**
- A Layer 3 failure whose detail contains a mock signal (`ResourceNotFoundException`) is learned as a
  pitfall on each of the three learner paths, and not written to mock-gaps.md.
- The same detail from a mock-layer failure still goes to mock-gaps.md.
- Each new test is shown failing by a cp-backup break recorded in the PR.
