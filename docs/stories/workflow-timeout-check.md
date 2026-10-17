---
kind: code
status: ready
depends_on: []
touches: [scripts/check_workflow_timeouts.sh (new), .github/workflows/doc-hygiene.yml, Makefile:doc-hygiene]
---

# A check fails when a workflow job has no timeout-minutes

#602 and #605 set a timeout on every job, but nothing stops a new job from landing on GitHub's
6-hour default. Add a script that lists every job in `.github/workflows/*.yml` without
`timeout-minutes` (python3 with a YAML parser if one is available on the runner, else a small
indentation-aware parser), and run it from `make doc-hygiene` and the doc-hygiene workflow.

**Done when:**
- Removing one job's timeout-minutes locally makes `make doc-hygiene` fail and name that job (shown in the PR body, then reverted).
- `make doc-hygiene` passes on the PR.
