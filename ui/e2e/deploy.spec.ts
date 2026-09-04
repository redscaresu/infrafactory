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
  already_live: [],
  already_live_unknown: false,
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

    const report = page.getByTestId('pending-deploy-report');
    await expect(report).toContainText('7c98d82e');
    await expect(report).not.toContainText('Deployed.');
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

    // A REPORT, so it renders in the layout rather than in this
    // page's outcome slot -- it has to survive leaving the page.
    const report = page.getByTestId('pending-deploy-report');
    await expect(report).toContainText('web-app-paris');
    await expect(report).toContainText('7c98d82e');
    await expect(page.getByTestId('deploy-outcome')).toHaveCount(0);
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
          already_live: [],
          already_live_unknown: false,
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
          already_live: [],
          already_live_unknown: false,
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
          already_live: [],
          already_live_unknown: false,
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

    await page.getByRole('navigation').getByRole('link', { name: 'Deployments' }).click();
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
    already_live: [],
    already_live_unknown: false,
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

test.describe('What a reloaded page knows', () => {
  // The page no longer mirrors server state. It knows about deploys IT
  // started, and after a reload it does not know — so it says so and
  // points at the thing that does.
  //
  // Mirroring produced 36 review findings across three rounds, none of
  // which were about applying to a real cloud rather than a mock. This
  // is less convenient and it cannot be wrong.
  //
  // The guard against a second deploy is entirely server-side, which is
  // why there is no test here for the page refusing one: a refresh
  // wipes a page-side guard, a second tab never had it, and a curl
  // never consulted it.
  test('a reloaded page says what it does and does not know', async ({ page }) => {
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
          already_live: ['dep-existing'],
          already_live_unknown: false,
          cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
        })
      })
    );

    await page.goto('/scenarios/training/web-app-paris');

    // It does not pretend to know, and it does not block the button on
    // a guess -- the server refuses if it must.
    await expect(page.getByTestId('deploy-scope-note')).toContainText('deploys started from it');
    await expect(page.getByTestId('deploy-progress')).toHaveCount(0);

    // And the thing that DOES know is named in the confirmation, from
    // the server, where the answer is correct.
    await page.getByTestId('scenario-deploy').click();
    await expect(page.getByTestId('deploy-warning').first()).toContainText('dep-existing');
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
        already_live: [],
        already_live_unknown: false,
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

// afterNavigate does not fire for a component being DESTROYED, so
// leaving the scenarios SECTION -- the case the store's own doc names --
// left the banner to reappear on the next visit.
test('a finished deploy does not survive leaving the scenarios section', async ({ page }) => {
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
        already_live: [],
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

  // Leave the SECTION, which destroys the component.
  await page.getByRole('navigation').getByRole('link', { name: 'Deployments' }).click();
  await expect(page).toHaveURL(/\/deployments/);

  await page.getByTestId('sidebar-scenario-training/web-app-paris').click();
  await expect(page.locator('main h1')).toContainText('web-app-paris');
  await expect(page.getByTestId('deploy-outcome')).toHaveCount(0);
});

// The client's 423 handling was a comment only: nothing tested that a
// refusal is thrown rather than parsed as an ActionResult. A revert to
// 409, or adding 423 to the special case, would restore "resources may
// still be running" on a request that touched nothing, with every suite
// green.
test('a refused deploy does not claim resources may be running', async ({ page }) => {
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
        already_live: [],
        already_live_unknown: false,
        cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
      })
    })
  );
  await page.route('**/api/deployments', (route) =>
    route.request().method() === 'POST'
      ? route.fulfill({
          status: 423,
          contentType: 'application/json',
          body: JSON.stringify({
            error: 'web-app-paris is already deploying; wait for it to finish or tear it down',
            started_nothing: true
          })
        })
      : route.continue()
  );

  await page.goto('/scenarios/training/web-app-paris');
  await page.getByTestId('scenario-deploy').click();
  await page.getByTestId('deploy-confirm-go').click();

  // A refused deploy started nothing, so nothing is shown as running,
  // and the message says only what the server said. It is recorded as
  // an OUTCOME like every other ending -- the store keys those by
  // scenario, which is what scopes it to this page.
  const outcome = page.getByTestId('deploy-outcome');
  await expect(outcome).toContainText('already deploying');
  await expect(outcome).not.toContainText('may still be running');
  await expect(page.getByTestId('deploy-progress')).toHaveCount(0);
});

