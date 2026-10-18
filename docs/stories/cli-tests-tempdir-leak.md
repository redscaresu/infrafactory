---
kind: code
status: ready
depends_on: []
touches: [internal/cli/*_test.go (the tests #614's review named)]
---

# internal/cli tests leave no temp dirs behind

Found in review of #614 (see its review comments for the tests named). Some internal/cli tests
create directories with os.MkdirTemp and never remove them, so each run leaves files in $TMPDIR.
Use `t.TempDir()`, or `t.Cleanup` where the path must be made elsewhere.

**Done when:**
- `go test ./internal/cli/` with TMPDIR set to an empty directory leaves it empty (the PR body shows `ls -A` before and after).
- No `os.MkdirTemp` in internal/cli/*_test.go is left without a matching cleanup (a grep in the PR body).
