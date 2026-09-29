---
kind: code
status: ready
touches: [".github/workflows/scenario-gate.yml"]
---

# `scenario-gate` shows a skip as a skip, not as a pass

The user decided (2026-09-29) not to add an `OPENROUTER_API_KEY` secret: the harness keeps
calling the `claude` command for now. So on every scenario PR, `scenario-gate` skips at
`scenario-gate.yml:65-68` and reports success having run nothing (as on #355). A reader of the PR
cannot tell that from a real pass.

Make the skip visible: the job's conclusion is `skipped` or `neutral`, or the check name says
it was skipped. Keep fork PRs working.

**Done when:** a scenario PR on a repo with no `OPENROUTER_API_KEY` shows `scenario-gate` as
skipped (not success) in `gh pr checks`, and a PR that touches no scenario is unaffected.
