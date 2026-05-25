import { test, expect } from '@playwright/test';

// UI walkthrough demo (live-run variant): drives the InfraFactory web
// UI through an actual `infrafactory run` of lb-paris — a 4-resource
// Scaleway load-balancer stack (scaleway_lb_ip + scaleway_lb +
// scaleway_lb_backend + scaleway_lb_frontend). Multi-resource so the
// viewer sees a real composition being built, but small enough to
// converge reliably in 1 iteration (~30–40s end-to-end).
//
// Captured as docs/demo/ui-walkthrough-run.webm via
// playwright-demo.config.ts (video: 'on'). Run from the repo root:
//
//     make demo-ui-run
//
// Pacing: dwell times sized for first-time viewers reading the UI for
// context. The Live page is the centrepiece — generous waits while
// iteration stages light up live so the viewer watches the AI build
// the stack in real time, not just see the result.
//
// Prerequisites: mockway running on :8080 + Claude CLI authenticated
// (or OPENROUTER_API_KEY exported).
test('UI walkthrough: live run of lb-paris', async ({ page }) => {
  test.setTimeout(240_000);

  // 1. Open the scenario page. lb-paris is a 4-resource Scaleway
  //    load-balancer stack (LB + IP + frontend + backend) — small
  //    enough to converge in one iteration, big enough to be a
  //    genuine multi-resource composition.
  await page.goto('/scenarios/training/lb-paris');
  await expect(page.locator('main h1')).toContainText('lb-paris', { timeout: 10_000 });
  await page.waitForTimeout(3_000);

  // 2. Read the scenario YAML — viewer sees the declared intent
  //    (load_balancer + backends + region_restriction policy).
  const yamlTextarea = page.locator('[data-testid="scenario-yaml"]');
  await expect(yamlTextarea).toBeVisible();
  const yaml = await yamlTextarea.inputValue();
  expect(yaml).toContain('lb-paris');
  expect(yaml).toContain('load_balancer');
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

  // 6. Wait for the first iteration card to render with at least one
  //    stage pill (i.e., `iteration_1_generate` started). This is the
  //    moment the AI starts building — the viewer sees the timeline
  //    populate live.
  const iterationTimeline = page.locator('text=Iteration Timeline');
  await expect(iterationTimeline).toBeVisible({ timeout: 60_000 });
  await iterationTimeline.scrollIntoViewIfNeeded();
  await page.waitForTimeout(4_000);

  // 7. Hold while the remaining stages (validate, test) light up.
  //    The Live page polls the run-status API every couple of seconds;
  //    iteration card colour flips emerald-green as each stage passes.
  //    Total LLM + validation + mock-apply time for lb-paris is
  //    ~25–35s end-to-end, so wait until the success banner.
  await expect(page.getByText('Run succeeded')).toBeVisible({ timeout: 180_000 });

  // 8. Hold on the success state so the viewer sees the full
  //    iteration breakdown (generate / validate / test all green).
  await page.waitForTimeout(5_000);

  // 9. Navigate to the Runs page — viewer sees the freshly-completed
  //    run at the top of the history list with its terminal status.
  await page.locator('aside a[href="/runs"]').click();
  await expect(page.locator('main h1')).toContainText('Run History', { timeout: 10_000 });
  await page.waitForTimeout(3_500);

  // 10. Click into the lb-paris run we just created — the per-run
  //     page renders the Snapshots picker + Files list + IaC Preview
  //     for the freshly-built stack. Viewer sees the four resources
  //     the AI converged on: scaleway_lb_ip + scaleway_lb +
  //     scaleway_lb_backend + scaleway_lb_frontend.
  const lbParisLink = page.locator('a[href^="/runs/lb-paris/"]').first();
  if (await lbParisLink.isVisible().catch(() => false)) {
    await lbParisLink.click();
    // Per-run page heading is "Run <runID>".
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

  // 12. Close on the home page so the recording ends on a familiar
  //     view.
  await page.locator('aside a[href="/"]').click();
  await expect(page.locator('h1')).toContainText('InfraFactory Dashboard');
  await page.waitForTimeout(2_000);
});
