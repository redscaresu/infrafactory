---
kind: chore
status: ready
risk: high
depends_on: []
touches: [.github/workflows/scenario-gate.yml, scripts/scenario_change_gate.sh]
---

# scenario-gate labels a hang as timeout, fits its job timeout, and uploads only allowlisted logs

Found in review of #605.
- `scripts/scenario_change_gate.sh:74` treats rc 124 as a timeout, but the per-scenario timeout is a
  perl alarm, which exits 142. A hung scenario is never labelled `timeout`. Match the code the
  wrapper actually returns, and test it with a scenario stub that sleeps past a 1-second timeout.
- The job's `timeout-minutes: 90` is below MAX_SCENARIOS x PER_SCENARIO_TIMEOUT (120 min), so a
  run of slow scenarios is cancelled before the script's own limits apply. Make the job timeout
  exceed the product, with a comment showing the arithmetic.
- The job holds OPENROUTER_API_KEY and uploads `/tmp/scenario_gate_logs/` unfiltered. Upload only
  named files (the results TSV and each scenario's log). Before upload, fail the step if any file
  contains the key's value. That check is a match against the secret, not a redaction: never a sed
  that rewrites it.

**Done when:**
- The stub test shows a timed-out scenario's row reads `timeout`.
- The job's timeout-minutes exceeds MAX_SCENARIOS x PER_SCENARIO_TIMEOUT / 60, and a comment shows the sum.
- The upload `path:` lists named files only. The PR body shows the secret check failing on a file seeded with a fake key in a test run.
