---
kind: code
status: ready
depends_on: []
touches: [internal/cli/root_test.go]
---

# TestNewRootCmdHasExpectedCommands checks the whole command set

Found in review of #608. root_test.go's hand-kept lists miss `mock reset`, `reap`, `deploy`, `live`,
`pitfalls` and `rehearse`. A command can be added or removed with no test noticing. Compare the
root's and `mock`'s full subcommand names with `assert.ElementsMatch` against the expected lists,
so a missing command and an unlisted new one both fail.

**Done when:**
- Adding a dummy subcommand locally fails the test naming it, and so does removing `mock reset` (both shown in the PR body, then reverted).
- `go test ./internal/cli/ -run TestNewRootCmdHasExpectedCommands` passes.