// The server computed already_live and nothing read it: the guard the
// ADR described was documented, tested on the server, and absent from
// the screen it was for.
test('the confirmation warns about a deployment that already exists', async ({ page }) => {
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
        already_live: ['dep-existing'],
        already_live_unknown: false,
        cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
      })
    })
  );

  await page.goto('/scenarios/training/web-app-paris');
  await page.getByTestId('scenario-deploy').click();

  const warning = page.getByTestId('deploy-warning').first();
  await expect(warning).toContainText('dep-existing');
  await expect(warning).toContainText('SECOND project');
});

// The server detaches the apply from the request that starts it, so a
// rejected fetch does NOT mean nothing happened. A sleeping laptop, a
// wifi hop or a proxy timeout leaves the apply running and creating
// billable infrastructure.
//
// The page used to call forgetDeploy here — deleting the entry and every
// progress line collected so far — under a comment asserting "Nothing
// was started". It told the reader a project that was being created did
// not exist.
test('a deploy whose connection drops is not called a deploy that never ran', async ({ page }) => {
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
        already_live: [],
        already_live_unknown: false,
        cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
      })
    })
  );
  // Not a status code: the request never gets an answer at all, which
  // is what a dropped connection looks like to fetch.
  await page.route('**/api/deployments', (route) =>
    route.request().method() === 'POST' ? route.abort('failed') : route.continue()
  );

  await page.goto('/scenarios/training/web-app-paris');
  await page.getByTestId('scenario-deploy').click();
  await page.getByTestId('deploy-confirm-go').click();

  // It reports the truth: we lost track, and the apply may be running.
  // A report, so the layout carries it -- it must outlive this page.
  const outcome = page.getByTestId('pending-deploy-report');
  await expect(outcome).toContainText('may still be running');
  await expect(outcome).toContainText('Deployments page');

  // And it does NOT say the server refused. The entry and its log
  // survive, which is what an outcome rendering at all demonstrates:
  // the replaced code deleted both and showed a bare failure message.
  await expect(outcome).not.toContainText('already deploying');
});

// Three different questions — one is applying, some are already live,
// the estate could not be fully read — and all three can be true at
// once. An earlier version returned on the first and dropped the rest.
test('a scenario that is both live and applying warns about both', async ({ page }) => {
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
        already_live: ['dep-existing'],
        already_live_unknown: false,
        already_deploying: true,
        cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
      })
    })
  );

  await page.goto('/scenarios/training/web-app-paris');
  await page.getByTestId('scenario-deploy').click();

  // Separate warnings, so the page can render them separately -- and
  // the strongest LEADS. The early return dropped it; concatenating the
  // three into one paragraph then demoted it to the second sentence of
  // the first, which a test reading `.first()` cannot tell apart.
  const warnings = page.getByTestId('deploy-warning');
  await expect(warnings.first()).toContainText('dep-existing');
  await expect(warnings.first()).toContainText('SECOND project');
  await expect(warnings.first()).not.toContainText('being deployed right now');
  await expect(warnings.nth(1)).toContainText('being deployed right now');
});

// A refusal describes the attempt it came from. Left on screen, a retry
// that SUCCEEDED rendered "already deploying" and "deployed" together.
test('a refusal does not outlive the attempt that caused it', async ({ page }) => {
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
        already_live: [],
        already_live_unknown: false,
        cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
      })
    })
  );

  let attempt = 0;
  await page.route('**/api/deployments', (route) => {
    if (route.request().method() !== 'POST') return route.continue();
    attempt += 1;
    return attempt === 1
      ? route.fulfill({
          status: 423,
          contentType: 'application/json',
          body: JSON.stringify({
            error: 'web-app-paris is already deploying; wait for it to finish or tear it down',
            started_nothing: true
          })
        })
      : route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ clean: true, failures: [] })
        });
  });

  await page.goto('/scenarios/training/web-app-paris');
  await page.getByTestId('scenario-deploy').click();
  await page.getByTestId('deploy-confirm-go').click();
  await expect(page.getByTestId('deploy-outcome')).toContainText('already deploying');

  await page.getByTestId('scenario-deploy').click();
  await page.getByTestId('deploy-confirm-go').click();

  const outcome = page.getByTestId('deploy-outcome');
  await expect(outcome).toContainText('Deployed. It is listed');
  await expect(outcome).not.toContainText('already deploying');
});

