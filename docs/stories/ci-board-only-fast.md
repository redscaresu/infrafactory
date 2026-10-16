---
kind: code
status: ready
touches: [.github/workflows/ci.yml, scripts/ci_board_only.sh, scripts/ci_board_only_test.sh]
risk: high
---

# A PR that changes only board files passes CI in about a minute

A board PR (a status flip, a new story file) runs the full Go suite and the AWS e2e tests: about
10 minutes each, roughly an hour of waiting across three board PRs on 2026-10-16. `test`,
`gitleaks`, `check-doc-hygiene` and `aws-e2e` are required checks on main, so the jobs cannot be
skipped by a `paths` filter: a job that never runs never reports, and the PR stays blocked.

Instead, in .github/workflows/ci.yml, the `test` and `aws-e2e` jobs (and `govulncheck` and
`build-binary` if they run on pull requests) first compute the PR's changed files against its
base (`git diff --name-only <base sha>...<head sha>` after a fetch deep enough to hold the merge
base). When every changed file is under `docs/stories/`, `docs/epics/` or `docs/hld/`, or is
`docs/Board.base` or `STATUS.md`, the job prints the list and the reason and ends green before
any setup. In every other case, the full job runs, including when the diff cannot be computed
(fetch fails, base missing, empty list). That case fails closed, never skips green.
`gitleaks` and `check-doc-hygiene` always run in full. Pushes to main always run in full.

**Done when:**
- A board-only PR (this repo, a scratch branch touching only docs/stories/) shows `test` and `aws-e2e` green in under 2 minutes, each printing the file list and "board only: skipped".
- A PR touching one Go file and one story file runs the full `test` and `aws-e2e`, as their logs show.
- The detection step is a script (for example scripts/ci_board_only.sh) with a test (bash or Go) covering: all board files gives skip; one non-board file gives run; an empty list gives run; a failed `git diff` gives run. Each case is shown failing with the script's check broken (cp backup).
- `gitleaks` and `check-doc-hygiene` are unchanged, and a push to main runs every job in full.
