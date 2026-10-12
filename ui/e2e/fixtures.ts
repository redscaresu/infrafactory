import { test as base } from '@playwright/test';
import type { ScenarioRunModeResponse } from '../src/lib/types';

// Every spec imports `test` from here rather than from @playwright/test.
//
// The suite's server runs with no mock behind it, so the real run-mode
// handler answers every scenario page with a 500 and logs "could not
// determine the run mode". That is correct for a real server; here it
// was noise in the log and an error state under the visual baselines.
// So every page answers run-mode itself. A spec that needs another answer
// routes it again: the route registered last wins. A spec route that
// matches run-mode and lets the request through calls route.fallback(),
// not route.continue(), which skips this route and reaches the server.
export * from '@playwright/test';

export const RUN_MODE_ROUTE = /\/api\/scenarios\/.+\/run-mode$/;

export const RUN_MODE: ScenarioRunModeResponse = {
  name: 'fixture',
  path: 'training/fixture',
  cloud: 'scaleway',
  mock_provider: 'mockway',
  mode: 'clean',
  reason: 'missing mockway state, terraform.tfstate, previous successful run',
  has_mock_resources: false,
  has_tfstate: false,
  has_previous_successful_run: false,
};

export const test = base.extend({
  page: async ({ page }, use) => {
    await page.route(RUN_MODE_ROUTE, (route) => route.fulfill({ json: RUN_MODE }));
    await use(page);
  },
});
