---
kind: docs
status: blocked
blocked_by: [rehearsal-passthrough-e2e]
epic: rehearsal-passthrough
depends_on: [rehearsal-passthrough-e2e]
touches: [STATUS.md, docs/epics/rehearsal-passthrough.md, docs/hld/2026-09-30-import-production-state-into-the-mocks.md, docs/operations.md]
---

# docs/operations.md § Passthrough; epic closed

In docs/operations.md, add ### Passthrough after ### Prepare and before ### End (L733). It covers: the `--` form; the actions; the refused actions and flags; -input=false, and that apply or destroy needs -auto-approve or a plan file; the var file being passed automatically; -out inside the work dir; live output; no --output json; the exit codes and the 2-vs-2 rule; one Ctrl-C; no deadline, set against Prepare's 10-minute one (L696); the rehearsal kept after a failure. Also edit the --var-file bullet (L659), the intro (L603-604), and End (run end after passthrough). The HLD ## Epics line (L487) becomes `rehearsal-passthrough — done (#PRs)`. Delete docs/epics/rehearsal-passthrough.md. Edit STATUS.md only if Now names the epic. Run humanizer first.

**Done when:**
- `make doc-hygiene` passes. `git grep -n 'epics/rehearsal-passthrough.md'` prints nothing. The PR quotes `go run ./cmd/infrafactory rehearse plan --help`, and its Use line matches the form the new section shows.

**From review (binding; overrides the scope above where they differ):**
- operations.md says apply takes no saved plan file and runs with -auto-approve, and gives the 2-hour deadline beside Prepare's 10 minutes.

The checks in the epic's **Checks before every stage** apply.
