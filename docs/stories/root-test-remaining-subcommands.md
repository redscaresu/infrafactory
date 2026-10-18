---
kind: code
status: ready
depends_on: []
touches: [internal/cli/root_test.go]
---

# The root test checks live's, pitfalls' and rehearse's subcommand sets too

Found in review of #616, which checks the root's and `mock`'s full subcommand sets with
ElementsMatch. `live`, `pitfalls` and `rehearse` also have subcommands, and nothing notices one
added or removed. Extend the same check to each, from one table so a new parent is one line.

**Done when:**
- Removing one `rehearse` subcommand locally fails the test naming it (shown in the PR body, then reverted).
- `go test ./internal/cli/ -run TestNewRootCmdHasExpectedCommands` passes.
