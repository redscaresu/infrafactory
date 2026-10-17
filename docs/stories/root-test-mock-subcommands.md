---
kind: code
status: ready
depends_on: []
touches: [internal/cli/root_test.go]
---

# TestNewRootCmdHasExpectedCommands fails when a mock subcommand is missing

Found by the conductor on #603. internal/cli/root_test.go:116 checks only the error from
`root.Find([]string{"mock", sub})`. cobra's Find returns the deepest command it matched, so a
missing `mock <sub>` comes back as `mock` with no error, and the test still passes. Check the
found command's `Name()` equals `sub` (and `name` in the first loop), with `require`, as the
testing rules in AGENTS.md prefer.

**Done when:**
- Unregistering `mock logs` locally makes the test fail with a message naming `logs`. Say so in the PR body, and revert it.
- `go test ./internal/cli/ -run TestNewRootCmdHasExpectedCommands` passes.
