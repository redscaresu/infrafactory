---
kind: code
status: ready
depends_on: []
touches: ["internal/cli/layer3_teardown_cloud_test.go"]
---

# TestInterruptedTestOnAnotherCloudTearsNothingDown passes every time, not 9 in 10

It fails about 19 of 200 runs on a clean main (measured 2026-10-10 by the #438 builder), and it
failed main's CI after #437 (`/gcp/marker_only`). A flaky interrupt test either hides a real signal
regression or trains everyone to press re-run. Find the race (timing in how the test sends the
signal or waits for the guard's listener, likely the second-listener wait #429 added) and make the
test deterministic. Change the test's synchronisation, not the code under test, unless the race is
in the code; if it is, say so and fix it there.

**Done when:**
- `go test -tags noui ./internal/cli/ -run TestInterruptedTestOnAnotherCloudTearsNothingDown -count=500 -race`
  passes, with the before and after failure counts recorded in the PR.
- The test still fails if the behaviour it guards is broken (mutation).
