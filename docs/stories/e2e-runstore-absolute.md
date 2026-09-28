---
kind: code
status: ready
touches: ["internal/e2e/"]
risk: high
---

# The opt-in e2e suites run again: the harness gives run commands an absolute run-store root

Found by #315 (2026-09-28): with `INFRAFACTORY_ENABLE_E2E=1` or
`INFRAFACTORY_ENABLE_REALTOOL_INCREMENTAL=1`, the e2e suites (e.g. `full_stack_paris`,
`web_app_paris`, `scaleway_services`) fail before any policy runs with
`test isolation: runstoreRoot must be absolute under go test` (`internal/cli/run_command.go:79`,
the M81 guard from fc5f1bd). The guard is right; the e2e harness never sets an absolute run-store
root. CI does not run these suites, so the break was silent.

Fix the harness, not the guard: every e2e path that runs `run`/`test` gets an absolute output dir
and run-store root under `t.TempDir()` (the `isolatedRunOpts` shape). Do not weaken or bypass the
guard. Needs mockway/fakeaws running locally; never real cloud.

**Done when:**
- With `INFRAFACTORY_ENABLE_E2E=1` and the mocks up, `go test -tags noui ./internal/e2e/ -count=1`
  no longer reports the isolation error anywhere; the PR lists every e2e test's result (pass, or a
  failure with its cause filed as a story).
- A unit test in the default (ungated) suite fails if the e2e harness's run options would carry a
  relative output dir or run-store root.
- The M81 guard in `internal/cli/run_command.go` is unchanged.
