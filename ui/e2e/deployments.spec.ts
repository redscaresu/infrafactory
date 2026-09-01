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
      body: JSON.stringify({ schema: 'infrafactory.api.deployments.v1', ...payload })
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

  test('the page is reachable from the sidebar', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('link', { name: 'Deployments' }).click();
    await expect(page).toHaveURL(/\/deployments/);
  });
});
