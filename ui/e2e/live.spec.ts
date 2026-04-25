import { test, expect } from '@playwright/test';

type Run = { scenario: string; run_id: string; status?: string };

/** Find a run with at least one iteration, optionally requiring failures. */
async function findRunWithIterations(
  request: any,
  opts: { requireFailures?: boolean } = {}
): Promise<{ scenario: string; runID: string } | null> {
  const resp = await request.get('/api/runs');
  const body = await resp.json();
  const runs: Run[] = body.runs || [];

  // Prefer completed (non-running) runs, iterate from the newest.
  for (const run of runs) {
    if (run.status === 'running') continue;
    const s = run.scenario;
    const r = run.run_id;
    try {
      const iterResp = await request.get(`/api/runs/${s}/${r}/iterations/1`);
      if (!iterResp.ok()) continue;
      const iterData = await iterResp.json();

      if (opts.requireFailures) {
        if (!iterData.failures || iterData.failures.length === 0) {
          // Check up to 5 iterations for one with failures.
          let found = false;
          for (let n = 2; n <= 5; n++) {
            const nr = await request.get(`/api/runs/${s}/${r}/iterations/${n}`);
            if (!nr.ok()) break;
            const nd = await nr.json();
            if (nd.failures && nd.failures.length > 0) { found = true; break; }
          }
          if (!found) continue;
        }
      }

      return { scenario: s, runID: r };
    } catch {
      continue;
    }
  }
  return null;
}

test.describe('Live page', () => {
  test('loads with heading visible', async ({ page }) => {
    await page.goto('/live');
    await expect(page.locator('main h1')).toContainText('Live Run');
  });

  test('shows status message or metadata when no params provided', async ({ page }) => {
    await page.goto('/live');
    await expect(page.locator('main h1')).toContainText('Live Run');

    // With no query params the page auto-selects the latest run.
    // Either a status message appears (no runs) or the metadata card appears.
    const statusOrMeta = page.locator('main').getByText(/No runs recorded|Scenario:/);
    await expect(statusOrMeta).toBeVisible({ timeout: 10_000 });
  });

  test('shows run metadata when a run exists', async ({ page, request }) => {
    const target = await findRunWithIterations(request);
    if (!target) { test.skip(); return; }

    await page.goto(`/live?scenario=${encodeURIComponent(target.scenario)}&run_id=${encodeURIComponent(target.runID)}`);

    // Metadata card fields
    await expect(page.locator('main').getByText('Scenario:')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('main').getByText('Run ID:')).toBeVisible();
    await expect(page.locator('main').getByText('Status:')).toBeVisible();
  });

  test('iteration timeline shows completed iterations', async ({ page, request }) => {
    const target = await findRunWithIterations(request);
    if (!target) { test.skip(); return; }

    await page.goto(`/live?scenario=${encodeURIComponent(target.scenario)}&run_id=${encodeURIComponent(target.runID)}`);

    // Wait for the Iteration Timeline heading (rendered when iterations array is populated).
    await expect(page.getByRole('heading', { name: 'Iteration Timeline' })).toBeVisible({ timeout: 15_000 });

    // At least one iteration card should be rendered inside a <section>.
    const iterationCard = page.locator('section').filter({ hasText: /Iteration \d+/ });
    await expect(iterationCard.first()).toBeVisible();

    // Each card has a pass/fail badge.
    const badge = iterationCard.first().locator('span').filter({ hasText: /passed|failure/ });
    await expect(badge.first()).toBeVisible();
  });

  test('iteration cards show stage pills when stages exist', async ({ page, request }) => {
    const target = await findRunWithIterations(request);
    if (!target) { test.skip(); return; }

    await page.goto(`/live?scenario=${encodeURIComponent(target.scenario)}&run_id=${encodeURIComponent(target.runID)}`);

    await expect(page.getByRole('heading', { name: 'Iteration Timeline' })).toBeVisible({ timeout: 15_000 });

    // Stage pills contain text like "generate: pass" or "validate: fail".
    const stagePill = page.locator('section').filter({ hasText: /Iteration \d+/ })
      .first().locator('span').filter({ hasText: /:\s*(pass|fail|unknown)/ });
    await expect(stagePill.first()).toBeVisible();
  });

  test('failed iterations show retry reason', async ({ page, request }) => {
    const target = await findRunWithIterations(request, { requireFailures: true });
    if (!target) { test.skip(); return; }

    await page.goto(`/live?scenario=${encodeURIComponent(target.scenario)}&run_id=${encodeURIComponent(target.runID)}`);

    await expect(page.getByRole('heading', { name: 'Iteration Timeline' })).toBeVisible({ timeout: 15_000 });

    // "Retry reason:" text appears in failed iteration cards (may appear multiple times).
    await expect(page.getByText('Retry reason:').first()).toBeVisible();
  });
});
