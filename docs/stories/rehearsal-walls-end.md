---
kind: code
status: blocked
blocked_by: [rehearsal-walls-regate]
epic: rehearsal-walls
depends_on: [rehearsal-walls-regate]
touches: [internal/cli/rehearse_end.go, internal/cli/rehearse_end_test.go, internal/cli/rehearse.go, internal/cli/command_golden_test.go, internal/cli/testdata/golden/commands/rehearse_end*]
risk: high
---

# `rehearse end <id>`: stop this rehearsal's fakeaws only when it proves itself, then delete everything, idempotently

New file `internal/cli/rehearse_end.go`, registered at rehearse.go:47. `rehearse end <id>` finds the rehearsal with rehearsalOpen (rehearsal-walls-regate), never rehearsalsDir, which mkdirs and writes a .gitignore (rehearse_prepare_copy.go:151-190). A bad id (not a single path element, `.`, `..`, or a symlinked `<id>`) is a usage error, exit 2, and deletes nothing. A missing rehearsal, or a repo with no `.infrafactory/rehearsals`, exits 0 reporting nothing deleted and creates nothing. A dead proxy that accepts does not stop `end`, which runs no tofu; the report names it.

With a record, `end` signals only when /healthz on 127.0.0.1:<port> is 200 and GET /mock/requests is 200 with the record's token. It then sends SIGTERM to -pid, waits until the port refuses a dial, and sends SIGKILL to the group after a fallback (a package var tests shorten). If the port still answers after SIGKILL, it reports `stop failed: pid P port N`, exits 1, and deletes nothing, so rehearsal.json stays for a later `end` to retry. Otherwise (no record, an unparseable one, a dead port, /healthz not 200, /mock/requests not 200 with the token) it signals nothing and reports why, naming the pid and port when the record has them. A record holding only pid, port and token (a prepare killed right after the start) is enough to signal.

It then lists every file under <id> (relative) and removes <id> whole: work, data, home, state, state-config, vars, record, fakeaws/ db with -wal/-shm, snapshot, log and reads/. It reports {id, fakeaws: stopped | not signalled: <why> | stop failed: <why>, deleted: [...]} in human and json. A cancel before the stop refuses with nothing signalled or deleted, exit 1; once the stop has begun, `end` finishes. Goldens for success, a missing id and a bad id.

Tests use a stub fakeaws that is a helper-process re-exec of the test binary (the rehearsal-walls-fakeaws-start shell-script pattern, no TestMain edit), in its own process group, serving /healthz and a token-guarded GET /mock/requests on the recorded port, so stopping the group closes the port.

**Done when:**
- With the stub recorded: `end` stops the group (kill(-pgid,0) fails), the recorded port refuses a dial afterwards, the report says `stopped`, and every planted file is gone and listed. The planted files are work/, data/, state/, fakeaws/fakeaws.db, -wal, -shm, a snapshot, reads/recorded-read.json and state-config/.
- A stub that ignores SIGTERM is stopped by SIGKILL within the shortened fallback; removing the SIGKILL fallback turns it red.
- A token-guarded listener held by the test process itself, outside the stub's group, keeps answering after SIGKILL: `end` reports `stop failed: pid P port N`, exits 1, and rehearsal.json and every other file still exist.
- A table of cases, each with a live sleeper stub in its own group: missing rehearsal.json, unparseable rehearsal.json, a dead (refusing) port, /healthz non-200, and /mock/requests 401 for the record's token. In each the sleeper is still alive, the rehearsal's files are deleted, and the report names the reason and the pid when known. Removing the token check turns the 401 case red (cp-backup).
- A record holding only pid, port and token, with no override files (what a prepare killed after the start leaves), is ended: the group is stopped and <id> is gone.
- A second `end` on the same id exits 0 with an empty deleted list. `end ../x`, `end a/b` and a symlinked <id> each exit 2, and a sentinel file outside rehearsals/ survives. `end x` on a repo with no `.infrafactory` exits 0, and `.infrafactory` still does not exist.
- A cancel injected before the stop leaves the sleeper alive and every file in place, with exit 1.
- TestCommandOutputGoldenSnapshots covers rehearse_end success, missing and bad-id cases.
- Checks before every stage in docs/epics/rehearsal-walls.md hold.

**From review (binding; overrides the scope above where they differ):**
- The httptest stand-in is replaced by a helper-process re-exec stub in its own group, so a broken stop cannot pass on the SIGKILL fallback; the port refusing a dial and 'stopped' are asserted (skeptic).
- The SIGTERM-ignoring case reaches SIGKILL within the fallback, red if the fallback is removed (skeptic).
- If the port still answers after SIGKILL: `stop failed: pid P port N`, exit 1, rehearsal.json kept (skeptic).
- A bad id, a symlink included, is exit 2, one value. `end` on a repo with no rehearsals creates nothing; the id is resolved by rehearsalOpen, never rehearsalsDir (skeptic, critic).
- Table cases for missing record, unparseable record, dead port, /healthz non-200 and /mock/requests non-200 each leave the sleeper alive, delete only the rehearsal's files when not cancelled, and name the reason (codex).
- A record written right after the start is enough for `end` to stop the group of a prepare killed there (critic).
- Depends on rehearsal-walls-regate, which holds rehearsalOpen (critic).
