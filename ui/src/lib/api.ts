import { fetchPitfalls, fetchSavePitfalls } from "$lib/pitfalls-api.js";
import type {
  Pitfall,
  PitfallsResponse,
  SavePitfallsResponse,
  ScenarioLayer3StatusResponse,
  ScenarioRunModeResponse,
  ActionResult,
  DeployPreview,
  DeploymentsResponse,
  StartRunOptions
} from "$lib/types";

const base = "";

/**
 * DeployError says whether the apply STARTED, which is the only thing a
 * caller can safely conclude from a failed deploy request.
 *
 * The deploy is deliberately detached from the request that starts it
 * (`destructiveContext`: "a client disconnecting halfway would leave
 * resources with no completed record"). So a rejected promise has two
 * completely different meanings:
 *
 *   - The SERVER answered, and its answer was a refusal issued before
 *     anything ran -- 423 while another deploy holds the lock, 404 for
 *     an unknown scenario or a server without `--allow-deploy`, 400 for
 *     a malformed request. Nothing was created. `startedNothing`.
 *   - Anything else -- a sleeping laptop, a wifi hop, a proxy timeout,
 *     a 500 from a deploy that ran and failed. The apply may be running
 *     right now, creating a project and a bill.
 *
 * Collapsing the second into the first is how a page came to delete the
 * progress log of a live apply and tell the reader nothing had
 * happened.
 */
export class DeployError extends Error {
  readonly startedNothing: boolean;

  constructor(message: string, startedNothing: boolean) {
    super(message);
    this.name = "DeployError";
    this.startedNothing = startedNothing;
  }
}

// startedNothing reads the SERVER's word for it.
//
// `started_nothing: true` is written by `writeRefusal`, on the paths
// that reject a request before it can touch the cloud. Absence means
// unknown, which is the safe default: erring this way sends a reader to
// check the Deployments page for infrastructure that was never created,
// and erring the other way tells them nothing happened while a project
// is being created and billed.
//
// This replaced a client-side allowlist of status codes, which was a
// copy of server semantics -- the class this arc spent nine rounds
// deleting -- and the copy was already wrong: `deployHandler` answers
// 404 both for "no such scenario", before the apply, and for an
// os.ErrNotExist returned by Deploy, after it. The allowlist called
// both of them "nothing started".
function startedNothing(body: unknown): boolean {
  return typeof body === "object" && body !== null && (body as { started_nothing?: unknown }).started_nothing === true;
}

// isActionResult decides whether a body is a result or an error.
//
// The `clean` field is the discriminator, and checking it closes a
// class rather than an instance. The deploy path special-cased a bare
// 409 as an ActionResult because `writeActionResult` produces one --
// but so can a reverse proxy, an intermediary, or the next refusal
// somebody adds. A `{"error": ...}` body parsed as a result has no
// `clean` field, so it rendered "resources may still be running" for a
// request that never reached the deployer. That is the exact defect
// moving the "already deploying" refusal to 423 fixed for ONE producer;
// this fixes it for all of them.
function isActionResult(body: unknown): body is ActionResult {
  return typeof body === "object" && body !== null && "clean" in body;
}

/**
 * readJSON returns null instead of throwing on an unparseable body.
 *
 * A body that will not parse is not an ActionResult and carries no
 * error message, which is exactly what `null` says. Letting the
 * SyntaxError escape instead discarded the status the caller had
 * already used to decide whether anything had been started.
 */
async function readJSON(res: Response): Promise<unknown> {
  try {
    return await res.json();
  } catch {
    return null;
  }
}

