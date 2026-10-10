---
kind: code
status: ready
depends_on: []
touches: ["ui/e2e/spot-checks.spec.ts"]
---

# The runs-page spot-check waits for the page instead of sampling it once

`ui/e2e/spot-checks.spec.ts` "runs page renders a table or empty-state notice" failed CI on #441
(2026-10-10), a docs-only PR. It calls `isVisible()` once right after `page.goto('/runs')`, so a
page still loading its runs reads as neither a table nor the empty state. Wait for one of the two
with Playwright's retrying assertions (for example `expect(page.locator('main table').or(<the empty
state>)).toBeVisible()`), and name the empty state's real text rather than the /no runs|empty|0
runs/ guess.

**Done when:**
- The spot-check uses a retrying assertion, passes 50 runs in a row locally (`--repeat-each=50`),
  and fails if the runs page renders neither (mutation).
