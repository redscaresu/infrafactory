---
kind: code
status: ready
depends_on: []
touches: [internal/cli/rehearse_prepare_test.go, internal/cli/rehearse_read_test.go]
---

# The rehearse tests fail clearly when a subcommand is missing

internal/cli/rehearse_prepare_test.go:41-42 and rehearse_read_test.go:127-128 discard cobra `Find`'s error (`group, _, _ := root.Find(...)`). If `rehearse`, `prepare` or `read` is renamed or unregistered, the test panics on a nil command, or tests the wrong one, instead of failing with a message. root_test.go:23 already checks the error. Use `require.NoError` on each Find and `require.Equal` on the found command's Name, as the testing rules in AGENTS.md prefer.

**Done when:**
- No `Find(` call in internal/cli/*_test.go discards its error (a grep in the PR body shows none).
- Renaming the `prepare` subcommand locally makes rehearse_prepare_test fail with a require message, not a panic. Say so in the PR body, and revert the rename.
- `go test ./internal/cli/` passes.