function withFormat(path: string): string {
  const sep = path.includes("?") ? "&" : "?";
  return `${path}${sep}format=1`;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${base}${path}`, init);
  if (!res.ok) {
    const ctype = res.headers.get("content-type") || "";
    if (ctype.includes("application/json")) {
      const payload = (await res.json()) as { error?: string };
      throw new Error(payload.error || `request failed: ${res.status}`);
    }
    const text = await res.text();
    throw new Error(text || `request failed: ${res.status}`);
  }
  const ctype = res.headers.get("content-type") || "";
  if (ctype.includes("application/json")) {
    return (await res.json()) as T;
  }
  return (await res.text()) as T;
}

export const api = {
  getScenarios: () => request<{ groups: unknown[] }>("/api/scenarios"),
  getDiagnostics: () => request("/api/diagnostics"),
  getScenario: (path: string) => request(`/api/scenarios/${path}`),
  getScenarioRunMode: (path: string) => request<ScenarioRunModeResponse>(`/api/scenarios/${path}/run-mode`),
  getDeployments: () => request<DeploymentsResponse>("/api/deployments"),
  getDeployPreview: (scenario: string, ttl = "") =>
    request<DeployPreview>(
      `/api/deployments/preview?scenario=${encodeURIComponent(scenario)}` +
        (ttl ? `&ttl=${encodeURIComponent(ttl)}` : "")
    ),
  // Like teardown, this reads a 409 body rather than throwing it away: a
  // deploy that could not prove itself carries the per-stage failures,
  // and those name the leaked project id and how to remove it by hand.
  deployScenario: async (scenario: string, ttl = ""): Promise<ActionResult> => {
    const res = await fetch(`${base}/api/deployments`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(ttl ? { scenario, ttl } : { scenario })
    });
    const ctype = res.headers.get("content-type") || "";
    // 409 carries an ActionResult from a deploy that RAN and could not
    // prove itself clean. A refusal carries an error and must NOT be
    // parsed as one -- the body decides, not the status.
    if (ctype.includes("application/json")) {
      // Parsed defensively. A truncated body -- a proxy giving up, a
      // server killed mid-write -- makes `res.json()` throw a
      // SyntaxError, which is not a DeployError, so the caller loses
      // the startedNothing classification the STATUS had already
      // settled and shows a JavaScript parser message on the screen
      // this whole slice exists to make trustworthy.
      const body = await readJSON(res);
      if ((res.ok || res.status === 409) && isActionResult(body)) return body;
      throw new DeployError(
        (body as { error?: string })?.error || `deploy failed: ${res.status}`,
        startedNothing(body)
      );
    }
    // No JSON body at all, so no claim: unknown.
    throw new DeployError((await res.text()) || `deploy failed: ${res.status}`, false);
  },
  // Not `request`, deliberately. A teardown that could not prove the
  // account clean answers 409 WITH a full ActionResult -- the per-stage
  // failures are the whole point, and `request` would throw them away
  // and leave the page showing a generic message instead of "the state
  // file has vanished and the resources may still be running".
  tearDownDeployment: async (id: string): Promise<ActionResult> => {
    const res = await fetch(`${base}/api/deployments/${encodeURIComponent(id)}`, {
      method: "DELETE"
    });
    const ctype = res.headers.get("content-type") || "";
    // Same discriminator as deploy, for the same reason: a 409 that did
    // not come from `writeActionResult` has no `clean` field, and
    // parsing it as a result renders "resources may still be running"
    // over a request that never reached the store.
    if (ctype.includes("application/json")) {
      const body = await readJSON(res);
      if ((res.ok || res.status === 409) && isActionResult(body)) return body;
      throw new Error((body as { error?: string })?.error || `teardown failed: ${res.status}`);
    }
    throw new Error((await res.text()) || `teardown failed: ${res.status}`);
  },
  getScenarioLayer3Status: (path: string) => request<ScenarioLayer3StatusResponse>(`/api/scenarios/${path}/layer3-status`),
  putScenario: (path: string, rawYAML: string) =>
    request(`/api/scenarios/${path}`, {
      method: "PUT",
      headers: { "Content-Type": "application/x-yaml" },
      body: rawYAML
    }),
  getRuns: () => request<{ runs: unknown[] }>("/api/runs"),
  getRunsForScenario: (scenario: string) => request<{ runs: unknown[] }>(`/api/runs/${scenario}`),
  getRun: (scenario: string, runID: string) => request(`/api/runs/${scenario}/${runID}`),
  getRunLog: (scenario: string, runID: string) => request<string>(`/api/runs/${scenario}/${runID}/log`),
  getRunPlan: (scenario: string, runID: string) => request<string>(`/api/runs/${scenario}/${runID}/plan`),
  getRunBaseline: (scenario: string, runID: string) => request<string>(`/api/runs/${scenario}/${runID}/baseline`),
  getIterations: (scenario: string, runID: string) => request<{ iterations: number[] }>(`/api/runs/${scenario}/${runID}/iterations`),
  getRunFiles: (scenario: string, runID: string) => request<{ files: string[] }>(`/api/runs/${scenario}/${runID}/files`),
  getRunFile: (scenario: string, runID: string, file: string) => request<string>(withFormat(`/api/runs/${scenario}/${runID}/files/${file}`)),
  getIterationFiles: (scenario: string, runID: string, iteration: number) =>
    request<{ files: string[] }>(`/api/runs/${scenario}/${runID}/iterations/${iteration}/files`),
  getIterationFile: (scenario: string, runID: string, iteration: number, file: string) =>
    request<string>(withFormat(`/api/runs/${scenario}/${runID}/iterations/${iteration}/files/${file}`)),
  getIteration: (scenario: string, runID: string, iteration: number) =>
    request(`/api/runs/${scenario}/${runID}/iterations/${iteration}`),
  startRun: (scenario: string, options: StartRunOptions = {}) =>
    request<{ run_id: string }>(`/api/runs/${scenario}/start`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(options)
    }),
  compareRuns: (scenario: string, run1: string, run2: string) =>
    request<{ run1: string; run2: string; diffs: { filename: string; status: string; unified_diff?: string }[] }>(
      `/api/runs/${scenario}/compare?run1=${encodeURIComponent(run1)}&run2=${encodeURIComponent(run2)}`
    ),
  validateScenarioYAML: (yaml: string) =>
    request<{ valid: boolean; errors: { path: string; message: string }[] }>("/api/scenarios/validate", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ yaml })
    }),
  getOutputFiles: (scenario: string) => request<{ files: string[] }>(`/api/output/${scenario}`),
  getOutputFile: (scenario: string, file: string) => request<string>(withFormat(`/api/output/${scenario}/${file}`)),
  getConfig: () => request("/api/config"),
  getPitfalls: (): Promise<PitfallsResponse> => fetchPitfalls() as Promise<PitfallsResponse>,
  savePitfalls: (provider: string, pitfalls: Pitfall[]): Promise<SavePitfallsResponse> =>
    fetchSavePitfalls(provider, pitfalls) as Promise<SavePitfallsResponse>
};
