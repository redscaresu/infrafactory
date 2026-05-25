import { test, expect } from '@playwright/test';

// UI walkthrough demo (tour variant): drives the InfraFactory web UI
// through the most resource-dense Scaleway scenario (full-stack-paris:
// VPC + private network + Postgres + GKE + Redis + container registry +
// IAM) without actually running it. Captured as
// docs/demo/ui-walkthrough.webm via playwright-demo.config.ts
// (video: 'on'). Run from the repo root:
//
//     make demo-ui
//
// Pacing: dwell times are sized for first-time viewers reading the UI
// for context. New screens get 5–7s; YAML / dense text gets 8–12s;
// hovers/transitions get 1.5–2s. Total recording length ~90s.
//
// No `infrafactory run` call here — that needs an LLM credential. The
// matching live-run demo (click Run + watch it build) is at
// docs/demo/ui-walkthrough-run.webm (`make demo-ui-run`).
test('UI walkthrough: full-stack-paris', async ({ page }) => {
  test.setTimeout(180_000);

  // 1. Landing page — dashboard heading + per-cloud scenario sidebar.
  //    The viewer needs to see what InfraFactory does at a glance:
  //    scenario-driven IaC across multiple clouds.
  await page.goto('/');
  await expect(page.locator('aside')).toBeVisible();
  await expect(page.locator('h1')).toContainText('InfraFactory Dashboard');
  await page.waitForTimeout(5_000);

  // 2. Per-cloud sidebar tour — Scaleway, GCP, AWS. Hover each group
  //    so the viewer registers the multi-cloud surface.
  await expect(page.locator('[data-testid="sidebar-cloud-scaleway"]')).toBeVisible();
  await page.locator('[data-testid="sidebar-cloud-scaleway"] h2').hover();
  await page.waitForTimeout(2_500);
  await page.locator('[data-testid="sidebar-cloud-gcp"] h2').hover();
  await page.waitForTimeout(2_500);
  const awsGroup = page.locator('[data-testid="sidebar-cloud-aws"] h2');
  if (await awsGroup.isVisible().catch(() => false)) {
    await awsGroup.hover();
    await page.waitForTimeout(2_500);
  }

  // 3. Click into full-stack-paris — the densest composition.
  const fullStackLink = page.locator('[data-testid="sidebar-scenario-training/full-stack-paris"]');
  await fullStackLink.scrollIntoViewIfNeeded();
  await fullStackLink.hover();
  await page.waitForTimeout(1_500);
  await fullStackLink.click();
  await expect(page.locator('main h1')).toContainText('full-stack-paris', { timeout: 10_000 });
  await page.waitForTimeout(2_500);

  // 4. Scenario YAML — the intent declaration. Dense (~50 lines:
  //    resources block + acceptance criteria). Give the viewer time
  //    to actually read the resource composition.
  const yamlTextarea = page.locator('main textarea').first();
  await expect(yamlTextarea).toBeVisible();
  const yaml = await yamlTextarea.inputValue();
  expect(yaml).toContain('full-stack-paris');
  expect(yaml).toContain('kubernetes');
  expect(yaml).toContain('database');
  await yamlTextarea.scrollIntoViewIfNeeded();
  await page.waitForTimeout(10_000);

  // 5. Real-time validation indicator — scenarios validate as you
  //    type against scenario.schema.json.
  const validation = page.locator('[data-testid="scenario-validation"]');
  if (await validation.isVisible().catch(() => false)) {
    await validation.scrollIntoViewIfNeeded();
    await page.waitForTimeout(3_500);
  }

  // 6. Run-control surface — Next Run Mode card (clean / --no-destroy
  //    toggles) plus the Layer 3 section (real-cloud deploy gate).
  await page.locator('text=Next Run Mode').first().scrollIntoViewIfNeeded();
  await page.waitForTimeout(4_500);
  const layer3 = page.locator('[data-testid="scenario-layer3-label"]');
  if (await layer3.isVisible().catch(() => false)) {
    await layer3.scrollIntoViewIfNeeded();
    await page.waitForTimeout(4_000);
  }

  // 7. Runs page — every completed run is recorded with terminal
  //    status (success / failed / stuck), iteration count, and a
  //    link to the immutable artefacts under .infrafactory/runs.
  await page.locator('aside a[href="/runs"]').click();
  await expect(page.locator('main h1')).toContainText('Run History', { timeout: 10_000 });
  await page.waitForTimeout(5_500);

  // 8. Compare page — pick a scenario, then two run-ids, and the
  //    page renders a per-file diff of the generated HCL. Lets
  //    viewers see what the LLM changed between iterations or
  //    between separate runs.
  await page.locator('aside a[href="/compare"]').click();
  await expect(page.locator('[data-testid="compare-section"]')).toBeVisible({ timeout: 10_000 });
  await page.waitForTimeout(2_500);
  // Hover the scenario picker so the viewer sees it's a populated
  // dropdown of every scenario that has runs.
  const compareScenario = page.locator('[data-testid="compare-scenario"]');
  if (await compareScenario.isVisible().catch(() => false)) {
    await compareScenario.hover();
    await page.waitForTimeout(3_000);
  }
  await page.waitForTimeout(2_500);

  // 9. Pitfalls page — the per-cloud correction database. Static
  //    pitfalls + learned ones (auto-promoted from successful self-
  //    corrections, ADR-0012). Cycle through provider tabs so the
  //    viewer sees the per-cloud breakdown.
  await page.locator('aside a[href="/pitfalls"]').click();
  await expect(page.locator('main h1')).toContainText('Pitfalls', { timeout: 10_000 });
  await page.waitForTimeout(3_000);
  // Click through each provider tab.
  for (const provider of ['scaleway', 'gcp', 'aws']) {
    const tab = page.locator(`[data-testid="pitfalls-tab-${provider}"]`);
    if (await tab.isVisible().catch(() => false)) {
      await tab.click();
      await page.waitForTimeout(2_500);
    }
  }
  // Hover a pitfall row so the viewer sees the source-badge
  // (static / learned).
  const firstRow = page.locator('[data-testid="pitfalls-row"]').first();
  if (await firstRow.isVisible().catch(() => false)) {
    await firstRow.scrollIntoViewIfNeeded();
    await page.waitForTimeout(3_500);
  }

  // 10. Diagnostics page — agent + backend health, mock connection
  //     status, embedded build info.
  await page.locator('aside a[href="/diagnostics"]').click();
  await expect(page.locator('main')).toBeVisible({ timeout: 10_000 });
  await page.waitForTimeout(4_500);

  // 11. Close on the home page so the recording ends on a familiar
  //     starting view.
  await page.locator('aside a[href="/"]').click();
  await expect(page.locator('h1')).toContainText('InfraFactory Dashboard');
  await page.waitForTimeout(2_500);
});
