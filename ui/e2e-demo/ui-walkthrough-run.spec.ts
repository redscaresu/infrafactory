import { test, expect } from '@playwright/test';

// UI walkthrough demo (live-run variant): drives the InfraFactory web
// UI through an actual `infrafactory run` of gcp-pubsub — a 2-resource
// GCP Pub/Sub topic→subscription FK chain against fakegcp. The
// scenario reliably converges in 2 LLM iterations (the first fails
// because fakegcp doesn't model google_project_service yet; the LLM
// sees the feedback and drops it on the second attempt). This is
// MORE compelling for the demo than a single-iteration converge —
// the viewer watches the auto-correcting loop in action.
//
// Captured as docs/demo/ui-walkthrough-run.webm via
// playwright-demo.config.ts (video: 'on'). Run from the repo root:
//
//     make demo-ui-run
//
// Pacing: dwell times sized for first-time viewers reading the UI for
// context. The Live page is the centrepiece — generous waits while
// iteration stages light up live so the viewer watches the AI build
// the resources in real time.
//
// Prerequisites: all mocks running (`make mocks-up`) + Claude CLI
// authenticated (or OPENROUTER_API_KEY exported).
test('UI walkthrough: live run of gcp-pubsub', async ({ page }) => {
  test.setTimeout(300_000);

  // 1. Open the scenario page. gcp-pubsub: a Pub/Sub topic + a
  //    pull subscription that depends on it. Small enough to be
  //    readable, with an FK chain so the viewer sees what fakegcp
  //    catches that real plan/validate misses.
  await page.goto('/scenarios/training/gcp-pubsub');
  await expect(page.locator('main h1')).toContainText('gcp-pubsub', { timeout: 10_000 });
  await page.waitForTimeout(3_000);

  // 2. Read the scenario YAML — viewer sees the declared intent
  //    (pubsub + subscription + region_restriction policy).
  const yamlTextarea = page.locator('[data-testid="scenario-yaml"]');
  await expect(yamlTextarea).toBeVisible();
  const yaml = await yamlTextarea.inputValue();
  expect(yaml).toContain('gcp-pubsub');
  expect(yaml).toContain('pubsub');
  await yamlTextarea.scrollIntoViewIfNeeded();
  await page.waitForTimeout(7_000);

  // 3. Surface the Run controls — viewer sees the toggles before the
  //    click so the next step (Run) makes sense in context.
  await page.locator('text=Next Run Mode').first().scrollIntoViewIfNeeded();
  await page.waitForTimeout(3_000);

  // 4. Click Run — full navigation to /live?scenario=...&run_id=...
  const runButton = page.getByRole('button', { name: 'Run', exact: true });
  await expect(runButton).toBeEnabled();
  await runButton.hover();
  await page.waitForTimeout(800);
  await runButton.click();

  // 5. Live page opens. Show the initial "starting" state — the
  //    viewer sees Status: starting → running as the iteration begins.
  await page.waitForURL(/\/live\?/, { timeout: 30_000 });
  await expect(page.locator('main h1')).toContainText('Live Run', { timeout: 10_000 });
  await page.waitForTimeout(3_000);

  // 6. Wait for the first iteration card to render. This is the
  //    moment the AI starts building — the viewer sees the timeline
  //    populate live.
  const iterationTimeline = page.locator('text=Iteration Timeline');
  await expect(iterationTimeline).toBeVisible({ timeout: 60_000 });
  await iterationTimeline.scrollIntoViewIfNeeded();
  await page.waitForTimeout(4_000);

  // 7. Hold while the iteration stages light up. gcp-pubsub typically
  //    fails iteration 1 (google_project_service not modelled in
  //    fakegcp), then iteration 2 retries with corrected HCL and
  //    succeeds. Total wall time ~2–2.5 min. The "Run succeeded"
  //    banner only appears when the entire run completes.
  await expect(page.getByText('Run succeeded')).toBeVisible({ timeout: 240_000 });

  // 8. Hold on the success state so the viewer sees the iteration
  //    breakdown — should show iter 1 (red, with failure detail) +
  //    iter 2 (green, all stages passed).
  await page.waitForTimeout(5_000);

  // 9. Navigate to Runs page — the freshly-completed run sits at the
  //    top of the history list.
  await page.locator('aside a[href="/runs"]').click();
  await expect(page.locator('main h1')).toContainText('Run History', { timeout: 10_000 });
  await page.waitForTimeout(3_500);

  // 10. Click into the gcp-pubsub run we just created — the per-run
  //     page renders the Snapshots picker + Files list + IaC Preview
  //     for the freshly-built resources.
  const runLink = page.locator('a[href^="/runs/gcp-pubsub/"]').first();
  if (await runLink.isVisible().catch(() => false)) {
    await runLink.click();
    await expect(page.locator('main')).toContainText('Snapshots', { timeout: 10_000 });
    await page.waitForTimeout(4_500);

    // Hold on the IaC Preview so the viewer can actually read the
    // generated HCL the AI converged on.
    const iacPreview = page.locator('text=IaC Preview').first();
    if (await iacPreview.isVisible().catch(() => false)) {
      await iacPreview.scrollIntoViewIfNeeded();
      await page.waitForTimeout(8_000);
    }
  }

  // 11. Brief tour of the Runs page on the way out — the completed
  //     run shows in the history list with its terminal status.
  await page.locator('aside a[href="/runs"]').click();
  await expect(page.locator('main h1')).toContainText('Run History', { timeout: 10_000 });
  await page.waitForTimeout(2_000);

  // 12. Close on the home page so the recording ends on a familiar
  //     view.
  await page.locator('aside a[href="/"]').click();
  await expect(page.locator('h1')).toContainText('InfraFactory Dashboard');
  await page.waitForTimeout(2_000);
});