// The two possible answers to one request used to be scoped two
// different ways: an outcome lived in the scenario-keyed store and
// survived navigation, while a refusal was a component variable. Once
// the refusal was guarded by a navigation token, one arriving after any
// navigation deleted the entry and reported nothing at all -- the button
// silently reverting to "Deploy…" as though the click had never landed.
test('a refusal that arrives after a detour is still reported', async ({ page }) => {
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
        already_live: [],
        already_live_unknown: false,
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
      status: 423,
      contentType: 'application/json',
      body: JSON.stringify({
        error: 'web-app-paris is already deploying; wait for it to finish or tear it down',
        started_nothing: true
      })
    });
  });

  await page.goto('/scenarios/training/web-app-paris');
  await page.getByTestId('scenario-deploy').click();
  await page.getByTestId('deploy-confirm-go').click();

  // Away and back while the POST is outstanding, so the response
  // belongs to an earlier navigation than the one on screen. Sidebar
  // clicks, not page.goto: a reload would destroy the store along with
  // the request, which is a different case entirely.
  await page.getByTestId('sidebar-scenario-training/lb-serving-paris').click();
  await expect(page.locator('main h1')).toContainText('lb-serving-paris');
  await page.getByTestId('sidebar-scenario-training/web-app-paris').click();
  await expect(page.locator('main h1')).toContainText('web-app-paris');
  release();

  await expect(page.getByTestId('deploy-outcome')).toContainText('already deploying');
});


// The two hooks that drop a finished banner run when the reader LEAVES.
// Neither can drop a deploy that was still running then and finished
// afterwards -- and that entry lived forever, greeting every later visit
// with "deployed. It is listed on the Deployments page until its TTL
// expires." for infrastructure whose TTL may have gone.
test('a deploy that finishes after you leave does not greet you on your return', async ({
  page
}) => {
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
        already_live: [],
        already_live_unknown: false,
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

  // Leave while it is STILL RUNNING, so neither leave-hook can drop it,
  // and only then let it finish.
  await page.getByTestId('sidebar-scenario-training/lb-serving-paris').click();
  await expect(page.locator('main h1')).toContainText('lb-serving-paris');
  release();

  await page.getByTestId('sidebar-scenario-training/web-app-paris').click();
  await expect(page.locator('main h1')).toContainText('web-app-paris');
  await expect(page.getByTestId('deploy-outcome')).toHaveCount(0);

  // And the button works, rather than being stuck behind a ghost.
  await expect(page.getByTestId('scenario-deploy')).toBeEnabled();
});

// The mirror image of the test above, and the more important half.
//
// A failure is not a stale claim, it is an UNREAD REPORT: "it may have
// created resources that are still running", carrying the project id
// somebody has to remove by hand. A deploy that fails before
// registration has no live record either, so this banner is the only
// place it is ever said -- and dropping it because the reader looked
// away is how the leak goes unnoticed.
test('a deploy that FAILS while you are away still reports when you return', async ({ page }) => {
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
        already_live: [],
        already_live_unknown: false,
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
      status: 409,
      contentType: 'application/json',
      body: JSON.stringify({
        clean: false,
        steps: [],
        failures: [{ detail: 'project 7c98d82e is live and could not be deleted' }]
      })
    });
  });

  await page.goto('/scenarios/training/web-app-paris');
  await page.getByTestId('scenario-deploy').click();
  await page.getByTestId('deploy-confirm-go').click();
  await expect(page.getByTestId('deploy-progress')).toBeVisible();

  await page.getByTestId('sidebar-scenario-training/lb-serving-paris').click();
  await expect(page.locator('main h1')).toContainText('lb-serving-paris');
  release();

  await page.getByTestId('sidebar-scenario-training/web-app-paris').click();
  await expect(page.locator('main h1')).toContainText('web-app-paris');

  const outcome = page.getByTestId('pending-deploy-report');
  await expect(outcome).toContainText('may have created resources that are still running');
  // The project id is the handle for removing it by hand.
  await expect(outcome).toContainText('7c98d82e');
});

