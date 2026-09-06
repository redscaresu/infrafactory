import { test, expect } from '@playwright/test';

// S164: the journey, end to end — deploy from the scenario page, find it
// on the estate page, tear it down there, and see the scenario page stop
// warning about it.
//
// # Why this file exists when there are already 78 deploy/estate tests
//
// Every one of them mocks a single endpoint with a FIXED body, so each
// page is tested against a world that agrees with it by construction.
// Nothing tests the HANDOFF. The deploy outcome tells the reader to
// "check the Deployments page before starting another"; no test has ever
// checked that what they find there is the deploy they just made, or
// that tearing it down there is visible back on the scenario page.
//
// That handoff is where the arc's claims actually live. `already_live`
// exists to stop a second bill, and it is computed on the server from
// the estate — so a fixture that hardcodes it is asserting the client
// renders a constant, not that the guard works.
//
// # The fake is STATEFUL, and that is the whole point
//
// One `estate` array behind all four routes. A deploy appends to it, a
// teardown removes from it, and both the estate page and the preview
// read it. A fixed-body mock cannot fail the way the real system fails:
// it agrees with itself no matter what the client does.

type Record = {
  id: string;
  scenario: string;
  state: string;
  address: string;
  unreadable: boolean;
  expired: boolean;
  upgraded: boolean;
  upgraded_at: string | null;
  upgrade_started_at: string | null;
  time_to_live_seconds: number;
  health: { status: string; version: string; at: string | null; observations: number };
};

function recordFor(id: string, scenario: string): Record {
  return {
    id,
    scenario,
    state: 'live',
    address: '51.15.0.9',
    unreadable: false,
    expired: false,
    upgraded: false,
    upgraded_at: null,
    upgrade_started_at: null,
    time_to_live_seconds: 3600,
    health: { status: 'healthy', version: 'confirmed', at: '2026-09-06T10:00:00Z', observations: 3 }
  };
}

/**
 * serveEstate wires all four deployment routes to one mutable estate.
 *
 * Returns the array so a test can assert against the server's view
 * rather than only against the screen — a page that renders nothing and
 * a server that recorded nothing look identical from the DOM alone.
 */
async function serveEstate(page, scenario: string) {
  const estate: Record[] = [];
  let nextID = 1;

  // ONE route, dispatching internally on path and method.
  //
  // Three overlapping patterns do not work here. Playwright matches in
  // reverse registration order, so `**/api/deployments/*` was consulted
  // for `/api/deployments/preview` -- and its non-DELETE `route.continue()`
  // sends the request to the NETWORK rather than to the next handler,
  // reaching the real server, which has no deployer and answers
  // `deploy_allowed: false`. The confirm button was then correctly
  // disabled, and the test looked like a UI bug. `route.fallback()`
  // would chain, but one dispatcher removes the ordering question
  // instead of answering it.
  await page.route('**/api/deployments**', (route) => {
    const url = new URL(route.request().url());
    const method = route.request().method();
    const json = (body: unknown, status = 200) =>
      route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });

    if (url.pathname.endsWith('/preview')) {
      const asked = url.searchParams.get('scenario') || '';
      return json({
        scenario: asked,
        deployable: true,
        expires_at: null,
        internet_facing: false,
        deploy_allowed: true,
        // Computed FROM THE ESTATE, like the server does. Hardcoding it
        // is what makes the existing tests unable to catch a guard that
        // stopped guarding.
        already_live: estate.filter((d) => d.scenario === asked).map((d) => d.id),
        already_live_unknown: false,
        already_deploying: false,
        cost: { components: [], eur_per_hour: 0.02, unpriced: [], complete: true, modelled: true }
      });
    }

    if (method === 'DELETE') {
      const id = decodeURIComponent(url.pathname.split('/').pop() || '');
      const at = estate.findIndex((d) => d.id === id);
      if (at >= 0) estate.splice(at, 1);
      return json({ clean: true, steps: [], failures: [], deployment: id });
    }

    if (method === 'POST') {
      const body = JSON.parse(route.request().postData() || '{}');
      const id = `dep-journey-${nextID++}`;
      estate.push(recordFor(id, body.scenario));
      return json({ clean: true, steps: [], failures: [], deployment: id });
    }

    return json({
      schema: 'infrafactory.api.deployments.v1',
      teardown_allowed: true,
      deploying: [],
      deployments: estate,
      unreadable: []
    });
  });

  return estate;
}

const SCENARIO = 'web-app-paris';
const PATH = `/scenarios/training/${SCENARIO}`;

test('a deploy made on the scenario page is findable on the estate page', async ({ page }) => {
  const estate = await serveEstate(page, SCENARIO);

  await page.goto(PATH);
  await page.getByTestId('scenario-deploy').click();
  await page.getByTestId('deploy-confirm-go').click();
  await expect(page.getByTestId('deploy-outcome')).toContainText('Deployed');

  // The server actually recorded one. Asserted separately from the
  // screen, because a page that renders nothing and a server that
  // recorded nothing are indistinguishable in the DOM.
  expect(estate).toHaveLength(1);
  const id = estate[0].id;

  // ...and the reader following the outcome's own advice finds THAT
  // deployment, by id, rather than merely a non-empty table.
  await page.goto('/deployments');
  await expect(page.getByTestId(`deployment-health-${id}`)).toBeVisible();
  await expect(page.locator('body')).toContainText(SCENARIO);
});

// The guard that exists to stop a second bill, tested against a live
// estate rather than a constant.
test('a second deploy is warned about using the estate the first one created', async ({ page }) => {
  const estate = await serveEstate(page, SCENARIO);

  await page.goto(PATH);
  await page.getByTestId('scenario-deploy').click();
  // Nothing is live yet, so no already-deployed warning.
  await expect(page.getByTestId('deploy-warning')).toHaveCount(0);
  await page.getByTestId('deploy-confirm-go').click();
  await expect(page.getByTestId('deploy-outcome')).toContainText('Deployed');

  const id = estate[0].id;

  // Reopening the confirmation re-reads the preview, which now sees the
  // record the first deploy created.
  await page.getByTestId('scenario-deploy').click();
  const warning = page.getByTestId('deploy-warning').first();
  await expect(warning).toContainText(id);
  await expect(warning).toContainText('SECOND project');
});

test('tearing down on the estate page stops the scenario page warning about it', async ({
  page
}) => {
  const estate = await serveEstate(page, SCENARIO);

  await page.goto(PATH);
  await page.getByTestId('scenario-deploy').click();
  await page.getByTestId('deploy-confirm-go').click();
  await expect(page.getByTestId('deploy-outcome')).toContainText('Deployed');
  const id = estate[0].id;

  await page.goto('/deployments');
  await page.getByTestId(`deployment-teardown-${id}`).click();
  await page.getByTestId(`deployment-destroy-${id}`).click();

  await expect(page.getByTestId(`deployment-health-${id}`)).toHaveCount(0);
  expect(estate).toHaveLength(0);

  // The round trip closes here. The warning was driven by the estate, so
  // removing the record must remove the warning -- and a fixed-body
  // preview mock would keep asserting it forever.
  await page.goto(PATH);
  await page.getByTestId('scenario-deploy').click();
  await expect(page.getByTestId('deploy-warning')).toHaveCount(0);
});
