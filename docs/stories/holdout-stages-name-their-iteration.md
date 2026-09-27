---
kind: code
status: ready
touches: [internal/cli/holdout.go, internal/cli/run_command.go, internal/cli/testdata/golden]
---

# Holdout stages say which iteration they belong to

Run `20260927T171147Z` reported `holdout/skipped: skip (earlier checks failed…)` beside
`holdout/web-live-paris-unseen: pass`. They were iterations 1 and 2, but the stage names do not say
so, and together they read as a contradiction. Every other per-iteration stage is prefixed
(`run/iteration_N_test`).

**Done when:** each holdout stage names its iteration the way other per-iteration stages do, the
golden files show it, and a test covers a run whose first iteration skips the holdout and whose
second passes it.
