---
kind: chore
status: ready
depends_on: []
touches: [ui/playwright.config.ts]
---

# CI's Playwright run writes the report the upload step looks for

Found in review of #605. ci.yml's "Upload Playwright report" step uploads `ui/playwright-report/` on
`failure() || cancelled()`, but ui/playwright.config.ts sets no `reporter`, so CI uses the default
`list` reporter and nothing writes that directory. The upload always finds nothing. Set
`reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list'` (or the equivalent),
so local runs keep their output.

**Done when:**
- A PR run where one UI spec is made to fail (in a throwaway commit, reverted before review) uploads a `playwright-report` artifact that holds index.html. The PR body links that run.
- Local `make test` output is unchanged (no report opens).
