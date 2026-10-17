---
kind: chore
status: ready
depends_on: []
touches: [.github/workflows/ci.yml, .github/workflows/doc-hygiene.yml, .github/workflows/release.yml, .github/workflows/scenario-gate.yml]
---

# Every CI job has a timeout, and a timed-out UI run still uploads its report

Found by the conductor on #602, which set timeouts on ci.yml's jobs. Three jobs still run on
GitHub's 6-hour default: doc-hygiene.yml `check-doc-hygiene` (a required check), release.yml
`build-and-release`, and scenario-gate.yml `llm-credentials`. Set each from its recent run times
(`gh run list --workflow <file> --json`), with headroom of about 2x the slowest.

ci.yml's "Upload Playwright report on failure" step runs on `if: failure()`. When the new
timeout cancels the job the step is skipped, so a hung UI run leaves no report. Change it to
`if: failure() || cancelled()`. scenario-gate.yml's upload step has the same condition: change
it the same way.

**Done when:**
- `grep -L timeout-minutes` finds no job without one across `.github/workflows/*.yml` (the PR body lists each job and the value set, with the run times it came from).
- Both upload steps read `if: failure() || cancelled()`.
- CI on the PR is green.
