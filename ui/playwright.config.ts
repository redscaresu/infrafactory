import { defineConfig } from '@playwright/test';

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
    baseURL: 'http://127.0.0.1:4173',
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
    command: 'cd .. && go run ./cmd/infrafactory ui --addr 127.0.0.1:4173',
    url: 'http://127.0.0.1:4173/api/config',
    timeout: 30_000,
    reuseExistingServer: !process.env.CI,
  },
});
