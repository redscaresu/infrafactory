import { test, expect } from '@playwright/test';

// UI walkthrough demo (live-run variant): drives the InfraFactory web
// UI through an actual `infrafactory run` of registry-paris — the
// fastest-converging Scaleway scenario (single scaleway_registry_namespace,
// converges in 1 iteration). The recording shows the scenario page,
// the click of Run, the live page populating with iterations / stages,
// and the success banner.
//
// Recorded as docs/demo/ui-walkthrough-run.webm via
// playwright-demo.config.ts (video: 'on'). Run from the repo root:
//
//     make demo-ui-run
//
// Prerequisites: mockway running on :8080 + Claude CLI authenticated
// (or OPENROUTER_API_KEY exported). The run is deterministic enough
// to keep the recording length stable (~60–90s).
test('UI walkthrough: live run of registry-paris', async ({ page }) => {
  test.setTimeout(180_000);

  // 1. Open the scenario page directly — viewers don't need to see the
  //    sidebar tour again (separate ui-walkthrough.spec.ts covers that).
  await page.goto('/scenarios/training/registry-paris');
  await expect(page.locator('main h1')).toContainText('registry-paris', { timeout: 10_000 });
  await page.waitForTimeout(2_000);

  // 2. Let the viewer read the scenario YAML — single registry resource,
  //    region_restriction + destruction acceptance criteria.
  const yamlTextarea = page.locator('[data-testid="scenario-yaml"]');
  await expect(yamlTextarea).toBeVisible();
  const yaml = await yamlTextarea.inputValue();
  expect(yaml).toContain('registry-paris');
  expect(yaml).toContain('registry');
  await yamlTextarea.scrollIntoViewIfNeeded();
  await page.waitForTimeout(3_000);

  // 3. Surface the Run controls so the viewer sees the toggles before
  //    the click.
  await expect(page.getByText('Next Run Mode')).toBeVisible();
  await page.getByText('Next Run Mode').scrollIntoViewIfNeeded();
  await page.waitForTimeout(2_000);

  // 4. Click Run — full navigation to /live?scenario=...&run_id=...
  const runButton = page.getByRole('button', { name: 'Run', exact: true });
  await expect(runButton).toBeEnabled();
  await runButton.hover();
  await page.waitForTimeout(500);
  await runButton.click();

  // 5. Wait for navigation to the Live page.
  await page.waitForURL(/\/live\?/, { timeout: 30_000 });
  await expect(page.locator('main')).toBeVisible();
  // Pause so the viewer sees the initial "starting" state.
  await page.waitForTimeout(2_000);

  // 6. Wait for the live page to fill in. The iteration timeline grows
  //    as the run progresses; the success banner ("Run succeeded")
  //    only appears when the run completes. registry-paris converges
  //    in 1 iteration with default config — typically 45–75s end-to-end
  //    against mockway + Claude.
  await expect(page.getByText('Run succeeded')).toBeVisible({ timeout: 150_000 });

  // 7. Hold on the success state so the viewer sees the iteration
  //    breakdown (generate/validate/test stages all green).
  await page.waitForTimeout(4_000);

  // 8. Brief tour of the Runs page — the completed run now shows in
  //    the history with its terminal status.
  await page.locator('aside a[href="/runs"]').click();
  await expect(page.locator('main')).toBeVisible();
  await page.waitForTimeout(3_000);

  // 9. Close on the home page so the recording ends on a familiar view.
  await page.locator('aside a[href="/"]').click();
  await expect(page.locator('h1')).toContainText('InfraFactory Dashboard');
  await page.waitForTimeout(1_500);
});
