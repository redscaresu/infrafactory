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

    // The confirmation closing is what marks the POST as sent. The
    // OUTCOME deliberately does not appear here: outcomes are keyed by
    // the scenario deployed, and this fixture makes that differ from the
    // page's scenario to prove the POST follows the preview. A deploy of
    // one scenario must not report on another's page.
    await expect(page.getByTestId('deploy-confirm')).toHaveCount(0);
    await expect.poll(() => posted).toBe('the-previewed-one');
    await expect(page.getByTestId('deploy-outcome')).toHaveCount(0);
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
    // Same scenario as the page, which is the only shape the real flow
    // produces: the preview is fetched for the scenario on screen.
    await servePreview(page, { scenario: 'web-app-paris' });
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

    await expect(page.getByTestId('deploy-outcome')).toContainText('web-app-paris');
  });

  test('a failed outcome names its scenario too', async ({ page }) => {
    await servePreview(page, { scenario: 'web-app-paris' });
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
    await expect(outcome).toContainText('web-app-paris');
    await expect(outcome).toContainText('7c98d82e');
  });
});

test.describe('Deploy progress', () => {
  // Minutes of silence reads as broken. The reader cannot tell a long
  // apply from a hung one, and the difference matters when the thing
  // running is creating billable infrastructure.
  test('a deploy in flight shows something is happening', async ({ page }) => {
    await page.route('**/api/deployments/preview**', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          scenario: 'web-app-paris',
          deployable: true,
          expires_at: null,
          internet_facing: false,
          deploy_allowed: true,
          cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
        })
      })
    );

    let release: () => void = () => {};
    const held = new Promise<void>((resolve) => (release = resolve));
    await page.route('**/api/deployments', async (route) => {
      if (route.request().method() !== 'POST') return route.continue();
      await held;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ clean: true, steps: [], failures: [] })
      });
    });

    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();
    await page.getByTestId('deploy-confirm-go').click();

    // Before any line arrives, the page still says it is working.
    await expect(page.getByTestId('deploy-progress')).toContainText('Starting…');

    release();
    await expect(page.getByTestId('deploy-outcome')).toBeVisible();
  });

  // SvelteKit REUSES the [...path] component across scenario routes, so
  // leaving one scenario page for another destroys nothing and
  // `onDestroy` never fires. Without resetting from the navigation path
  // too, a previous scenario's progress keeps rendering under the new
  // one.
  test('progress does not follow you to another scenario', async ({ page }) => {
    await servePreview(page);

    let release: () => void = () => {};
    const held = new Promise<void>((resolve) => (release = resolve));
    await page.route('**/api/deployments', async (route) => {
      if (route.request().method() !== 'POST') return route.continue();
      await held;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ clean: true, steps: [], failures: [] })
      });
    });

    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();
    await page.getByTestId('deploy-confirm-go').click();
    await expect(page.getByTestId('deploy-progress')).toBeVisible();

    // Client-side navigation, which reuses the component.
    await page.getByTestId('sidebar-scenario-training/lb-serving-paris').click();
    await expect(page.locator('main h1')).toContainText('lb-serving-paris');

    await expect(page.getByTestId('deploy-progress')).toHaveCount(0);

    release();
    // And the finished deploy must not paint its result here either.
    await expect(page.getByTestId('deploy-progress')).toHaveCount(0);
  });
});

