import { test as base } from '@playwright/test';
import type { ScenarioGroup, ScenarioRunModeResponse } from '../src/lib/types';

// Every spec imports `test` from here rather than from @playwright/test.
//
// The suite's server runs with no mock behind it, so the real run-mode
// handler answers every scenario page with a 500 and logs "could not
// determine the run mode". That is correct for a real server; here it
// was noise in the log and an error state under the visual baselines.
// So every page answers run-mode itself, as a clean run of the scenario
// it asked about. A spec that needs another answer routes it again: the
// route registered last wins. A spec route that matches run-mode and lets
// the request through calls route.fallback(), not route.continue(), which
// skips this route and reaches the server.
export * from '@playwright/test';

export const RUN_MODE_ROUTE = /\/api\/scenarios\/(.+)\/run-mode$/;

// Mirrors serverState.mockStateForCloud (internal/api/server.go): a GCP
// scenario reads fakegcp, every other cloud reads mockway.
const mockProviderFor = (cloud: string) => (cloud === 'gcp' ? 'fakegcp' : 'mockway');

export const test = base.extend({
  page: async ({ page, request }, use) => {
    // Every scenario's cloud, read from the list before the test starts. The
    // route below then answers without a request of its own, so nothing is
    // left in flight when a test ends mid-load.
    const list = await request.get('/api/scenarios');
    if (!list.ok()) throw new Error(`GET /api/scenarios: ${list.status()}`);
    const { groups } = (await list.json()) as { groups: ScenarioGroup[] };
    const scenarios = new Map(groups.flatMap((g) => g.scenarios).map((s) => [s.path, s] as const));

    await page.route(RUN_MODE_ROUTE, (route) => {
      const path = RUN_MODE_ROUTE.exec(new URL(route.request().url()).pathname)![1];
      const scenario = scenarios.get(path);
      if (!scenario) return route.fulfill({ status: 404, json: { error: 'scenario not found' } });
      const provider = mockProviderFor(scenario.cloud);
      const runMode: ScenarioRunModeResponse = {
        name: scenario.name,
        path,
        cloud: scenario.cloud,
        mock_provider: provider,
        mode: 'clean',
        reason: `missing ${provider} state, terraform.tfstate, previous successful run`,
        has_mock_resources: false,
        has_tfstate: false,
        has_previous_successful_run: false,
      };
      return route.fulfill({ json: runMode });
    });
    await use(page);
  },
});
