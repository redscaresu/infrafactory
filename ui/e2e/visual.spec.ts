import { test, expect } from '@playwright/test';

// S40-T1 + S40-T2: visual-regression coverage. Playwright captures
// baseline screenshots on first run and pixel-diffs subsequent runs
// against them (threshold tuned in playwright.config.ts). The mask
// pattern hides volatile chrome — session ids, backend timestamps,
// run-id strings — so unrelated state changes don't break the suite.

const VOLATILE_SELECTORS = [
  // Sidebar diagnostics block (session id + start time change every run).
  'aside .text-xs.text-slate-600',
];

test.describe('Visual regression baselines', () => {
  test('home page', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('aside')).toBeVisible();
    await expect(page).toHaveScreenshot('home.png', {
      mask: VOLATILE_SELECTORS.map((sel) => page.locator(sel)),
      fullPage: true,
    });
  });

  test('runs page', async ({ page }) => {
    await page.goto('/runs');
    await expect(page.locator('main')).toBeVisible();
    await expect(page).toHaveScreenshot('runs.png', {
      mask: VOLATILE_SELECTORS.map((sel) => page.locator(sel)),
      fullPage: true,
    });
  });

  test('diagnostics page', async ({ page }) => {
    await page.goto('/diagnostics');
    await expect(page.locator('main')).toBeVisible();
    // Diagnostics surfaces session id + start time inside main; mask
    // the whole main pane and rely on the sidebar comparison for layout.
    await expect(page).toHaveScreenshot('diagnostics.png', {
      mask: VOLATILE_SELECTORS.map((sel) => page.locator(sel)).concat([page.locator('main')]),
      fullPage: true,
    });
  });

  test('pitfalls page', async ({ page }) => {
    await page.goto('/pitfalls');
    await expect(page.locator('main')).toBeVisible();
    await expect(page).toHaveScreenshot('pitfalls.png', {
      mask: VOLATILE_SELECTORS.map((sel) => page.locator(sel)),
      fullPage: true,
    });
  });

  test('compare page', async ({ page }) => {
    await page.goto('/compare');
    await expect(page.locator('main')).toBeVisible();
    await expect(page).toHaveScreenshot('compare.png', {
      mask: VOLATILE_SELECTORS.map((sel) => page.locator(sel)),
      fullPage: true,
    });
  });

  test('scenario page (scaleway)', async ({ page }) => {
    await page.goto('/scenarios/training/web-app-paris');
    await expect(page.locator('main h1')).toContainText('web-app-paris');
    // Mask the YAML textarea — content is large and stable enough that
    // a single-character change would force a re-baseline. We assert
    // layout/chrome, not the YAML body.
    await expect(page).toHaveScreenshot('scenario-scaleway.png', {
      mask: VOLATILE_SELECTORS.map((sel) => page.locator(sel)).concat([page.locator('main textarea')]),
      fullPage: true,
    });
  });

  test('scenario page (gcp)', async ({ page }) => {
    await page.goto('/scenarios/training/gcp-gke-cluster');
    await expect(page.locator('main h1')).toContainText('gcp-gke-cluster');
    await expect(page).toHaveScreenshot('scenario-gcp.png', {
      mask: VOLATILE_SELECTORS.map((sel) => page.locator(sel)).concat([page.locator('main textarea')]),
      fullPage: true,
    });
  });
});
