import { defineConfig } from '@playwright/test';

// Overridable so the suite can run while a real UI holds the default
// port. It has adopted one before: reuseExistingServer means a
// credentialed server the operator started gets treated as the fixture,
// and the failures that produces look like product bugs.
const PORT = process.env.PLAYWRIGHT_PORT || '4173';
const ORIGIN = `http://127.0.0.1:${PORT}`;

export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  retries: 0,
  // The webServer is a single embedded UI process backed by a single
  // pitfalls/runstore filesystem state, so cross-test races are
  // possible (e.g. compare-page run-list mutations or pitfalls table
  // edits leaking into a sibling worker). Serial execution removes the
  // parallel race surface; the suite still finishes well under the
  // 30s test timeout.
  workers: 1,
  fullyParallel: false,
  use: {
    baseURL: ORIGIN,
    headless: true,
  },
  // S40-T2 visual-regression threshold: small pixel-diff tolerance keeps
  // anti-alias noise from flagging unchanged screens while still catching
  // real visual regressions. maxDiffPixelRatio is the dominant gate;
  // threshold tunes per-pixel color sensitivity.
  expect: {
    toHaveScreenshot: {
      maxDiffPixelRatio: 0.02,
      threshold: 0.2,
    },
  },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
  ],
  webServer: {
    command: `cd .. && go run ./cmd/infrafactory ui --addr 127.0.0.1:${PORT}`,
    url: `${ORIGIN}/api/config`,
    // 30s is a warm-laptop number. `go run` COMPILES the binary, and on
    // a cold CI runner with an empty build cache that alone exceeded it:
    // the first run of this suite in CI failed with "Timed out waiting
    // 30000ms from config.webServer" having never reached a test.
    // Anyone on a fresh clone hits the same wall.
    timeout: 180_000,
    reuseExistingServer: !process.env.CI,
  },
});
