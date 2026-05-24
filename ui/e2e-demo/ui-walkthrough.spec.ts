import { test, expect } from '@playwright/test';

// UI walkthrough demo: drives the InfraFactory web UI through the
// most resource-dense Scaleway scenario (full-stack-paris: VPC +
// private network + Postgres + GKE + Redis + container registry +
// IAM). Captured as docs/demo/ui-walkthrough.webm via
// playwright-demo.config.ts (video: 'on'). Run from the repo root:
//
//     make demo-ui
//
// The recording is deliberately slow-paced (explicit waits + scroll
// + click flow) so a viewer can follow along. There is no
// `infrafactory run` call here — that would need an LLM credential
// and would make the recording length depend on model latency. The
// run demonstration is the matching CLI cast at
// docs/demo/infrafactory.cast (gcp-full-stack scenario).
test('UI walkthrough: full-stack-paris', async ({ page }) => {
  test.setTimeout(120_000);

  // 1. Landing page — the dashboard groups every training scenario by
  //    cloud. Pause so the viewer reads the page chrome.
  await page.goto('/');
  await expect(page.locator('aside')).toBeVisible();
  await expect(page.locator('h1')).toContainText('InfraFactory Dashboard');
  await page.waitForTimeout(2_000);

  // 2. Sidebar groups: Scaleway / GCP / AWS. Hover the Scaleway group
  //    so the viewer sees the per-cloud breakdown before clicking
  //    through.
  await expect(page.locator('[data-testid="sidebar-cloud-scaleway"]')).toBeVisible();
  await expect(page.locator('[data-testid="sidebar-cloud-gcp"]')).toBeVisible();
  await page.locator('[data-testid="sidebar-cloud-scaleway"] h2').hover();
  await page.waitForTimeout(1_500);

  // 3. Open the full-stack-paris scenario — the deepest composition in
  //    the suite (7 resources across compute / network / db / k8s /
  //    cache / registry / iam).
  const fullStackLink = page.locator('[data-testid="sidebar-scenario-training/full-stack-paris"]');
  await fullStackLink.scrollIntoViewIfNeeded();
  await fullStackLink.hover();
  await page.waitForTimeout(1_000);
  await fullStackLink.click();
  await expect(page.locator('main h1')).toContainText('full-stack-paris', { timeout: 10_000 });
  await page.waitForTimeout(1_500);

  // 4. Scenario YAML — let the viewer see the intent declaration. The
  //    textarea contains the full scenario definition.
  const yamlTextarea = page.locator('main textarea').first();
  await expect(yamlTextarea).toBeVisible();
  const yaml = await yamlTextarea.inputValue();
  expect(yaml).toContain('full-stack-paris');
  expect(yaml).toContain('kubernetes');
  expect(yaml).toContain('database');
  await yamlTextarea.scrollIntoViewIfNeeded();
  await page.waitForTimeout(3_000);

  // 5. Run controls — surface the Next Run Mode card + Layer 3 toggle
  //    so the viewer sees the validation pipeline switches that gate
  //    real-cloud deploys.
  await expect(page.getByText('Next Run Mode')).toBeVisible();
  await page.getByText('Next Run Mode').scrollIntoViewIfNeeded();
  await page.waitForTimeout(2_000);

  // 6. Tour the rest of the navigation: Runs page (history),
  //    Compare page (diff between two runs), Pitfalls (the per-cloud
  //    correction database that feeds back into prompts), Diagnostics.
  await page.locator('aside a[href="/runs"]').click();
  await expect(page.locator('main')).toBeVisible();
  await page.waitForTimeout(2_500);

  await page.locator('aside a[href="/compare"]').click();
  await expect(page.locator('main')).toBeVisible();
  await page.waitForTimeout(2_500);

  await page.locator('aside a[href="/pitfalls"]').click();
  await expect(page.locator('main')).toBeVisible();
  // Pitfalls page surfaces provider tabs — let the viewer see the
  // per-cloud breakdown.
  await page.waitForTimeout(3_000);

  await page.locator('aside a[href="/diagnostics"]').click();
  await expect(page.locator('main')).toBeVisible();
  await page.waitForTimeout(2_000);

  // 7. Final pause on the home page so the recording closes on the
  //    starting view.
  await page.locator('aside a[href="/"]').click();
  await expect(page.locator('h1')).toContainText('InfraFactory Dashboard');
  await page.waitForTimeout(2_000);
});
