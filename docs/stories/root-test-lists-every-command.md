---
kind: code
status: ready
depends_on: []
touches: [internal/cli/root_test.go]
---

# root_test's table lists every command with subcommands

Found in review of #621. root_test.go's table of commands with subcommands is written by hand, so
a new command is silently untested. Add a test that walks the root command's tree and fails
naming any command with subcommands that the table does not list.

**Done when:**
- Removing one entry from the table makes the new test fail, naming that command.
