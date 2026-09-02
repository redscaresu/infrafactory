import { test, expect } from '@playwright/test';

// The deploy button (S162c). Every test is about the confirmation
// telling the truth before somebody creates infrastructure that
// persists, bills hourly, and serves the internet.

const PREVIEW = {
  scenario: 'web-app-paris',
  cloud: 'scaleway',
  deployable: true,
  image: 'nginx:1.27',
  ttl: '4h0m0s',
  expires_at: '2026-09-03T03:47:00Z',
  expires_at_wall_clock: 'Wed 3 Sep 03:47 UTC',
  cost_summary: 'about €0.04/hour at list price, €0.17 for 4h0m0s',
  internet_facing: true,
  deploy_allowed: true,
  cost: {
    components: [
      { name: 'DEV1-S instance', count: 1, eur_per_hour: 0.00898, priced: true },
      { name: 'public IPv4 address', count: 2, eur_per_hour: 0.005, priced: true }
    ],
    eur_per_hour: 0.042,
    unpriced: [],
    complete: true,
    modelled: true
  }
};

async function servePreview(page, over = {}) {
  await page.route('**/api/deployments/preview**', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ ...PREVIEW, ...over })
    })
  );
}

test.describe('Deploy from the scenario page', () => {
  test('deploy is a separate button from run', async ({ page }) => {
    await page.goto('/scenarios/training/web-app-paris');

    await expect(page.getByRole('button', { name: 'Run' })).toBeVisible();
    await expect(page.getByTestId('scenario-deploy')).toBeVisible();
    await expect(page.getByTestId('scenario-deploy')).not.toHaveText('Run');
  });

  // Cost, lifetime and blast radius before the click (ADR-0027 §2).
  test('the confirmation states shape, cost, expiry and exposure', async ({ page }) => {
    await servePreview(page);
    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();

    const confirm = page.getByTestId('deploy-confirm');
    await expect(confirm).toContainText('DEV1-S instance');
    await expect(confirm).toContainText('list price');
    await expect(confirm).toContainText('Wed 3 Sep 03:47');
    await expect(confirm).toContainText('nginx:1.27');
    await expect(page.getByTestId('deploy-warning').first()).toContainText('public internet');
  });

  // A click cannot create infrastructure on its own.
  test('nothing is created until the confirmation is accepted', async ({ page }) => {
    let posted = 0;
    await servePreview(page);
    await page.route('**/api/deployments', (route) => {
      if (route.request().method() === 'POST') posted += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ clean: true, steps: [], failures: [] })
      });
    });
    await page.goto('/scenarios/training/web-app-paris');

    await page.getByTestId('scenario-deploy').click();
    expect(posted).toBe(0);

    await page.getByTestId('deploy-cancel').click();
    await expect(page.getByTestId('deploy-confirm')).toHaveCount(0);
    expect(posted).toBe(0);
  });

  // The one that invalidates everything above it: an unmodelled
  // scenario's empty component list and €0.00 mean "unknown", not
  // "nothing".
  test('an unmodelled scenario says its figures are not complete', async ({ page }) => {
    await servePreview(page, {
      cost_summary:
        "this scenario's resources are not modelled here, so what it creates and what it costs are both unknown — do not read this as free",
      cost: { components: [], eur_per_hour: 0, unpriced: [], complete: false, modelled: false }
    });
    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();

    await expect(page.getByTestId('deploy-warning').first()).toContainText('not modelled');
    await expect(page.getByTestId('deploy-confirm')).toContainText('unknown');
  });

  test('an incomplete estimate is described as a floor, not a total', async ({ page }) => {
    await servePreview(page, {
      internet_facing: false,
      cost: {
        components: [{ name: 'Kubernetes cluster', count: 1, eur_per_hour: 0, priced: false }],
        eur_per_hour: 0.042,
        unpriced: ['Kubernetes cluster'],
        complete: false,
        modelled: true
      }
    });
    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();

    await expect(page.getByTestId('deploy-warning').first()).toContainText('floor, not a total');
  });

  // A page must not offer a button it knows will 404, and must say why.
  test('a server that cannot deploy says so instead of failing later', async ({ page }) => {
    await servePreview(page, { deploy_allowed: false });
    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();

    await expect(page.getByTestId('deploy-not-allowed')).toContainText('--allow-deploy');
    await expect(page.getByTestId('deploy-confirm-go')).toBeDisabled();
  });

  test('a scenario with nothing to deploy explains why', async ({ page }) => {
    await servePreview(page, {
      deployable: false,
      reason: 'this scenario declares no service: block, so there is nothing to deploy'
    });
    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();

    await expect(page.getByTestId('deploy-not-deployable')).toContainText('no service: block');
    await expect(page.getByTestId('deploy-confirm-go')).toBeDisabled();
  });

  // A deploy that could not prove itself is never rendered as success:
  // the 409 carries the leaked project id and how to remove it by hand.
  test('a deploy that did not succeed shows what went wrong', async ({ page }) => {
    await servePreview(page);
    await page.route('**/api/deployments', (route) =>
      route.request().method() === 'POST'
        ? route.fulfill({
            status: 409,
            contentType: 'application/json',
            body: JSON.stringify({
              clean: false,
              steps: [],
              failures: [
                {
                  stage: 'run_project',
                  status: 'fail',
                  detail: 'project 7c98d82e is live and could not be deleted'
                }
              ]
            })
          })
        : route.continue()
    );
    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();
    await page.getByTestId('deploy-confirm-go').click();

    const outcome = page.getByTestId('deploy-outcome');
    await expect(outcome).toContainText('7c98d82e');
    await expect(outcome).not.toContainText('Deployed.');
  });

  // A confirmation that describes one thing and does another is worse
  // than no confirmation: it converts a careful person into a confident
  // one. This component is reused across scenario routes, so an open
  // confirmation can outlive the page it was opened on.
  test('navigating away clears a confirmation from the previous scenario', async ({ page }) => {
    await servePreview(page);
    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();
    await expect(page.getByTestId('deploy-confirm')).toBeVisible();

    await page.goto('/scenarios/training/lb-serving-paris');

    await expect(page.getByTestId('deploy-confirm')).toHaveCount(0);
  });

  // And the deeper guarantee, independent of the reset firing: the POST
  // carries the scenario that was PREVIEWED, not whatever the page shows
  // now.
  test('the deploy posts the scenario the confirmation described', async ({ page }) => {
    let posted = '';
    await servePreview(page, { scenario: 'the-previewed-one' });
    await page.route('**/api/deployments', (route) => {
      if (route.request().method() === 'POST') {
        posted = JSON.parse(route.request().postData() || '{}').scenario;
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ clean: true, steps: [], failures: [] })
        });
      }
      return route.continue();
    });
    await page.goto('/scenarios/training/web-app-paris');

    await page.getByTestId('scenario-deploy').click();
    await page.getByTestId('deploy-confirm-go').click();

    await expect(page.getByTestId('deploy-outcome')).toBeVisible();
    expect(posted).toBe('the-previewed-one');
  });

  // afterNavigate clearing the dialog is not enough on its own: a
  // preview takes a round trip, and a response arriving after the route
  // changed would re-open a confirmation for a scenario the reader is no
  // longer looking at.
  test('a slow preview does not open a confirmation on the page you moved to', async ({ page }) => {
    let release: () => void = () => {};
    const held = new Promise<void>((resolve) => (release = resolve));

    await page.route('**/api/deployments/preview**', async (route) => {
      await held;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(PREVIEW)
      });
    });

    await page.goto('/scenarios/training/web-app-paris');
    const clicked = page.getByTestId('scenario-deploy').click();

    await page.goto('/scenarios/training/lb-serving-paris');
    release();
    await clicked.catch(() => {});

    await expect(page.getByTestId('deploy-confirm')).toHaveCount(0);
  });

  test('a successful deploy points at where it now lives', async ({ page }) => {
    await servePreview(page);
    await page.route('**/api/deployments', (route) =>
      route.request().method() === 'POST'
        ? route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({ clean: true, steps: [], failures: [] })
          })
        : route.continue()
    );
    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();
    await page.getByTestId('deploy-confirm-go').click();

    await expect(page.getByTestId('deploy-outcome')).toContainText('Deployments page');
  });

  // An apply takes minutes and the reader can navigate during it, so
  // this message can land on a different scenario's page. An
  // unattributed "Deployed." there is a claim about the wrong thing.
  //
  // Attributed rather than discarded: the deploy really did create
  // infrastructure, and throwing the news away because the reader moved
  // is the worse of the two failures.
  // Between the route changing and the new scenario loading, `detail`
  // used to still hold the PREVIOUS one -- so Deploy in that window
  // previewed and deployed the old scenario from a URL that said
  // otherwise. Clearing `detail` hides the whole page, button included,
  // until the new scenario is really there.
  test('deploy cannot act on the scenario the address bar no longer names', async ({ page }) => {
    await servePreview(page);
    let release: () => void = () => {};
    const held = new Promise<void>((resolve) => (release = resolve));
    await page.route('**/api/scenarios/training/lb-serving-paris**', async (route) => {
      await held;
      return route.continue();
    });

    await page.goto('/scenarios/training/web-app-paris');
    await expect(page.getByTestId('scenario-deploy')).toBeVisible();

    const navigating = page.goto('/scenarios/training/lb-serving-paris');
    // While the new scenario is still loading, there is nothing to click.
    await expect(page.getByTestId('scenario-deploy')).toHaveCount(0);

    release();
    await navigating;
    await expect(page.getByTestId('scenario-deploy')).toBeVisible();
  });

  // Route loads can resolve OUT OF ORDER, and the race lives in
  // CLIENT-SIDE routing, where the component is reused rather than
  // rebuilt. Comparing `scenarioPath` is not enough there: A → B → A
  // leaves the path equal to what an in-flight request for the first A
  // captured, so a genuinely stale response passes. A monotonic
  // navigation token cannot collide that way.
  test('a scenario load that resolves after navigation does not repopulate the page', async ({
    page
  }) => {
    await servePreview(page);

    let release: () => void = () => {};
    const held = new Promise<void>((resolve) => (release = resolve));
    await page.route('**/api/scenarios/training/web-app-paris**', async (route) => {
      await held;
      return route.continue();
    });

    // Settle on one scenario with a full load, then move CLIENT-SIDE.
    await page.goto('/scenarios/training/lb-serving-paris');
    await expect(page.locator('main h1')).toContainText('lb-serving-paris');

    await page.getByTestId('sidebar-scenario-training/web-app-paris').click();
    // Its detail request is held, so nothing is rendered yet.
    await expect(page.getByTestId('scenario-deploy')).toHaveCount(0);

    // Go back before it resolves, then let it through.
    await page.getByTestId('sidebar-scenario-training/lb-serving-paris').click();
    await expect(page.locator('main h1')).toContainText('lb-serving-paris');
    release();

    // The stale response must not overwrite the page it arrived on.
    await expect(page.locator('main h1')).toContainText('lb-serving-paris');
    await expect(page).toHaveURL(/lb-serving-paris/);
  });

  // Two clicks left two previews racing, and an older response could
  // reopen a dialog the reader had already dismissed. Only one may be in
  // flight.
  test('repeated clicks cannot reopen a dismissed confirmation', async ({ page }) => {
    let calls = 0;
    let release: () => void = () => {};
    const held = new Promise<void>((resolve) => (release = resolve));
    await page.route('**/api/deployments/preview**', async (route) => {
      calls += 1;
      await held;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(PREVIEW)
      });
    });

    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();
    await expect(page.getByTestId('scenario-deploy')).toBeDisabled();

    release();
    await expect(page.getByTestId('deploy-confirm')).toBeVisible();
    expect(calls).toBe(1);

    await page.getByTestId('deploy-cancel').click();
    await expect(page.getByTestId('deploy-confirm')).toHaveCount(0);
  });

  test('the outcome names the scenario it is about', async ({ page }) => {
    await servePreview(page, { scenario: 'the-deployed-one' });
    await page.route('**/api/deployments', (route) =>
      route.request().method() === 'POST'
        ? route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({ clean: true, steps: [], failures: [] })
          })
        : route.continue()
    );
    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();
    await page.getByTestId('deploy-confirm-go').click();

    await expect(page.getByTestId('deploy-outcome')).toContainText('the-deployed-one');
  });

  test('a failed outcome names its scenario too', async ({ page }) => {
    await servePreview(page, { scenario: 'the-failed-one' });
    await page.route('**/api/deployments', (route) =>
      route.request().method() === 'POST'
        ? route.fulfill({
            status: 409,
            contentType: 'application/json',
            body: JSON.stringify({
              clean: false,
              steps: [],
              failures: [{ stage: 'run_project', status: 'fail', detail: 'project 7c98d82e is live' }]
            })
          })
        : route.continue()
    );
    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();
    await page.getByTestId('deploy-confirm-go').click();

    const outcome = page.getByTestId('deploy-outcome');
    await expect(outcome).toContainText('the-failed-one');
    await expect(outcome).toContainText('7c98d82e');
  });
});
