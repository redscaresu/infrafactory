import { defineConfig } from '@playwright/test';

// Standalone Playwright config for the demo recordings. Kept separate
// from playwright.config.ts so the main test suite (which has
// screenshot baselines + workers=1 + retries=0) is not affected by the
// demo's video-capture settings.
//
// Output: a .webm video under docs/demo/walkthrough/ that gets renamed
// + promoted to docs/demo/ui-walkthrough.webm by the matching make
// target. Run via `make demo-ui` from the repo root.
export default defineConfig({
  testDir: './e2e-demo',
  timeout: 120_000,
  workers: 1,
  retries: 0,
  use: {
    baseURL: 'http://127.0.0.1:4173',
    headless: true,
    viewport: { width: 1280, height: 800 },
    video: {
      mode: 'on',
      size: { width: 1280, height: 800 },
    },
  },
  // Drop the video into the repo's docs/demo/ tree rather than the
  // default test-results/ (which is .gitignored). The directory below
  // is .gitignored apart from the curated final-cut .webm file.
  outputDir: '../docs/demo/walkthrough',
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
  ],
  webServer: {
    command: 'cd .. && go run ./cmd/infrafactory ui --addr 127.0.0.1:4173',
    url: 'http://127.0.0.1:4173/api/config',
    timeout: 60_000,
    reuseExistingServer: !process.env.CI,
  },
});
