---
kind: code
status: ready
epic: rehearsal-passthrough
depends_on: []
touches: [.github/fakeaws-e2e-tests, docs/decisions/0042-rehearsal-command-contract-and-walls.md, docs/hld/2026-09-30-import-production-state-into-the-mocks.md, internal/cli/exec_runner.go, internal/cli/exec_runner_test.go, internal/cli/rehearsal_assumptions_test.go, internal/harness/static.go]
risk: high
---

# The runner streams to caller writers on its one exec path, and a spike decides console

harness.Command gains Stdout and Stderr io.Writer fields (nil keeps the buffered CommandResult). Run uses them on the same exec.CommandContext, Cancel, Setpgid, catchSignalsDuringRun and commandEnv (exec_runner.go:222-294), with no second call site and no change to CommandRunner. Console spike, over a pipe only (see decisions): if console is kept, add Stdin io.Reader. The runner feeds it through a pipe it owns and never hands the caller's fd to the Setpgid child (that would be SIGTTIN). It closes the pipe when the child exits and never waits on the caller's reader. If console is refused: no Stdin, and console is struck from HLD L317 and ADR-0042 L39. Either way, one line in ADR-0042 § The family records the outcome.

**Done when:**
- `go test ./internal/cli -run TestExecRunnerStream -v` passes with TestExecRunnerStreamsBeforeExit (the caller's writer gets a line while the child still runs), TestExecRunnerStreamingOneSIGINT (cancel sends the group one SIGINT, as at exec_runner_test.go:262), TestExecRunnerStreamingCancelNeverSucceeds (the child traps SIGINT and exits 0, and Run still returns an error) and TestExecRunnerStreamingReplaceEnv. `go test ./internal/cli -run TestAssumption_Console -v` with local tofu shows the outcome. If kept: TestAssumption_ConsoleAnswersEachPipedLine, TestAssumption_ConsoleInputFalseNeverPrompts, TestExecRunnerStdinReachesChild and TestExecRunnerReturnsWithStdinOpen all pass, the last showing Run returns within 2s of the child's exit while an io.Pipe stdin stays open. If refused: the PR quotes the failing assumption's output, and `git grep -n console docs/decisions/0042-rehearsal-command-contract-and-walls.md docs/hld/2026-09-30-import-production-state-into-the-mocks.md` shows the list amended.

**From review (binding; overrides the scope above where they differ):**
- Every spike command runs with INFRAFACTORY_ENABLE_E2E=1, and the PR shows `--- PASS` lines (not SKIP) for each TestAssumption_Console* case.
- If console is refused, keep the assumption test, rewritten to assert the failure observed (for example `console reads piped stdin only at EOF`) so that it passes and records why console was struck. Quote its output in the PR.
- Add a case to TestExecRunnerStreamsBeforeExit with Stdout set to the write end of an os.Pipe (an *os.File, the path production takes). The test reads a line from the read end before the child exits, and Run returns once the child exits.
- Each kept TestAssumption_Console* case is listed in .github/fakeaws-e2e-tests, and the PR shows its PASS line from the CI log.

The checks in the epic's **Checks before every stage** apply.