test.describe('Deploy state outlives the page', () => {
  // Component state died with the component. Navigating away and back
  // during a deploy left a real, billable apply rendered as an
  // unlabelled disabled button with no log and no warning; leaving the
  // section entirely let a SECOND deploy of the same scenario start,
  // because the server has no in-flight lock.
  test('a running deploy is still shown after navigating away and back', async ({ page }) => {
    await page.route('**/api/deployments/preview**', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          scenario: 'web-app-paris',
          deployable: true,
          expires_at: null,
          internet_facing: false,
          deploy_allowed: true,
          cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
        })
      })
    );

    let release: () => void = () => {};
    const held = new Promise<void>((resolve) => (release = resolve));
    await page.route('**/api/deployments', async (route) => {
      if (route.request().method() !== 'POST') return route.continue();
      await held;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ clean: true, steps: [], failures: [] })
      });
    });

    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();
    await page.getByTestId('deploy-confirm-go').click();
    await expect(page.getByTestId('deploy-progress')).toBeVisible();

    // Away — the other scenario shows nothing about this deploy...
    await page.getByTestId('sidebar-scenario-training/lb-serving-paris').click();
    await expect(page.locator('main h1')).toContainText('lb-serving-paris');
    await expect(page.getByTestId('deploy-progress')).toHaveCount(0);
    await expect(page.getByTestId('scenario-deploy')).toBeEnabled();

    // ...and back — the running deploy is visible again, and cannot be
    // started a second time.
    await page.getByTestId('sidebar-scenario-training/web-app-paris').click();
    await expect(page.locator('main h1')).toContainText('web-app-paris');
    await expect(page.getByTestId('deploy-progress')).toBeVisible();
    await expect(page.getByTestId('scenario-deploy')).toBeDisabled();

    release();
    await expect(page.getByTestId('deploy-outcome')).toBeVisible();
  });

  // Leaving the section unmounts the component entirely, which is what
  // made the second deploy reachable.
  test('a running deploy survives leaving the scenarios section', async ({ page }) => {
    await page.route('**/api/deployments/preview**', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          scenario: 'web-app-paris',
          deployable: true,
          expires_at: null,
          internet_facing: false,
          deploy_allowed: true,
          cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
        })
      })
    );

    let posts = 0;
    let release: () => void = () => {};
    const held = new Promise<void>((resolve) => (release = resolve));
    await page.route('**/api/deployments', async (route) => {
      if (route.request().method() !== 'POST') return route.continue();
      posts += 1;
      await held;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ clean: true, steps: [], failures: [] })
      });
    });

    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();
    await page.getByTestId('deploy-confirm-go').click();
    await expect(page.getByTestId('deploy-progress')).toBeVisible();

    await page.getByRole('link', { name: 'Deployments' }).click();
    await expect(page).toHaveURL(/\/deployments/);

    await page.getByTestId('sidebar-scenario-training/web-app-paris').click();
    await expect(page.locator('main h1')).toContainText('web-app-paris');

    await expect(page.getByTestId('scenario-deploy')).toBeDisabled();
    expect(posts).toBe(1);

    release();
  });
});

// Progress that is actually RENDERED.
//
// Every earlier test in this file intercepts the POST in the browser, so
// the Go server never broadcasts and no progress event is ever produced.
// A review demonstrated the cost: replacing the whole `{#each}` block
// with a constant string, and separately hiding the disconnected banner
// entirely, both passed the full suite. Behaviours 1, 2 and 3 were
// invisible in the browser.
//
// `routeWebSocket` lets the test be the server for the socket, so real
// frames reach the real filter and the real DOM.
test.describe('Deploy progress in the DOM', () => {
  const preview = {
    scenario: 'web-app-paris',
    deployable: true,
    expires_at: null,
    internet_facing: false,
    deploy_allowed: true,
    cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
  };

  async function startDeploy(page, { holdPost = true } = {}) {
    await page.route('**/api/deployments/preview**', (route) =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(preview) })
    );

    let release: () => void = () => {};
    const held = new Promise<void>((resolve) => (release = resolve));
    await page.route('**/api/deployments', async (route) => {
      if (route.request().method() !== 'POST') return route.continue();
      if (holdPost) await held;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ clean: true, steps: [], failures: [] })
      });
    });

    await page.goto('/scenarios/training/web-app-paris');
    await page.getByTestId('scenario-deploy').click();
    await page.getByTestId('deploy-confirm-go').click();
    return release;
  }

  test('stage lines are rendered as they arrive', async ({ page }) => {
    let send: (data: string) => void = () => {};
    await page.routeWebSocket('**/api/ws', (ws) => {
      send = (data) => ws.send(data);
    });

    const release = await startDeploy(page);
    await expect(page.getByTestId('deploy-progress')).toBeVisible();

    const line = (text: string) =>
      JSON.stringify({ type: 'deploy_progress', data: { subject: 'web-app-paris', line: text } });

    send(line('init: running'));
    await expect(page.getByTestId('deploy-progress')).toContainText('init: running');

    send(line('apply: FAILED after 3s: transient provider error'));
    await expect(page.getByTestId('deploy-progress')).toContainText('apply: FAILED');
    await expect(page.getByTestId('deploy-progress')).toContainText('transient provider error');

    // Each line is its own row, not a single blob.
    await expect(page.getByTestId('deploy-progress').locator('p')).toHaveCount(2);

    release();
  });

  test('a line for another scenario is not rendered here', async ({ page }) => {
    let send: (data: string) => void = () => {};
    await page.routeWebSocket('**/api/ws', (ws) => {
      send = (data) => ws.send(data);
    });

    const release = await startDeploy(page);
    await expect(page.getByTestId('deploy-progress')).toBeVisible();

    send(
      JSON.stringify({
        type: 'deploy_progress',
        data: { subject: 'lb-serving-paris', line: 'belongs elsewhere' }
      })
    );
    send(
      JSON.stringify({
        type: 'deploy_progress',
        data: { subject: 'web-app-paris', line: 'init: running' }
      })
    );

    await expect(page.getByTestId('deploy-progress')).toContainText('init: running');
    await expect(page.getByTestId('deploy-progress')).not.toContainText('belongs elsewhere');

    release();
  });

  // A dropped socket and "nothing has happened yet" both produce an
  // empty log. Rendering them the same way tells the reader an apply is
  // quiet when the truth is that it is UNOBSERVED.
  test('a socket that never connects says so rather than "Starting"', async ({ page }) => {
    // Accept and immediately close, so onStatus(false) fires and no
    // reconnect ever succeeds either.
    await page.routeWebSocket('**/api/ws', (ws) => ws.close());

    const release = await startDeploy(page);

    await expect(page.getByTestId('deploy-progress-disconnected')).toBeVisible();
    await expect(page.getByTestId('deploy-progress')).not.toContainText('Starting…');

    release();
  });
});

