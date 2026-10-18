---
kind: code
status: ready
depends_on: []
touches: [internal/cli/rehearse_fakeaws.go, internal/cli/rehearse_fakeaws_test.go, internal/cli/rehearse_walls.go, internal/cli/rehearse_walls_test.go]
---

# A SIGKILLed prepare leaves a record `rehearse end` can stop

Found in review of #626. `rehearse prepare` writes the rehearsal's pid, port and token to
rehearsal.json only after fakeaws's ready checks and retries (rehearse_walls.go). A prepare
SIGKILLed in between leaves fakeaws running on the rehearsal's db, and `rehearse end` on that
rehearsal finds no record, signals nothing and removes the dir. The token is known before Start,
so record pid, port and token right after `cmd.Start()`, before the ready checks, the retries and
the full record write.

**Done when:**
- A test kills prepare between Start and the ready check, and `rehearse end` stops that fakeaws.
