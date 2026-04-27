import { test, expect, type Page } from '@playwright/test';
import { readFileSync, writeFileSync, statSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

// The edit-save-reload test below rewrites pitfalls/<provider>.yaml in
// the actual repo via the API's atomic-rename. To avoid leaving the
// working tree dirty (whether the test passes, fails, or the YAML
// formatter normalises the seeded folded scalars), snapshot the bytes
// before each test and restore them after.
const __dirname = dirname(fileURLToPath(import.meta.url));
const PITFALLS_DIR = resolve(__dirname, '..', '..', 'pitfalls');
function snapshotPitfalls(): Map<string, Buffer> {
  const snap = new Map<string, Buffer>();
  for (const provider of ['aws', 'gcp', 'scaleway']) {
    const path = resolve(PITFALLS_DIR, `${provider}.yaml`);
    try {
      statSync(path);
      snap.set(path, readFileSync(path));
    } catch {
      // file missing — leave snapshot entry absent.
    }
  }
  return snap;
}
function restorePitfalls(snap: Map<string, Buffer>) {
  for (const [path, bytes] of snap) {
    writeFileSync(path, bytes);
  }
}

// The /pitfalls page reads pitfalls/<provider>.yaml files at startup. The
// S43-T11 added pitfalls/aws.yaml. The repo now seeds aws.yaml +
// gcp.yaml + scaleway.yaml, so the alphabetically-first provider
// is 'aws' and the second is 'gcp'.
const FIRST_PROVIDER = 'aws';
const SECOND_PROVIDER = 'gcp';

async function gotoPitfalls(page: Page) {
  await page.goto('/');
  await page.click('aside nav a[href="/pitfalls"]');
  await page.waitForURL('**/pitfalls');
  await expect(page.locator('main h1')).toContainText('Pitfalls');
}

async function selectProvider(page: Page, provider: string) {
  await page.click(`[data-testid="pitfalls-tab-${provider}"]`);
  await expect(page.locator('[data-testid="pitfalls-section"]')).toHaveAttribute(
    'data-provider',
    provider,
    { timeout: 5_000 }
  );
}

test.describe('Pitfalls page', () => {
  let pitfallsSnapshot: Map<string, Buffer>;
  test.beforeEach(() => {
    pitfallsSnapshot = snapshotPitfalls();
  });
  test.afterEach(() => {
    if (pitfallsSnapshot) restorePitfalls(pitfallsSnapshot);
  });

  test('loads via sidebar and shows provider tabs', async ({ page }) => {
    await gotoPitfalls(page);
    await expect(page.locator('[data-testid="pitfalls-load-error"]')).toHaveCount(0);
    // At least one of the seeded providers must be present.
    const tabs = page.locator(
      `[data-testid="pitfalls-tab-${FIRST_PROVIDER}"], [data-testid="pitfalls-tab-${SECOND_PROVIDER}"]`
    );
    await expect(tabs.first()).toBeVisible();
  });

  test('default tab is the first provider alphabetically and renders rows', async ({ page }) => {
    await gotoPitfalls(page);
    await expect(page.locator('[data-testid="pitfalls-section"]')).toHaveAttribute(
      'data-provider',
      FIRST_PROVIDER
    );
    const rows = page.locator('[data-testid="pitfalls-row"]');
    await expect(rows.first()).toBeVisible({ timeout: 5_000 });
    expect(await rows.count()).toBeGreaterThan(0);
    // Source badges render with one of the documented label values.
    const badge = page.locator('[data-testid="pitfalls-source-badge"]').first();
    await expect(badge).toHaveText(/^(static|learned)$/);
  });

  test('switching provider tabs updates the active section and row set', async ({ page }) => {
    await gotoPitfalls(page);
    const firstCount = await page.locator('[data-testid="pitfalls-row"]').count();
    expect(firstCount).toBeGreaterThan(0);

    await selectProvider(page, SECOND_PROVIDER);
    await expect(
      page.locator(`[data-testid="pitfalls-tab-${SECOND_PROVIDER}"]`)
    ).toHaveAttribute('aria-selected', 'true');
    const secondCount = await page.locator('[data-testid="pitfalls-row"]').count();
    expect(secondCount).toBeGreaterThan(0);
  });

  test('add then delete a row toggles the row count without saving', async ({ page }) => {
    await gotoPitfalls(page);
    // Wait for the seeded table to render before snapshotting the row count;
    // otherwise a slow first paint races with `rows.count()` and `before` is
    // captured at 0 instead of the seeded total.
    const rows = page.locator('[data-testid="pitfalls-row"]');
    await expect(rows.first()).toBeVisible();
    const before = await rows.count();

    await page.click('[data-testid="pitfalls-add"]');
    await expect(rows).toHaveCount(before + 1);

    // Delete the just-added row (last row in the table).
    await page.locator('[data-testid="pitfalls-delete"]').last().click();
    await expect(rows).toHaveCount(before);
  });

  test('edit + save persists across reload, then restore keeps the file clean', async ({ page }) => {
    await gotoPitfalls(page);
    await selectProvider(page, FIRST_PROVIDER);

    // Operate on the first row's Rule textarea so we always have a known target.
    const ruleTextarea = page
      .locator('[data-testid="pitfalls-row"]')
      .first()
      .locator('textarea[aria-label="Rule"]');
    await expect(ruleTextarea).toBeVisible();

    // The page trims rule values before saving (see saveProvider in
    // routes/pitfalls/+page.svelte) and the backend re-marshals the YAML, so
    // any trailing whitespace from the seeded folded scalar is normalised
    // out on first save. We compare against the trimmed form so the test is
    // idempotent across CI re-runs.
    const originalRaw = await ruleTextarea.inputValue();
    const original = originalRaw.trim();
    expect(original.length).toBeGreaterThan(0);
    const marker = ' [e2e-edit-' + Date.now() + ']';
    const edited = original + marker;

    await ruleTextarea.fill(edited);
    await page.click('[data-testid="pitfalls-save"]');
    const status = page.locator('[data-testid="pitfalls-save-status"]');
    await expect(status).toBeVisible({ timeout: 10_000 });
    await expect(status).toHaveText(/^Saved \d+ pitfalls?\.$/);
    // Success messages render in green; failures in red.
    await expect(status).toHaveClass(/text-emerald-700/);

    // Reload the whole page and re-select the same tab; the edited text must persist.
    await page.reload();
    await expect(page.locator('main h1')).toContainText('Pitfalls');
    await selectProvider(page, FIRST_PROVIDER);
    const reloadedTextarea = page
      .locator('[data-testid="pitfalls-row"]')
      .first()
      .locator('textarea[aria-label="Rule"]');
    await expect(reloadedTextarea).toHaveValue(edited, { timeout: 5_000 });

    // Restore the original text so the YAML file content is back to its seeded value.
    await reloadedTextarea.fill(original);
    await page.click('[data-testid="pitfalls-save"]');
    await expect(status).toHaveText(/^Saved \d+ pitfalls?\.$/);

    // Verify the round-trip restored the exact original content.
    const restoredTextarea = page
      .locator('[data-testid="pitfalls-row"]')
      .first()
      .locator('textarea[aria-label="Rule"]');
    await expect(restoredTextarea).toHaveValue(original, { timeout: 5_000 });
  });
});
