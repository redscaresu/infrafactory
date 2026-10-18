---
kind: code
status: ready
depends_on: []
touches: [internal/cli/exec_runner.go, internal/cli/exec_runner_test.go]
---

# A tofu child does not outlive infrafactory

Found in review of #617. internal/cli/exec_runner.go:268 starts every command in its own process
group (`Setpgid: true`) so cancel can signal the group. When infrafactory itself is killed
(SIGKILL, a CI job cancelled), nothing signals that group, and tofu keeps running. #617's CI sweep
is a backstop for one workflow only. Fix it in the runner for every caller. On Linux, set
`Pdeathsig: SIGKILL` (build-tagged). On macOS, which has no Pdeathsig, have the child group
watched in a way that works without the parent, or document the gap in a comment with a
`ponytail:` note if no small fix exists.

**Done when:**
- A Linux test (run in CI) starts a long `sleep` through the runner from a helper process, SIGKILLs the helper, and finds the sleep gone within 2 seconds.
- macOS builds still pass, and the PR body says what macOS does.
