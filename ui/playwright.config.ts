import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  retries: 0,
  use: {
    baseURL: 'http://127.0.0.1:4173',
    headless: true,
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
