# Review pass 187 — the UI suites run in CI

`codex exec review --base main` on `ci/run-the-ui-suites`. Two passes, one
finding, accepted; second pass clean.

## Why

`ci.yml` ran `go test -race -count=2 ./...` and nothing else. **153 UI unit
tests and 135 Playwright tests existed and none of them ran on a pull
request.** They were guarded by a local pre-commit hook — which is exactly as
strong as the discipline of whoever is committing, and `--no-verify` is one
flag away. It was used on every commit in the session that found this.

For a project whose whole discipline is *drift becomes a failed test*, a suite
outside CI is a suite that drifts.

## The finding

**[P1] visual regression would have failed on day one.** `toHaveScreenshot()`
baselines are platform-specific: 8 `*-chromium-darwin.png` in the repo, zero
`*-chromium-linux.png`. On `ubuntu-latest` every one fails on a missing
baseline — so the change as first written would have made CI permanently red,
which is worse than not running it.

Excluded, with the limitation stated rather than hidden. Generating Linux
baselines is not a one-liner: the config's `webServer` is
`go run ./cmd/infrafactory ui`, so the official Playwright container — Node, no
Go — cannot start the app under test. That needs an image with both and is
worth its own change.

**127 functional tests now gate a pull request.** The 8 visual ones remain a
local, macOS-only check.

## The failing test that turned out not to be failing

`scenario page shows Layer 3 section` had been failing locally on every branch,
including `main`. It was diagnosed twice and wrongly:

1. "leaked `SCW_*` from the shell" — the shell had none
2. "pre-existing on main, unrelated to this work" — true, and not the cause

The cause is `reuseExistingServer: !process.env.CI`. Locally, Playwright
**adopts** whatever is already on :4173 rather than starting its own. With the
demo UI running — started with real Scaleway credentials — the scenario page
correctly reports `credentials ready` where the test expects `credentials
missing`. The test was right the whole time; the environment was contaminated.

In CI, `reuseExistingServer` is false and the runner has no credentials, so it
passes.

`make ui-test-e2e` now refuses to run while :4173 is occupied, and says why,
rather than producing a failure whose cause is three layers away from its
message.

## Verified where it can be, and not claimed where it cannot

The CI steps have not been executed locally — the port guard now (correctly)
refuses while the demo UI is up, and killing it was not mine to do. **CI is the
verification for this PR.** If the Playwright step fails there, that is the
change doing its job on its first run.
