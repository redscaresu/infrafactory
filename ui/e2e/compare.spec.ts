import { test, expect, type APIRequestContext } from '@playwright/test';

type RunSummary = { scenario: string; run_id: string; status?: string };

/**
 * Find a scenario that has at least two persisted runs whose generated/
 * snapshots are diffable. The /api/runs/{scenario}/compare endpoint returns
 * 404 when generated/ files are missing for either run, so we must probe
 * the compare endpoint itself rather than just count run records.
 */
async function findComparableScenario(
  request: APIRequestContext
): Promise<{ scenario: string; run1: string; run2: string } | null> {
  const resp = await request.get('/api/runs');
  if (!resp.ok()) return null;
  const body = await resp.json();
  const runs: RunSummary[] = body.runs || [];

  const byScenario = new Map<string, string[]>();
  for (const r of runs) {
    if (!r.scenario || !r.run_id) continue;
    const list = byScenario.get(r.scenario) || [];
    list.push(r.run_id);
    byScenario.set(r.scenario, list);
  }

  for (const [scenario, runIDs] of byScenario) {
    if (runIDs.length < 2) continue;
    runIDs.sort();
    // Newest two ids.
    const run1 = runIDs[runIDs.length - 2];
    const run2 = runIDs[runIDs.length - 1];
    const probe = await request.get(
      `/api/runs/${encodeURIComponent(scenario)}/compare?run1=${encodeURIComponent(run1)}&run2=${encodeURIComponent(run2)}`
    );
    if (probe.ok()) {
      const data = await probe.json();
      if ((data.diffs || []).length > 0) {
        return { scenario, run1, run2 };
      }
    }
  }
  return null;
}

test.describe('Compare page', () => {
  test('loads via sidebar and shows compare section', async ({ page }) => {
    await page.goto('/');
    await page.click('aside nav a[href="/compare"]');
    await page.waitForURL('**/compare');
    await expect(page).toHaveURL(/\/compare$/);
    await expect(page.locator('[data-testid="compare-section"]')).toBeVisible();
    await expect(page.locator('main h1')).toContainText('Compare Runs');
  });

  test('selecting a scenario with <2 runs leaves Compare disabled', async ({ page, request }) => {
    const resp = await request.get('/api/runs');
    const body = await resp.json();
    const runs: RunSummary[] = body.runs || [];
    const counts = new Map<string, number>();
    for (const r of runs) counts.set(r.scenario, (counts.get(r.scenario) || 0) + 1);
    const single = [...counts.entries()].find(([, n]) => n < 2)?.[0];
    if (!single) {
      test.skip(true, 'no scenarios with fewer than 2 runs available');
      return;
    }

    await page.goto('/compare');
    await expect(page.locator('[data-testid="compare-section"]')).toBeVisible();
    await page.locator('[data-testid="compare-scenario"]').selectOption(single);
    // Both run selectors empty → Compare disabled.
    await expect(page.locator('[data-testid="compare-run"]')).toBeDisabled();
  });

  test('comparing two runs renders file list and diff', async ({ page, request }) => {
    const target = await findComparableScenario(request);
    if (!target) {
      test.skip(true, 'no scenario has two persisted runs with generated snapshots');
      return;
    }

    await page.goto('/compare');
    await expect(page.locator('[data-testid="compare-section"]')).toBeVisible();

    await page.locator('[data-testid="compare-scenario"]').selectOption(target.scenario);
    await page.locator('[data-testid="compare-run1"]').selectOption(target.run1);
    await page.locator('[data-testid="compare-run2"]').selectOption(target.run2);

    const compareBtn = page.locator('[data-testid="compare-run"]');
    await expect(compareBtn).toBeEnabled();
    await compareBtn.click();

    const fileList = page.locator('[data-testid="compare-files"]');
    await expect(fileList).toBeVisible({ timeout: 10_000 });

    const fileRows = page.locator('[data-testid^="compare-file-"]');
    expect(await fileRows.count()).toBeGreaterThan(0);

    // Each row shows a status badge.
    const badge = page.locator('[data-testid^="compare-status-"]').first();
    await expect(badge).toHaveText(/^(added|removed|modified|unchanged)$/);

    await expect(page.locator('[data-testid="compare-diff"]')).toBeVisible();
  });

  test('clicking a different file updates the diff content', async ({ page, request }) => {
    const target = await findComparableScenario(request);
    if (!target) {
      test.skip(true, 'no scenario has two persisted runs with generated snapshots');
      return;
    }

    await page.goto('/compare');
    await page.locator('[data-testid="compare-scenario"]').selectOption(target.scenario);
    await page.locator('[data-testid="compare-run1"]').selectOption(target.run1);
    await page.locator('[data-testid="compare-run2"]').selectOption(target.run2);
    await page.locator('[data-testid="compare-run"]').click();

    await expect(page.locator('[data-testid="compare-files"]')).toBeVisible({ timeout: 10_000 });
    const fileRows = page.locator('[data-testid^="compare-file-"]');
    const rowCount = await fileRows.count();
    if (rowCount < 2) {
      test.skip(true, 'compare yielded fewer than 2 file entries — cannot exercise file switching');
      return;
    }

    const diff = page.locator('[data-testid="compare-diff"]');
    const initialText = (await diff.innerText()).trim();

    // Try every other row until the diff text changes.
    let switched = false;
    for (let i = 1; i < rowCount; i++) {
      await fileRows.nth(i).click();
      // Allow Svelte reactive update to flush.
      await page.waitForTimeout(100);
      const next = (await diff.innerText()).trim();
      if (next !== initialText) {
        switched = true;
        break;
      }
    }
    expect(switched).toBe(true);
  });
});
