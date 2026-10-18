---
kind: chore
status: ready
depends_on: []
touches: [scripts/sweep_39.sh]
---

# A sweep's mock reset has a deadline

Found in review of #627. scripts/sweep_39.sh runs `./bin/infrafactory mock reset` before each
scenario with no deadline, so one hung reset hangs the whole sweep. The scenario run now has a
deadline (SWEEP_RUN_TIMEOUT); the reset does not. Bound it the same way, with the same deadline
script and kill grace, and decide what a timed-out reset does: stop the sweep with a named error,
or record it and go on. Pick one and say which in the PR.

**Done when:**
- With the reset's deadline set to a few seconds and a fake reset that never returns, the sweep either goes on to the next scenario with the reset recorded as timed out, or stops with a named error and a non-zero exit. Either way it does not wait for the reset to finish.
- `scripts/sweep_39_test.sh` covers that case, and its new check fails when the reset deadline is removed (the FAIL line goes in the PR body).
