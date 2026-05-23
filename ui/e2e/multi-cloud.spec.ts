import { test, expect } from '@playwright/test';

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
    await expect(page.getByTestId('scenario-layer3-label')).toHaveText('Layer 3 (Real Scaleway)');
  });

  test('gcp scenario shows GCP badge and GCP Layer 3 label', async ({ page }) => {
    // pick any gcp-* scenario; gcp-gke-cluster is part of S36-T10 set.
    await page.goto('/scenarios/training/gcp-gke-cluster');

    await expect(page.getByTestId('scenario-cloud-badge')).toHaveText('GCP');
    await expect(page.getByTestId('scenario-layer3-label')).toHaveText('Layer 3 (Real GCP)');
  });

  test('mock-provider status pill reflects the scenario cloud', async ({ page }) => {
    // Scaleway scenario surfaces mockway, GCP surfaces fakegcp (or falls back
    // to mockway when fakegcp.url is unconfigured — accept either to keep the
    // assertion robust to test-server config).
    await page.goto('/scenarios/training/web-app-paris');
    await expect(page.getByTestId('scenario-mock-status')).toContainText('mockway');

    await page.goto('/scenarios/training/gcp-gke-cluster');
    await expect(page.getByTestId('scenario-mock-status')).toContainText(/fakegcp|mockway/);
  });
});
