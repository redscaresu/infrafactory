---
kind: chore
status: ready
depends_on: []
touches: [scripts/sweep_39.sh]
---

# A sweep's scenario run has a deadline

Found in review of #619. scripts/sweep_39.sh:98 runs `./bin/infrafactory run` with no deadline,
so one hung scenario hangs the whole sweep. Wrap it in a timeout (an env var with a default, e.g.
`SWEEP_RUN_TIMEOUT=1800`), record a timed-out run as its own outcome in the sweep's results, and
go on to the next scenario.

**Done when:**
- With the timeout set to a few seconds and a scenario that cannot finish in time, the sweep records it as timed out and runs the next scenario.
- `bash -n` and shellcheck (if the repo runs it) pass.
