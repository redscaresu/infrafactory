---
kind: chore
status: ready
depends_on: []
touches: [.github/workflows/ci.yml]
---

# Every CI job has a timeout, and the fakeaws e2e step has an explicit go test -timeout

Only the `board` job sets `timeout-minutes` (.github/workflows/ci.yml:31). Every other job (doc-links, test, test-full, the aws e2e jobs, govulncheck, gitleaks, build-binary) falls back to GitHub's 6-hour default, so a hung step holds a runner and blocks merges for hours. The fakeaws e2e step (ci.yml:351) relies on go test's 10-minute default.

Set `timeout-minutes` on every job: about 2× its slowest run in the last 20 runs on main (`gh run list` / `gh run view --json jobs`), rounded up to 5 minutes. Put the numbers you used in the PR body. Give the e2e `go test` an explicit `-timeout` a little under its job's timeout, so a hang fails with go's goroutine dump rather than a job cancel.

**Done when:**
- Every job in ci.yml has `timeout-minutes`, and the PR body lists each job's slowest recent run beside its timeout.
- The fakeaws e2e `go test` line has `-timeout`, shorter than its job's `timeout-minutes`.
- CI on the PR is green.
