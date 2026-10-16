---
kind: code
status: ready
touches: [internal/cli/exec_runner_test.go]
risk: high
---

# The exec-runner env tests never print an inherited value when they fail

Three tests in internal/cli/exec_runner_test.go compare the child's environment with
`assert.Equal` on the full `KEY=value` output: `TestExecRunnerReplaceEnvPassesOnlyPathAndEnv`,
`TestExecRunnerInheritedEnvIsUnchanged`, and `TestExecRunnerStreamingReplaceEnv` (#582; it checks
key names first, but then compares the whole output too, so a value mismatch still prints the
inherited `PATH`). When one fails, testify prints both sides in full. A failure in CI would put the
runner's secrets in the CI log, and a local failure puts the developer's tokens in a session
transcript; on 2026-10-16 a mutation run in the runner-stream build did exactly that locally.

All three compare the sorted key names first, then compare values without printing them: on a
mismatch the message names the key and says the value differs. Values the test itself set
(`PATH=/from-env`, `AWS_REGION=us-east-1`, `TF_IN_AUTOMATION=1`, `INFRAFACTORY_KEPT=kept`) may be
shown; inherited ones never are.

**Done when:**
- For each of the three tests, two mutations are run and their output is in the PR body:
  - a key-set mismatch (the expectation gains or loses a key): FAIL names the key;
  - a same-key value mismatch (for the inherited test, a planted
    `INFRAFACTORY_SECRET_PROBE=probe-value-123` whose expected value is changed; for the two
    replace-mode tests, `PATH` set to `/probe-path-123` in the test process and its expected value
    changed): FAIL names the key, and the output contains neither `probe-value-123` /
    `/probe-path-123` nor the changed expected value.
- All three pass unmutated.
