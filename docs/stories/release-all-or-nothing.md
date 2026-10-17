---
kind: chore
status: ready
depends_on: []
touches: [.github/workflows/release.yml]
---

# A release publishes only when every binary built

Found in review of #605. release.yml's matrix legs each upload to the release
(action-gh-release, line 67), so a leg that times out or fails leaves a published release with
some binaries missing. Build in the matrix and upload artifacts. Then publish in one job that
`needs:` every leg and attaches all binaries and checksums at once.

**Done when:**
- The publish job `needs:` the matrix job, and no matrix leg calls action-gh-release.
- A dry run on a fork or with `workflow_dispatch` against a draft release attaches every binary and .sha256 (the run is linked in the PR body). No real release is published.
