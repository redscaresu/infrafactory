import { test, expect } from '@playwright/test';

// The estate page (S161). Every test here is about one property: SILENCE
// MUST NOT LOOK LIKE HEALTH. A blank cell is the most natural way in the
// world to render "we do not know", and it reads as "fine".

const HEALTHY = {
  id: 'dep-healthy',
  scenario: 'web-live-paris',
  state: 'live',
  address: '51.15.0.1',
  unreadable: false,
  expired: false,
  upgraded: false,
  upgraded_at: null,
  upgrade_started_at: null,
  time_to_live_seconds: 7200,
  health: { status: 'healthy', version: 'confirmed', at: '2026-09-02T10:00:00Z', observations: 6 }
};

const NEVER_OBSERVED = {
  ...HEALTHY,
  id: 'dep-silent',
  address: '',
  time_to_live_seconds: 3600,
  health: { status: 'unobserved', version: 'unchecked', at: null, observations: 0 }
};

const DRIFTED = {
  ...HEALTHY,
  id: 'dep-drift',
  time_to_live_seconds: 1800,
  upgraded: true,
  upgraded_at: '2026-09-02T09:00:00Z',
  health: { status: 'healthy', version: 'unconfirmed', at: '2026-09-02T10:00:00Z', observations: 4 }
};

async function serveEstate(page, payload) {
  await page.route('**/api/deployments', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        schema: 'infrafactory.api.deployments.v1',
        teardown_allowed: false,
        // The server always sends this. An absent field means "we were
        // not told what is applying", which deliberately withholds the
        // page's only permitted emptiness claim -- so a fixture that
        // omitted it was testing that path by accident.
        deploying: [],
        ...payload
      })
    })
  );
}

