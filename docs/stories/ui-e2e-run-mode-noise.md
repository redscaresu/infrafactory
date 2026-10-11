---
kind: code
status: ready
touches: ["ui/e2e/*.spec.ts", "ui/e2e/visual.spec.ts-snapshots/*", "ui/playwright.config.ts"]
---

# The browser tests stop hitting a mock server that is not there: run-mode is answered by the test, and the suite's log carries no "could not determine the run mode"

The Playwright suite starts `infrafactory ui` (`ui/playwright.config.ts:39-49`) with no mock on
:8080. Every scenario page then calls `GET /api/scenarios/<path>/run-mode`
(`ui/src/lib/api.ts:62`), and the handler fails reading mock state
(`internal/api/handlers_scenarios.go:548-551`). The result is a 500, and `[WebServer] ... this server
could not determine the run mode: fetch mock state: ... connection refused` in the log, once per
page load: 114 times in main's CI run for #404. The tests pass, but the noise buries real
server errors, and the visual baselines are taken over the error state:
`ui/e2e/visual.spec.ts:28` masks `scenario-run-mode-error`.

Answer run-mode in the tests, as `deployments.spec.ts` does for `/api/deployments`
(`page.route`), with a fixed `ScenarioRunModeResponse`. Do not start a real mock for the UI suite,
and do not change the Go handler: a 500 when the mock is unreachable is correct behaviour for a
real server.

**Done when:**
- A full `make ui-test-e2e` log (and CI's) contains zero `could not determine the run mode` lines.
  A check in the suite or the workflow fails if one appears.
- The scenario page's visual baselines show the run-mode control in its loaded state. The
  `scenario-run-mode-error` mask is gone, and the baselines are regenerated for the runners CI
  uses.
- One spec still drives the run-mode error state through a routed 500, and asserts what the page
  shows.
- No Go file changes.
