import { test, expect } from '@playwright/test';

test.describe('Scenario navigation', () => {
  test('each scenario page loads correct data via direct navigation', async ({ page }) => {
    // Navigate to the first scenario
    await page.goto('/scenarios/training/web-app-paris');
    await expect(page.locator('main h1')).toContainText('web-app-paris');
    const yaml1 = await page.locator('main textarea').inputValue();
    expect(yaml1).toBeTruthy();

    // Navigate to a different scenario via page.goto
    await page.goto('/scenarios/training/iam-policies-paris');
    await expect(page.locator('main h1')).toContainText('iam-policies-paris');
    const yaml2 = await page.locator('main textarea').inputValue();
    expect(yaml2).toBeTruthy();
    expect(yaml2).not.toEqual(yaml1);

    // Navigate to a third scenario
    await page.goto('/scenarios/training/k8s-cluster-paris');
    await expect(page.locator('main h1')).toContainText('k8s-cluster-paris');
    const yaml3 = await page.locator('main textarea').inputValue();
    expect(yaml3).toBeTruthy();
    expect(yaml3).not.toEqual(yaml2);
  });

  // Regression test: clicking sidebar links must update the scenario
  // detail without a full page reload. Fixed by using afterNavigate
  // instead of onMount (which only fires once per component lifecycle).
  test('sidebar click navigation updates scenario data', async ({ page }) => {
    await page.goto('/scenarios/training/web-app-paris');
    await expect(page.locator('main h1')).toContainText('web-app-paris');

    // Click a different scenario in the sidebar
    const iamLink = page.locator('aside a[href="/scenarios/training/iam-policies-paris"]');
    await expect(iamLink).toBeVisible();
    await iamLink.click();
    await page.waitForURL('**/scenarios/training/iam-policies-paris');

    // This should update but currently doesn't due to the reactivity bug
    await expect(page.locator('main h1')).toContainText('iam-policies-paris', { timeout: 5_000 });
  });

  test('scenario page shows Next Run Mode card', async ({ page }) => {
    await page.goto('/scenarios/training/web-app-paris');
    await expect(page.locator('main h1')).toContainText('web-app-paris');

    // The run mode card heading
    await expect(page.getByText('Next Run Mode')).toBeVisible();
  });

  test('scenario page shows Layer 3 section', async ({ page }) => {
    await page.goto('/scenarios/training/web-app-paris');
    await expect(page.locator('main h1')).toContainText('web-app-paris');

    // Layer 3 checkbox label
    await expect(page.getByText('Layer 3 (Real Scaleway)')).toBeVisible();

    // Credentials status badge (missing in test since no env vars)
    await expect(page.getByText('credentials missing')).toBeVisible();
  });

  test('scenario page shows run controls', async ({ page }) => {
    await page.goto('/scenarios/training/web-app-paris');
    await expect(page.locator('main h1')).toContainText('web-app-paris');

    // Run and Save buttons
    await expect(page.locator('main button', { hasText: 'Run' })).toBeVisible();
    await expect(page.locator('main button', { hasText: 'Save' })).toBeVisible();

    // Checkboxes for --no-destroy and --clean
    await expect(page.getByText('Keep state')).toBeVisible();
    await expect(page.getByText('Force clean')).toBeVisible();
  });

  test('scenario page textarea is editable', async ({ page }) => {
    await page.goto('/scenarios/training/web-app-paris');
    await expect(page.locator('main h1')).toContainText('web-app-paris');

    const textarea = page.locator('main textarea');
    const original = await textarea.inputValue();

    // Type something into the textarea
    await textarea.fill(original + '\n# test comment');
    const updated = await textarea.inputValue();
    expect(updated).toContain('# test comment');
  });
});
