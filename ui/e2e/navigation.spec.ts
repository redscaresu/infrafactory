import { test, expect } from '@playwright/test';

test.describe('Navigation', () => {
  test('home page loads and shows scenarios sidebar', async ({ page }) => {
    await page.goto('/');

    // Layout sidebar shows the InfraFactory title link
    await expect(page.locator('aside a[href="/"]')).toContainText('InfraFactory');

    // Sidebar shows TRAINING section heading
    await expect(page.locator('aside h2').first()).toBeVisible();
  });

  test('home page shows dashboard heading', async ({ page }) => {
    await page.goto('/');

    await expect(page.locator('main h1')).toContainText('InfraFactory Dashboard');
  });

  test('runs page loads', async ({ page }) => {
    await page.goto('/runs');

    await expect(page.locator('main h1')).toContainText('Run History');
  });

  test('diagnostics page loads', async ({ page }) => {
    await page.goto('/diagnostics');

    await expect(page.locator('main h1')).toContainText('Backend Diagnostics');
  });

  test('sidebar navigation links work', async ({ page }) => {
    await page.goto('/');

    // Click Runs link in nav
    await page.click('nav a[href="/runs"]');
    await expect(page.locator('main h1')).toContainText('Run History');

    // Click Diagnostics link in nav
    await page.click('nav a[href="/diagnostics"]');
    await expect(page.locator('main h1')).toContainText('Backend Diagnostics');
  });

  test('scenario list shows training scenarios on home page', async ({ page }) => {
    await page.goto('/');

    // The dashboard cards should list known training scenarios
    await expect(page.locator('main').getByText('web-app-paris', { exact: true })).toBeVisible();
  });
});
