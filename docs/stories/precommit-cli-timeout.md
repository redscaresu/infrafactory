---
kind: chore
status: ready
depends_on: []
touches: [.githooks/pre-commit]
---

# The pre-commit go test does not time out on internal/cli

Found by the conductor's builder on #603. `.githooks/pre-commit:147` runs `go test -tags noui
$PKGS` with go's 10-minute default, and `internal/cli` takes about 21 minutes on the lead's
machine, so any commit touching it fails the hook unless the builder sets
`GOFLAGS=-timeout=40m`. Find what makes the package slow (`go test -json` per-test elapsed, the
top ten in the PR body). If one or two tests account for most of it and can be moved behind
`testing.Short()` or a build tag that CI still runs, do that and run the hook with `-short`.
Otherwise set `-timeout 40m` in the hook with a comment saying why.

**Done when:**
- A commit touching internal/cli passes the hook with no GOFLAGS set (the PR body gives the hook's run time).
- CI still runs every test it ran before (the PR body compares the test counts).
