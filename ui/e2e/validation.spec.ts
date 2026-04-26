import { test, expect, type Page } from '@playwright/test';

// 500ms debounce + a comfortable margin so the validation request always settles.
const DEBOUNCE_WAIT_MS = 800;

const SCENARIO_PATH = '/scenarios/training/web-app-paris';

async function openScenario(page: Page) {
  await page.goto(SCENARIO_PATH);
  await expect(page.locator('main h1')).toContainText('web-app-paris');
  const textarea = page.locator('[data-testid="scenario-yaml"]');
  await expect(textarea).toBeVisible();
  // Wait until the textarea has actual scenario content loaded.
  await expect.poll(async () => (await textarea.inputValue()).length, {
    timeout: 10_000
  }).toBeGreaterThan(0);
  return textarea;
}

test.describe('Real-time scenario validation', () => {
  test('seeded scenario reports valid after the debounce window', async ({ page }) => {
    await openScenario(page);
    await page.waitForTimeout(DEBOUNCE_WAIT_MS);
    await expect(page.locator('[data-testid="scenario-validation-valid"]')).toBeVisible({
      timeout: 5_000
    });
  });

  test('invalid edit transitions valid → errors with at least one entry', async ({ page }) => {
    const textarea = await openScenario(page);
    const original = await textarea.inputValue();

    try {
      // Wait for initial valid state so we know the baseline.
      await page.waitForTimeout(DEBOUNCE_WAIT_MS);
      await expect(page.locator('[data-testid="scenario-validation-valid"]')).toBeVisible({
        timeout: 5_000
      });

      // Break the cloud value — schema enforces an enum and this should fail
      // validation without producing yaml syntax errors.
      const broken = original.replace(/cloud:\s*scaleway/, 'cloud: aws');
      expect(broken).not.toEqual(original);
      await textarea.fill(broken);

      await page.waitForTimeout(DEBOUNCE_WAIT_MS);
      const errors = page.locator('[data-testid="scenario-validation-errors"]');
      await expect(errors).toBeVisible({ timeout: 5_000 });
      const items = errors.locator('li');
      expect(await items.count()).toBeGreaterThan(0);
    } finally {
      // Always restore the original text so the textarea state on disk is
      // unchanged for subsequent runs.
      await textarea.fill(original);
      await page.waitForTimeout(DEBOUNCE_WAIT_MS);
    }
  });

  test('garbage YAML surfaces a yaml syntax error', async ({ page }) => {
    const textarea = await openScenario(page);
    const original = await textarea.inputValue();

    try {
      await page.waitForTimeout(DEBOUNCE_WAIT_MS);

      // An unterminated flow sequence is a YAML parse error (vs. a schema
      // violation), so the backend reports it via the "yaml syntax: …" path.
      await textarea.fill('cloud: scaleway\nresources: [\n');
      await page.waitForTimeout(DEBOUNCE_WAIT_MS);

      const errors = page.locator('[data-testid="scenario-validation-errors"]');
      await expect(errors).toBeVisible({ timeout: 5_000 });
      await expect(errors).toContainText(/yaml syntax/i);
    } finally {
      await textarea.fill(original);
      await page.waitForTimeout(DEBOUNCE_WAIT_MS);
    }
  });

  test('restoring a valid edit returns the validation panel to valid', async ({ page }) => {
    const textarea = await openScenario(page);
    const original = await textarea.inputValue();

    try {
      await page.waitForTimeout(DEBOUNCE_WAIT_MS);
      await expect(page.locator('[data-testid="scenario-validation-valid"]')).toBeVisible({
        timeout: 5_000
      });

      // Add a benign trailing comment — still valid YAML and schema-compliant.
      await textarea.fill(original + '\n# e2e validation comment\n');
      await page.waitForTimeout(DEBOUNCE_WAIT_MS);
      await expect(page.locator('[data-testid="scenario-validation-valid"]')).toBeVisible({
        timeout: 5_000
      });
    } finally {
      await textarea.fill(original);
      await page.waitForTimeout(DEBOUNCE_WAIT_MS);
    }
  });
});
