---
kind: code
repo: fakeaws
status: ready
epic: fakeaws-rehearsal-instance
depends_on: [fakeaws-admin-token-and-bind]
touches: [repository/repository.go, repository/repository_test.go, handlers/admin.go, handlers/admin_test.go, AGENTS.md, SECURITY.md, CHANGELOG.md]
risk: high
---

# A file-backed db, its -wal/-shm, the snapshot and a restored copy are created 0600

- **The db.** In openDB/New (repository.go:75-113), when the db file does not exist, create it 0600 before sql.Open. SQLite then gives `-wal`/`-shm` the same mode (checked on modernc v1.52.0).
- **The snapshot.** In Snapshot, after the existing Remove (repository.go:326-328), create `.snapshot` with `os.OpenFile(snap, O_CREATE|O_EXCL|O_WRONLY, 0o600)`, close it, then `VACUUM INTO` it (repository.go:332). modernc accepts an empty existing target; only a non-empty one is refused. On any VACUUM error, `os.Remove(snap)` before returning: a left-over empty `.snapshot` would pass Restore's os.Stat (repository.go:356) and copy 0 bytes over the live db.
- **The vacuum seam.** Run VACUUM through a package-level func var in repository/ so a test can force it to fail.
- **The restore.** copyFile (repository.go:398-404) writes 0600, so the restored db is 0600 even when the live file was deleted.
- **The docstring.** The CRITICAL docstring goes on handleMockSnapshot/handleMockRestore (admin.go:200-234), because the contract audit scans handlers/*.go only.

Not changed: an existing db's mode, the parent MkdirAll(0755) at repository.go:323, and :memory:.

**Done when:**
- `TestContract_file_backed_db_owner_only` (handlers; the test sets umask 022): NewApplication on a fresh t.TempDir() file, one provider write, POST /mock/snapshot, POST /mock/restore. The db, `-wal`, `-shm`, `.snapshot` and the restored db are each exactly 0600, also when the live db file is removed before the restore.
- `.snapshot` is created 0600 before VACUUM INTO writes it (the PR cites the modernc check). A failed VACUUM leaves no `.snapshot`, so a following POST /mock/restore is 404 and the live db is intact. This is tested through the vacuum seam.
- :memory: snapshot/restore still answer 409, and the existing snapshot/restore tests pass unchanged.
- Each new test is shown failing by a cp-backup break, recorded in the PR: pre-create db 0600, pre-create snapshot 0600, remove-on-error, copyFile mode.
- AGENTS.md line 151 lists the id and TestAgentsMDListsEveryContract passes; SECURITY.md and CHANGELOG [Unreleased] (Changed: files fakeaws creates are 0600) are updated.