// The failure message says "check the Deployments page before starting
// another", and the Deployments link sits directly beneath the button.
// Following that instruction used to DELETE the project id the
// instruction was about: leaving forgot any finished deploy, and a
// deploy that fails before registration has no live record either, so
// nothing on the estate page replaced it.
test('following the advice in a failure message does not destroy the failure message', async ({
  page
}) => {
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
        already_live: [],
        already_live_unknown: false,
        cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
      })
    })
  );
  await page.route('**/api/deployments', (route) =>
    route.request().method() === 'POST'
      ? route.fulfill({
          status: 409,
          contentType: 'application/json',
          body: JSON.stringify({
            clean: false,
            steps: [],
            failures: [{ detail: 'project 7c98d82e is live and could not be deleted' }]
          })
        })
      : route.continue()
  );

  await page.goto('/scenarios/training/web-app-paris');
  await page.getByTestId('scenario-deploy').click();
  await page.getByTestId('deploy-confirm-go').click();
  await expect(page.getByTestId('pending-deploy-report')).toContainText('7c98d82e');

  // Leave the scenarios section entirely, exactly as the message says.
  await page.getByTestId('sidebar-scenario-training/lb-serving-paris').click();
  await expect(page.locator('main h1')).toContainText('lb-serving-paris');

  await page.getByTestId('sidebar-scenario-training/web-app-paris').click();
  await expect(page.locator('main h1')).toContainText('web-app-paris');
  await expect(page.getByTestId('pending-deploy-report')).toContainText('7c98d82e');
});

// A successful one still goes, because it is a claim about
// infrastructure whose TTL may have expired rather than a report.
test('a successful deploy banner does not follow you back', async ({ page }) => {
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
        already_live: [],
        already_live_unknown: false,
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
  await expect(page.getByTestId('deploy-outcome')).toContainText('Deployed. It is listed');

  await page.getByTestId('sidebar-scenario-training/lb-serving-paris').click();
  await expect(page.locator('main h1')).toContainText('lb-serving-paris');
  await page.getByTestId('sidebar-scenario-training/web-app-paris').click();
  await expect(page.locator('main h1')).toContainText('web-app-paris');

  await expect(page.getByTestId('deploy-outcome')).toHaveCount(0);
});

// A refusal started NOTHING, which is what `startedNothing` proves. So
// it is not a report of infrastructure and must not outlive the visit
// the way a real failure does -- keeping it made a transient "already
// deploying" banner reappear on every later visit for the rest of the
// session, under an enabled Deploy button, long after the apply it
// referred to had finished.
test('a refusal does not haunt the page after the reader gives up', async ({ page }) => {
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
        already_live: [],
        already_live_unknown: false,
        cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
      })
    })
  );
  await page.route('**/api/deployments', (route) =>
    route.request().method() === 'POST'
      ? route.fulfill({
          status: 423,
          contentType: 'application/json',
          body: JSON.stringify({
            error: 'web-app-paris is already deploying; wait for it to finish or tear it down',
            started_nothing: true
          })
        })
      : route.continue()
  );

  await page.goto('/scenarios/training/web-app-paris');
  await page.getByTestId('scenario-deploy').click();
  await page.getByTestId('deploy-confirm-go').click();

  const outcome = page.getByTestId('deploy-outcome');
  await expect(outcome).toContainText('already deploying');
  // The server names the scenario deliberately, so the page must not
  // name it again: "web-app-paris: web-app-paris is already deploying".
  await expect(outcome).not.toContainText('web-app-paris: web-app-paris');

  await page.getByTestId('sidebar-scenario-training/lb-serving-paris').click();
  await expect(page.locator('main h1')).toContainText('lb-serving-paris');
  await page.getByTestId('sidebar-scenario-training/web-app-paris').click();
  await expect(page.locator('main h1')).toContainText('web-app-paris');

  await expect(page.getByTestId('deploy-outcome')).toHaveCount(0);
});

// The outcome banner lives inside `{#if detail}`, so a transient
// scenario-load failure hid it -- taking with it the project id somebody
// has to remove by hand.
test('a failed scenario load does not hide a leaked project', async ({ page }) => {
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
        already_live: [],
        already_live_unknown: false,
        cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
      })
    })
  );
  await page.route('**/api/deployments', (route) =>
    route.request().method() === 'POST'
      ? route.fulfill({
          status: 409,
          contentType: 'application/json',
          body: JSON.stringify({
            clean: false,
            steps: [],
            failures: [{ detail: 'project 7c98d82e is live and could not be deleted' }]
          })
        })
      : route.continue()
  );

  await page.goto('/scenarios/training/web-app-paris');
  await page.getByTestId('scenario-deploy').click();
  await page.getByTestId('deploy-confirm-go').click();
  await expect(page.getByTestId('pending-deploy-report')).toContainText('7c98d82e');

  // The next scenario read fails, so the page cannot render its usual
  // body -- and used to render nothing else either.
  await page.route('**/api/scenarios/training/lb-serving-paris', (route) =>
    route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"disk went away"}' })
  );
  await page.getByTestId('sidebar-scenario-training/lb-serving-paris').click();

  await expect(page.getByTestId('scenario-load-error')).toContainText('disk went away');
  await expect(page.getByTestId('pending-deploy-report')).toContainText('7c98d82e');
});

