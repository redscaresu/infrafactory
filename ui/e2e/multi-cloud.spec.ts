import { test, expect, RUN_MODE_ROUTE } from './fixtures';

// Covers S36-T12 (GCP scenarios visible in UI list) and S42-T5 (multi-cloud
// UI: sidebar group + cloud badge + dynamic Layer 3 label + mock-provider
// status). One scenario per cloud is enough to lock down the per-cloud
// behavior — broader cloud coverage lives in unit tests.

test.describe('Multi-cloud UI', () => {
  test('sidebar groups scenarios by cloud and includes a GCP section', async ({ page }) => {
    await page.goto('/');

    const scalewayGroup = page.locator('aside [data-testid="sidebar-cloud-scaleway"]');
    const gcpGroup = page.locator('aside [data-testid="sidebar-cloud-gcp"]');

    await expect(scalewayGroup).toBeVisible();
    await expect(scalewayGroup.locator('[data-testid="sidebar-cloud-label"]')).toHaveText('SCALEWAY');

    await expect(gcpGroup).toBeVisible();
    await expect(gcpGroup.locator('[data-testid="sidebar-cloud-label"]')).toHaveText('GCP');

    // GCP group must contain at least one of the gcp-* training scenarios.
    const gcpLinks = gcpGroup.locator('a[href^="/scenarios/training/gcp-"]');
    expect(await gcpLinks.count()).toBeGreaterThan(0);
  });

  test('scaleway scenario shows Scaleway badge and SCW Layer 3 label', async ({ page }) => {
    await page.goto('/scenarios/training/web-app-paris');

    await expect(page.getByTestId('scenario-cloud-badge')).toHaveText('Scaleway');
    await expect(page.getByTestId('scenario-layer3-label')).toContainText('Layer 3 (Real Scaleway)');
  });

  test('gcp scenario shows GCP badge and GCP Layer 3 label', async ({ page }) => {
    // pick any gcp-* scenario; gcp-gke-cluster is part of S36-T10 set.
    await page.goto('/scenarios/training/gcp-gke-cluster');

    await expect(page.getByTestId('scenario-cloud-badge')).toHaveText('GCP');
    await expect(page.getByTestId('scenario-layer3-label')).toContainText('Layer 3 (Real GCP)');
  });

  test('mock-provider status pill reflects the scenario cloud', async ({ page }) => {
    // Run-mode is answered per scenario by fixtures.ts: Scaleway reads
    // mockway, GCP reads fakegcp.
    await page.goto('/scenarios/training/web-app-paris');
    await expect(page.getByTestId('scenario-mock-status')).toHaveText('mockway state: no');

    await page.goto('/scenarios/training/gcp-gke-cluster');
    await expect(page.getByTestId('scenario-mock-status')).toHaveText('fakegcp state: no');
  });

  // With no run-mode answer the pill names the mock from the scenario's
  // own cloud, so a GCP page still says fakegcp.
  test('a GCP page whose run-mode fails names fakegcp as unavailable', async ({ page }) => {
    await page.route(RUN_MODE_ROUTE, (route) =>
      route.fulfill({ status: 500, json: { error: 'this server could not determine the run mode; see the server log' } })
    );
    await page.goto('/scenarios/training/gcp-gke-cluster');
    await expect(page.getByTestId('scenario-mock-status')).toHaveText('fakegcp state: unavailable');
  });
});