// Deploy progress is broadcast to every client. The Live Run page
// renders whatever arrives and refetches run state per message, so
// without a filter a deploy in one tab fills an unrelated run's log with
// raw JSON and hammers the API.
test('deploy progress does not leak into the Live Run page', async ({ page }) => {
  let send: (data: string) => void = () => {};
  // The socket connects asynchronously, so the test must wait for it
  // rather than sending into a handler that has not run yet -- otherwise
  // it would pass by delivering nothing at all.
  let connected: () => void = () => {};
  const socketReady = new Promise<void>((resolve) => (connected = resolve));
  await page.routeWebSocket('**/api/ws', (ws) => {
    send = (data) => ws.send(data);
    connected();
  });

  await page.goto('/live');
  await socketReady;

  send(
    JSON.stringify({
      type: 'deploy_progress',
      data: { subject: 'web-app-paris', line: 'apply: running' }
    })
  );
  send(JSON.stringify({ type: 'log', data: 'a genuine run log line' }));

  await expect(page.locator('body')).toContainText('a genuine run log line');
  await expect(page.locator('body')).not.toContainText('deploy_progress');
});

test.describe('A reload cannot start a second deploy', () => {
  // The page was the only guard, and a page is exactly the wrong place
  // for one: a refresh wipes it, a second tab never had it, and a curl
  // never consulted it.
  test('a reloaded page shows the deploy the server says is running', async ({ page }) => {
    await page.route('**/api/deployments', (route) => {
      if (route.request().method() !== 'GET') return route.continue();
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          schema: 'infrafactory.api.deployments.v1',
          deployments: [],
          unreadable: [],
          teardown_allowed: false,
          deploy_allowed: true,
          deploying: ['web-app-paris']
        })
      });
    });
    await page.route('**/api/deployments/preview**', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          scenario: 'web-app-paris',
          deployable: true,
          expires_at: null,
          internet_facing: false,
          deploy_allowed: true,
          cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
        })
      })
    );

    await page.goto('/scenarios/training/web-app-paris');

    // The button is not offered, because the server would refuse it.
    await expect(page.getByTestId('scenario-deploy')).toBeDisabled();
    await expect(page.getByTestId('deploy-progress')).toBeVisible();
  });

  // And a scenario the server is NOT deploying stays available.
  test('a reload does not disable deploy for an idle scenario', async ({ page }) => {
    await page.route('**/api/deployments', (route) => {
      if (route.request().method() !== 'GET') return route.continue();
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          schema: 'infrafactory.api.deployments.v1',
          deployments: [],
          unreadable: [],
          teardown_allowed: false,
          deploy_allowed: true,
          deploying: ['some-other-scenario']
        })
      });
    });

    await page.goto('/scenarios/training/web-app-paris');

    await expect(page.getByTestId('scenario-deploy')).toBeEnabled();
    await expect(page.getByTestId('deploy-progress')).toHaveCount(0);
  });
});

// A success banner that reappears on every later visit is a claim about
// something that may no longer exist -- the TTL may well have expired.
test('a finished deploy does not haunt the scenario page', async ({ page }) => {
  await page.route('**/api/deployments/preview**', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        scenario: 'web-app-paris',
        deployable: true,
        expires_at: null,
        internet_facing: false,
        deploy_allowed: true,
        cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
      })
    })
  );
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
  await expect(page.getByTestId('deploy-outcome')).toBeVisible();

  await page.getByTestId('sidebar-scenario-training/lb-serving-paris').click();
  await expect(page.locator('main h1')).toContainText('lb-serving-paris');

  await page.getByTestId('sidebar-scenario-training/web-app-paris').click();
  await expect(page.locator('main h1')).toContainText('web-app-paris');
  await expect(page.getByTestId('deploy-outcome')).toHaveCount(0);
});
