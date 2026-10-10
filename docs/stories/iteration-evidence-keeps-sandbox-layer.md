---
kind: code
status: ready
depends_on: []
touches: ["internal/cli/run_command.go", "internal/cli/run_command_test.go"]
---

# A failed real apply is recorded at layer sandbox_deploy in iteration.json, not flattened to run

Seen on real AWS (aws-learned-pitfall-from-real-aws, run 20261010T114213Z): iteration 1's real apply
failed with InvalidSubnet.Range, and the pitfall learner recorded `learned_layer: sandbox_deploy`,
but iteration.json holds the failure only as `layer: run, stage: iteration_1_test, check: apply`,
with no sandbox_deploy apply stage. Evidence readers (aws-layer3-run-evidence's per-iteration
records, the epic's checks) cannot tell a real-cloud apply failure from a mock one there.

Keep the inner failure's layer in the run's per-iteration record: a sandbox_deploy apply failure
appears as layer sandbox_deploy (stage iteration_N_apply, check apply), the same way passing
sandbox stages are numbered today.

**Done when:**
- A run whose Layer 3 apply fails writes iteration.json with a failure at layer sandbox_deploy, stage
  iteration_1_apply, check apply, and the existing run/iteration_1_test summary is unchanged or
  removed consistently. Shown to fail on today's code (mutation).
