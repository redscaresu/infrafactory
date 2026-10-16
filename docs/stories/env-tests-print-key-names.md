---
kind: code
status: ready
touches: [internal/cli/exec_runner_test.go]
risk: high
---

# The exec-runner env tests print key names, never values, when they fail

`TestExecRunnerReplaceEnvPassesOnlyPathAndEnv` and `TestExecRunnerInheritedEnvIsUnchanged` in
internal/cli/exec_runner_test.go compare the child's whole environment with `assert.Equal`. When
one fails, testify prints both sides in full: every inherited variable and its value. A failure in
CI would put the runner's secrets in the CI log, and a local failure puts the developer's tokens in
a session transcript. On 2026-10-16 a mutation run in the runner-stream build did exactly that
locally. #582's new env test already compares key names first; these two do the same.

Each test first compares the sorted key names (`require.Equal` on keys), then compares values
without printing them: on a mismatch the message names the key and says the value differs, and
never prints a value. Values that the test itself set (`PATH=/from-env`, `AWS_REGION=us-east-1`,
`TF_IN_AUTOMATION=1`, `INFRAFACTORY_KEPT=kept`) may still be shown.

**Done when:**
- With a planted variable `INFRAFACTORY_SECRET_PROBE=probe-value-123` in the test process's
  environment and each test's expectation mutated to fail, the `go test` output names the failing
  key and does not contain `probe-value-123`. Both runs and their output are in the PR body.
- Both tests still pass unmutated, and still fail when the runner passes an extra variable or
  drops one (shown with a FAIL line each).
