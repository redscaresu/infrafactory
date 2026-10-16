---
kind: code
status: ready
epic: rehearsal-passthrough
depends_on: []
touches: [cmd/infrafactory/main.go, internal/cli/contract.go, internal/cli/contract_test.go]
risk: high
---

# An error type carries tofu's exit code to os.Exit and prints nothing

contract.go gains an error that carries the exit code of a tofu run that finished. ExitCodeForError finds it through wrapping with errors.As and returns the code when it is 1-255, and ExitCodeRuntime otherwise, never 0 (contract.go:72-83). The error is quiet: tofu has already written its own stderr. main.go calls one cli function that prints only non-quiet errors and returns the code, so the quiet rule is unit-tested (main.go:11-14). Usage stays 2. Nothing produces the error yet. Tip commit: `ADR: none — no command produces it until rehearsal-passthrough-commands amends ADR-0042 and ADR-0002`.

**Done when:**
- `go test ./internal/cli -run 'TestExitCodeForError|TestReportError' -v` passes with these cases: a carried 2 gives 2, also when wrapped with %w; a carried 0 or -1 gives 1; a usage CLIError gives 2; any other error gives 1; a quiet error writes nothing; a non-quiet error writes its message. contract_test.go:41 and :79 are unchanged.

**From review (binding; overrides the scope above where they differ):**
- cmd/infrafactory/main.go no longer prints by itself. Its body is `os.Exit(cli.<ReportFn>(root.Execute(), os.Stderr))` or the same without any Fprintln, and `git grep -n Fprintln cmd/infrafactory/main.go` prints nothing.
- `go test ./internal/cli -run TestReportError -v` lists at least one PASS line on its own.
- A carried 256 (and 1000) gives 1, never a value os.Exit would truncate to 0.

The checks in the epic's **Checks before every stage** apply.
