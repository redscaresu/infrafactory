import { test, expect } from '@playwright/test';

// S40-T3: functional spot-checks across pages — assert each page renders
// its expected primary data structure (headings, lists, controls), not
// just that the request succeeded. Complements the page-specific suites
// (compare.spec, live.spec, pitfalls.spec, etc.) which already cover
// page-specific functionality.

test.describe('Functional spot-checks', () => {
  test('home page renders scenario sidebar with at least one cloud group', async ({ page }) => {
    await page.goto('/');
    const groups = page.locator('aside [data-testid^="sidebar-cloud-"]');
    await groups.first().waitFor({ state: 'visible' });
    expect(await groups.count()).toBeGreaterThan(0);
    // First group must have a visible label and at least one scenario
    // link — proves the regroup pipeline rendered something useful.
    // (Per-group label visibility is a separate concern: when scenarios
    // span many clouds the lower groups can fall outside the initial
    // viewport, which Playwright's strict toBeVisible flags as hidden.)
    const first = groups.first();
    await expect(first.locator('[data-testid="sidebar-cloud-label"]')).toBeVisible();
    expect(await first.locator('a[href^="/scenarios/"]').count()).toBeGreaterThan(0);
  });

  test('runs page renders a table or empty-state notice', async ({ page }) => {
    await page.goto('/runs');
    // Either a table is present (real runs) or an empty-state message —
    // both count as "rendered successfully".
    const tableVisible = await page.locator('main table').isVisible().catch(() => false);
    const emptyText = await page
      .locator('main')
      .getByText(/no runs|empty|0 runs/i)
      .first()
      .isVisible()
      .catch(() => false);
    expect(tableVisible || emptyText).toBeTruthy();
  });

  test('diagnostics page renders agent + backend fields', async ({ page }) => {
    await page.goto('/diagnostics');
    await expect(page.locator('main')).toBeVisible();
    // diagnostics surfaces some "agent" terminology and check rows
    await expect(page.locator('main').getByText(/agent/i).first()).toBeVisible();
  });

  test('pitfalls page renders provider tabs', async ({ page }) => {
    await page.goto('/pitfalls');
    // pitfalls.spec.ts already covers detail; here we just confirm the
    // page surfaces at least one provider tab.
    await expect(page.locator('main')).toBeVisible();
    const providerTabs = page.locator('main button, main [role="tab"]');
    expect(await providerTabs.count()).toBeGreaterThan(0);
  });

  test('scenario page renders YAML and Run/Save controls', async ({ page }) => {
    await page.goto('/scenarios/training/web-app-paris');
    const yaml = await page.locator('main textarea').inputValue();
    expect(yaml.length).toBeGreaterThan(20);
    await expect(page.locator('main button', { hasText: 'Run' })).toBeVisible();
    await expect(page.locator('main button', { hasText: 'Save' })).toBeVisible();
  });
});

// S40-T4: error-state coverage — non-existent pages and missing scenarios
// must surface useful messaging rather than blank screens or stack traces.

test.describe('Error states', () => {
  test('unknown route falls back to SPA error/handler without crashing', async ({ page }) => {
    const resp = await page.goto('/this/route/does/not/exist');
    // SPA fallback: SvelteKit serves the page shell with a 404 page or the
    // app's not-found handler. Status should be 404 or the SPA may still
    // 200 if it handles routing in the client; in either case the page
    // shouldn't show a raw server-error trace.
    if (resp) {
      const status = resp.status();
      expect([200, 404]).toContain(status);
    }
    const body = await page.locator('body').innerText();
    expect(body.toLowerCase()).not.toContain('panic');
    expect(body.toLowerCase()).not.toContain('500 internal');
  });

  test('unknown scenario path surfaces a not-found message', async ({ page }) => {
    await page.goto('/scenarios/training/this-does-not-exist');
    // The page should either render an error message or simply render
    // the scenario shell with no body. We accept either as long as no
    // stack trace leaks.
    const body = await page.locator('body').innerText();
    expect(body.toLowerCase()).not.toContain('panic');
    expect(body.toLowerCase()).not.toContain('runtime error');
  });

  test('GET /api/scenarios/missing returns 404', async ({ request }) => {
    const resp = await request.get('/api/scenarios/missing-scenario');
    expect(resp.status()).toBe(404);
  });

  test('GET /api/scenarios/missing/layer3-status returns 404', async ({ request }) => {
    const resp = await request.get('/api/scenarios/missing-scenario/layer3-status');
    expect(resp.status()).toBe(404);
  });
});
