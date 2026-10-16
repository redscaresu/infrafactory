---
kind: code
status: ready
epic: rehearsal-passthrough
depends_on: []
touches: [internal/cli/exec_paths_test.go]
risk: high
---

# A test holds every exec path in the module to today's set

New internal/cli/exec_paths_test.go. It parses every non-test .go file in the module (go/parser) and pins a map of file to the count of exec.Command/exec.CommandContext call sites, matching today's tree: internal/cli/exec_runner.go:229 is 1, plus rehearse_fakeaws.go, mock_start_starter.go, ui_command.go, internal/api/formatter.go, internal/api/handlers_diagnostics.go, internal/e2e/helpers.go, internal/generator/claude_adapter.go and internal/runstore/commit.go. A file that imports os/exec and is not in the map fails, and the failure names the file. Any call to os.StartProcess, syscall.ForkExec, syscall.Exec or syscall.StartProcess also fails. Tip commit: `ADR: none — test only`.

**Done when:**
- `go test ./internal/cli -run TestNoNewExecPath -v` passes. The PR shows it failing three ways, each break made with a cp backup: a scratch internal/cli/zz_exec.go that imports os/exec (the failure names the file); a second exec.CommandContext in exec_runner.go (count 2, want 1); an os.StartProcess call.

The checks in the epic's **Checks before every stage** apply.