// The report is rendered in the LAYOUT, so it does not depend on which
// page the reader is on -- least of all on an unrelated fetch failing,
// which is how it was briefly reachable.
test('a leaked project stays named wherever the reader goes', async ({ page }) => {
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
        already_live: [],
        already_live_unknown: false,
        cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
      })
    })
  );
  await page.route('**/api/deployments', (route) =>
    route.request().method() === 'POST'
      ? route.fulfill({
          status: 409,
          contentType: 'application/json',
          body: JSON.stringify({
            clean: false,
            steps: [],
            failures: [{ detail: 'project 7c98d82e is live and could not be deleted' }]
          })
        })
      : route.continue()
  );

  await page.goto('/scenarios/training/web-app-paris');
  await page.getByTestId('scenario-deploy').click();
  await page.getByTestId('deploy-confirm-go').click();
  await expect(page.getByTestId('pending-deploy-report')).toContainText('7c98d82e');

  // Following the message's own advice.
  await page.getByTestId('sidebar-scenario-training/lb-serving-paris').click();
  await expect(page.locator('main h1')).toContainText('lb-serving-paris');
  await expect(page.getByTestId('pending-deploy-report')).toContainText('7c98d82e');
  await expect(page.getByTestId('pending-deploy-report')).toContainText('web-app-paris');
});

// Only the LOCK refusal names the scenario. "invalid json body",
// "method not allowed", the origin guard's message and the
// no---allow-deploy 404 do not — and the outcome slot is shared with
// every scenario the reader visits, so an unattributed refusal there is
// the attribution defect the prefix exists to prevent.
test('a refusal the server did not attribute is attributed here', async ({ page }) => {
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
        already_live: [],
        already_live_unknown: false,
        cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
      })
    })
  );
  await page.route('**/api/deployments', (route) =>
    route.request().method() === 'POST'
      ? route.fulfill({
          status: 403,
          contentType: 'application/json',
          body: JSON.stringify({
            error:
              'cross-origin request refused: this server is reachable only from a page served on loopback',
            started_nothing: true
          })
        })
      : route.continue()
  );

  await page.goto('/scenarios/training/web-app-paris');
  await page.getByTestId('scenario-deploy').click();
  await page.getByTestId('deploy-confirm-go').click();

  const outcome = page.getByTestId('deploy-outcome');
  await expect(outcome).toContainText('web-app-paris: cross-origin request refused');
  // And it is not a report of infrastructure: nothing was created.
  await expect(page.getByTestId('pending-deploy-report')).toHaveCount(0);
});

// Fail, retry, succeed, navigate — and the project the FIRST attempt
// leaked used to be gone, because the forget rule judged the entry by
// its last outcome and then deleted the whole entry, reports included.
test('a successful retry does not erase what the failed attempt leaked', async ({ page }) => {
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
        already_live: [],
        already_live_unknown: false,
        cost: { components: [], eur_per_hour: 0, unpriced: [], complete: true, modelled: true }
      })
    })
  );

  let attempt = 0;
  await page.route('**/api/deployments', (route) => {
    if (route.request().method() !== 'POST') return route.continue();
    attempt += 1;
    return attempt === 1
      ? route.fulfill({
          status: 409,
          contentType: 'application/json',
          body: JSON.stringify({
            clean: false,
            steps: [],
            failures: [{ detail: 'project 7c98d82e is live and could not be deleted' }]
          })
        })
      : route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ clean: true, steps: [], failures: [] })
        });
  });

  await page.goto('/scenarios/training/web-app-paris');
  await page.getByTestId('scenario-deploy').click();
  await page.getByTestId('deploy-confirm-go').click();
  await expect(page.getByTestId('pending-deploy-report')).toContainText('7c98d82e');

  // The obvious next action, and it works this time.
  await page.getByTestId('scenario-deploy').click();
  await page.getByTestId('deploy-confirm-go').click();
  await expect(page.getByTestId('deploy-outcome')).toContainText('Deployed. It is listed');

  // The second attempt succeeding does not un-leak the first one's
  // project, and navigating away must not be what deletes the record.
  await page.getByTestId('sidebar-scenario-training/lb-serving-paris').click();
  await expect(page.locator('main h1')).toContainText('lb-serving-paris');
  await expect(page.getByTestId('pending-deploy-report')).toContainText('7c98d82e');
});