test.describe('Deployments estate page', () => {
  test('a deployment nobody probed says so, on both axes', async ({ page }) => {
    await serveEstate(page, { deployments: [NEVER_OBSERVED], unreadable: [] });
    await page.goto('/deployments');

    await expect(page.getByTestId('deployment-health-dep-silent')).toHaveText('never observed');
    await expect(page.getByTestId('deployment-version-dep-silent')).toHaveText('version unchecked');
    await expect(page.getByTestId('deployment-observed-dep-silent')).toHaveText('never');
  });

  test('a never-observed deployment never shows a year-one date', async ({ page }) => {
    await serveEstate(page, { deployments: [NEVER_OBSERVED], unreadable: [] });
    await page.goto('/deployments');

    await expect(page.locator('body')).not.toContainText('0001');
    await expect(page.locator('body')).not.toContainText('1/1/1');
  });

  // The most dangerous state the system can be in: the service answers
  // perfectly while running something other than what the record claims.
  // Every other signal calls it healthy.
  test('a healthy service on an unconfirmed version is flagged, not quietly green', async ({
    page
  }) => {
    await serveEstate(page, { deployments: [DRIFTED], unreadable: [] });
    await page.goto('/deployments');

    await expect(page.getByTestId('deployment-health-dep-drift')).toHaveText('healthy');
    await expect(page.getByTestId('deployment-version-dep-drift')).toHaveText(
      'version NOT confirmed'
    );
    await expect(page.getByTestId('deployment-row-dep-drift')).toHaveAttribute(
      'data-attention',
      'true'
    );
  });

  test('a genuinely healthy deployment is not flagged', async ({ page }) => {
    await serveEstate(page, { deployments: [HEALTHY], unreadable: [] });
    await page.goto('/deployments');

    await expect(page.getByTestId('deployment-row-dep-healthy')).toHaveAttribute(
      'data-attention',
      'false'
    );
  });

  test('an upgraded deployment is distinguishable from one that never moved', async ({ page }) => {
    await serveEstate(page, { deployments: [DRIFTED, HEALTHY], unreadable: [] });
    await page.goto('/deployments');

    await expect(page.getByTestId('deployment-upgraded-dep-drift')).toBeVisible();
    await expect(page.getByTestId('deployment-upgraded-dep-healthy')).toHaveCount(0);
  });

  // A record that will not decode may describe running, billing
  // infrastructure. `live ls` exits non-zero for it; a page has to show it.
  test('records the store could not read are surfaced, not dropped', async ({ page }) => {
    await serveEstate(page, {
      deployments: [HEALTHY],
      unreadable: ['dep-broken.json: unexpected end of JSON input']
    });
    await page.goto('/deployments');

    const banner = page.getByTestId('estate-unreadable');
    await expect(banner).toBeVisible();
    await expect(banner).toContainText('dep-broken.json');
    await expect(banner).toContainText('still costing money');
  });

  // A page that cannot read the estate must not look like a page reading
  // an empty estate.
  test('a failed read says so rather than showing an empty table', async ({ page }) => {
    await page.route('**/api/deployments', (route) =>
      route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"boom"}' })
    );
    await page.goto('/deployments');

    await expect(page.getByTestId('estate-load-error')).toBeVisible();
    await expect(page.getByTestId('estate-load-error')).toContainText(
      'not evidence that nothing is running'
    );
    // The summary line directly above the banner is part of the same
    // claim. An earlier version of this test checked only the banner,
    // while the summary said "Nothing is deployed." right above it.
    await expect(page.getByTestId('estate-summary')).toHaveText(
      'The live estate could not be read. Whether anything is running is unknown.'
    );
  });

  test('the summary states what was examined, not only what is wrong', async ({ page }) => {
    await serveEstate(page, { deployments: [HEALTHY, DRIFTED], unreadable: [] });
    await page.goto('/deployments');

    await expect(page.getByTestId('estate-summary')).toHaveText(
      '2 deployments, 1 needing attention'
    );
  });

  test('an empty estate is stated plainly', async ({ page }) => {
    await serveEstate(page, { deployments: [], unreadable: [] });
    await page.goto('/deployments');

    await expect(page.getByTestId('estate-summary')).toHaveText('Nothing is deployed.');
  });

  // `live observe` probes address:port. A link that drops the port sends
  // the reader somewhere the system never checked.
  test('the address link carries the recorded port', async ({ page }) => {
    await serveEstate(page, {
      deployments: [{ ...HEALTHY, id: 'dep-8080', port: 8080 }],
      unreadable: []
    });
    await page.goto('/deployments');

    await expect(page.getByTestId('deployment-address-dep-8080')).toHaveAttribute(
      'href',
      'http://51.15.0.1:8080'
    );
  });

  // The class, closed on the PAGE rather than on any one branch.
  //
  // This page claims "nothing is running" in more than one place, and
  // three review findings in this slice were a variant of the same
  // defect: fixing the claim in front of me and leaving its neighbour.
  // Nothing anywhere may say the estate is empty unless it is known to be.
  for (const [name, payload] of [
    ['a failed read', null],
    ['unreadable records with no decoded deployments', {
      deployments: [],
      unreadable: ['dep-broken.json: unexpected end of JSON input']
    }]
  ] as const) {
    test(`no part of the page calls the estate empty during ${name}`, async ({ page }) => {
      if (payload === null) {
        await page.route('**/api/deployments', (route) =>
          route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"boom"}' })
        );
      } else {
        await serveEstate(page, payload);
      }
      await page.goto('/deployments');
      await expect(page.getByTestId('estate-summary')).toBeVisible();

      await expect(page.getByTestId('estate-empty')).toHaveCount(0);
      await expect(page.locator('body')).not.toContainText('Nothing is deployed.');
      await expect(page.locator('body')).not.toContainText('No live deployments.');
    });
  }

  // A page must not offer a button it knows will 404, and must say why
  // rather than silently omitting the capability.
  test('no teardown control when the server did not allow it', async ({ page }) => {
    await serveEstate(page, { deployments: [HEALTHY], unreadable: [] });
    await page.goto('/deployments');

    await expect(page.getByTestId('deployment-teardown-dep-healthy')).toHaveCount(0);
    await expect(page.getByTestId('estate-readonly-note')).toContainText('--allow-teardown');
  });

  // A click must not destroy anything on its own, and the confirmation
  // must NAME what is about to go -- "are you sure?" is a speed bump
  // people learn to click through.
  test('teardown needs a second, named confirmation', async ({ page }) => {
    let deleted = 0;
    await serveEstate(page, {
      deployments: [{ ...HEALTHY, project_id: 'proj-abc' }],
      unreadable: [],
      teardown_allowed: true
    });
    await page.route('**/api/deployments/dep-healthy', (route) => {
      deleted += 1;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ clean: true, steps: [], failures: [] })
      });
    });
    await page.goto('/deployments');

    await page.getByTestId('deployment-teardown-dep-healthy').click();
    const confirm = page.getByTestId('deployment-confirm-dep-healthy');
    await expect(confirm).toContainText('web-live-paris');
    await expect(confirm).toContainText('proj-abc');
    await expect(confirm).toContainText('cannot be undone');
    expect(deleted).toBe(0);

    await page.getByTestId('deployment-cancel-dep-healthy').click();
    await expect(page.getByTestId('deployment-confirm-dep-healthy')).toHaveCount(0);
    expect(deleted).toBe(0);
  });

  test('confirming destroys and reports a proven-clean teardown', async ({ page }) => {
    await serveEstate(page, {
      deployments: [HEALTHY],
      unreadable: [],
      teardown_allowed: true
    });
    await page.route('**/api/deployments/dep-healthy', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ clean: true, steps: [], failures: [] })
      })
    );
    await page.goto('/deployments');

    await page.getByTestId('deployment-teardown-dep-healthy').click();
    await page.getByTestId('deployment-destroy-dep-healthy').click();

    await expect(page.getByTestId('deployment-outcome-dep-healthy')).toContainText(
      'provably clean'
    );
  });

  // ADR-0024: a teardown that cannot PROVE the account clean must not
  // report success. The 409 carries the per-stage failures, and losing
  // them would leave a generic error where "resources may still be
  // running" belongs.
  test('a teardown that cannot prove clean is not shown as success', async ({ page }) => {
    await serveEstate(page, {
      deployments: [HEALTHY],
      unreadable: [],
      teardown_allowed: true
    });
    await page.route('**/api/deployments/dep-healthy', (route) =>
      route.fulfill({
        status: 409,
        contentType: 'application/json',
        body: JSON.stringify({
          clean: false,
          steps: [],
          failures: [
            { stage: 'teardown', status: 'fail', detail: 'state file has vanished' }
          ]
        })
      })
    );
    await page.goto('/deployments');

    await page.getByTestId('deployment-teardown-dep-healthy').click();
    await page.getByTestId('deployment-destroy-dep-healthy').click();

    const outcome = page.getByTestId('deployment-outcome-dep-healthy');
    await expect(outcome).toContainText('may still be running');
    await expect(outcome).toContainText('state file has vanished');
  });

  // The property S161 claimed a visual baseline would protect. It does
  // not: `maxDiffPixelRatio: 0.02` means a change confined to three
  // small badges on a full-page screenshot passes comfortably, and a
  // Deploy button added to another page proved it by not failing one.
  //
  // Asserted directly instead, which is both stricter and not subject to
  // font-rendering flake: the three states must be visually distinct
  // from each other, because a reader tells them apart by colour before
  // they read a word.
  test('healthy, failing and never-observed are visually distinct', async ({ page }) => {
    await serveEstate(page, {
      deployments: [
        HEALTHY,
        { ...HEALTHY, id: 'dep-bad', health: { status: 'unhealthy', version: 'confirmed', at: '2026-09-02T10:00:00Z', observations: 3 } },
        NEVER_OBSERVED
      ],
      unreadable: []
    });
    await page.goto('/deployments');

    const colourOf = async (id: string) =>
      page.getByTestId(`deployment-health-${id}`).evaluate(
        (el) => getComputedStyle(el).backgroundColor
      );

    const healthy = await colourOf('dep-healthy');
    const unhealthy = await colourOf('dep-bad');
    const unobserved = await colourOf('dep-silent');

    expect(new Set([healthy, unhealthy, unobserved]).size).toBe(3);
  });

  // And the version axis, where the dangerous state is the one that
  // looks fine on the health axis.
  test('an unconfirmed version is visually distinct from a confirmed one', async ({ page }) => {
    await serveEstate(page, {
      deployments: [
        HEALTHY,
        { ...HEALTHY, id: 'dep-drifted', health: { status: 'healthy', version: 'unconfirmed', at: '2026-09-02T10:00:00Z', observations: 3 } }
      ],
      unreadable: []
    });
    await page.goto('/deployments');

    const confirmed = await page
      .getByTestId('deployment-version-dep-healthy')
      .evaluate((el) => getComputedStyle(el).backgroundColor);
    const unconfirmed = await page
      .getByTestId('deployment-version-dep-drifted')
      .evaluate((el) => getComputedStyle(el).backgroundColor);

    expect(confirmed).not.toBe(unconfirmed);
  });

  // A deploy that is applying has no record yet -- registerDeployment
  // runs after the apply returns -- so it cannot appear in the table.
  // Without this the page meant to answer "what is running" is silent
  // about the thing most actively running.
  test('a deploy in progress is shown even though it has no record yet', async ({ page }) => {
    await serveEstate(page, { deployments: [], unreadable: [], deploying: ['web-app-paris'] });
    await page.goto('/deployments');

    // The COUNT belongs to the summary line; the banner names what is
    // applying and says why it is not in the table. Rendering the count
    // in both put the same sentence on screen twice.
    await expect(page.getByTestId('estate-summary')).toContainText('1 deploy in progress');

    const banner = page.getByTestId('estate-deploying');
    await expect(banner).toContainText('Applying now');
    await expect(banner).toContainText('web-app-paris');
    await expect(banner).toContainText('no record of its own yet');
  });

  // Redeploying is deliberately allowed, so the table can hold an
  // EARLIER deployment of the scenario that is applying. The banner
  // used to say it "does not appear below" -- an absence the reader
  // could see was untrue, on the page whose whole thesis is never
  // saying something false about the estate.
  test('the in-flight banner does not deny a row the reader can see', async ({ page }) => {
    await serveEstate(page, {
      deployments: [
        {
          id: 'dep-old',
          scenario: 'web-app-paris',
          state: 'live',
          project_id: 'p-1',
          health: { status: 'healthy', version: 'confirmed' },
          time_to_live_seconds: 3600
        }
      ],
      unreadable: [],
      deploying: ['web-app-paris']
    });
    await page.goto('/deployments');

    const banner = page.getByTestId('estate-deploying');
    await expect(banner).not.toContainText('does not appear below');
    await expect(banner).toContainText('earlier deployment');
  });

  test('nothing deploying shows no banner', async ({ page }) => {
    await serveEstate(page, { deployments: [], unreadable: [], deploying: [] });
    await page.goto('/deployments');

    await expect(page.getByTestId('estate-deploying')).toHaveCount(0);
  });

  test('the page is reachable from the sidebar', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('link', { name: 'Deployments' }).click();
    await expect(page).toHaveURL(/\/deployments/);
  });
});


// The server always sends `deploying`. One that predates the field, or a
// body trimmed by an intermediary, does not -- and reading that as
// "nothing is applying" would license "Nothing is deployed." on an
// estate that may be busy creating something.
test('an estate that did not say what is applying is not called empty', async ({ page }) => {
  await page.route('**/api/deployments', (route) =>
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        schema: 'infrafactory.api.deployments.v1',
        teardown_allowed: false,
        deployments: [],
        unreadable: []
      })
    })
  );
  await page.goto('/deployments');

  await expect(page.getByTestId('estate-summary')).not.toHaveText('Nothing is deployed.');
  await expect(page.getByTestId('estate-empty')).toHaveCount(0);
});
