import { fetchPitfalls, fetchSavePitfalls } from "$lib/pitfalls-api.js";
// Plain JS, so `node --test` can reach them. These four decide whether
// a reader is told "nothing was created" or "resources may still be
// running" -- the most consequential judgement in this slice -- and
// living in a .ts file put them beyond every unit test.
import { DeployError, isActionResult, readJSON, startedNothing } from "$lib/deploy-response.js";
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

function withFormat(path: string): string {
  const sep = path.includes("?") ? "&" : "?";
  return `${path}${sep}format=1`;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${base}${path}`, init);
  if (!res.ok) {
    const ctype = res.headers.get("content-type") || "";
    if (ctype.includes("application/json")) {
      // `readJSON`, not `res.json()`. A truncated body -- a proxy
      // giving up, a server killed mid-write -- throws a SyntaxError
      // that escapes as the error message, so a page renders
      // "Unexpected end of JSON input" where it means to render what
      // went wrong. The deploy path was hardened for this and left the
      // helper every other endpoint uses unguarded.
      const payload = (await readJSON(res)).value as { error?: string } | null;
      throw new Error(payload?.error || `request failed: ${res.status}`);
    }
    const text = await res.text();
    throw new Error(text || `request failed: ${res.status}`);
  }
  const ctype = res.headers.get("content-type") || "";
  if (ctype.includes("application/json")) {
    // Guarded on the SUCCESS path too. Hardening only the error branch
    // left the defect the comment above describes alive on every 2xx: a
    // truncated 200 threw a SyntaxError that surfaced as the estate
    // page's load error and the scenario page's preview error, reading
    // "Unexpected end of JSON input" where it meant to say what went
    // wrong.
    const parsed = await readJSON(res);
    if (!parsed.ok) throw new Error(`${path}: the response could not be read as JSON`);
    return parsed.value as T;
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
      const parsed = await readJSON(res);
      const body = parsed.value;
      if ((res.ok || res.status === 409) && isActionResult(body)) return body;
      // `parsed.ok` is the discriminator, and it is the reason
      // `readJSON` returns a pair. A body that FAILED TO PARSE means
      // the server was cut off mid-write, so the 2xx it had already
      // committed to is this server's word and the result was clean. A
      // body that parsed into something that is not an ActionResult
      // means something ELSE answered -- a captive portal, a proxy, a
      // dev-server fallback -- and its status proves nothing about a
      // deploy that may never have been dispatched.
      // A 2xx whose body will not parse is not a failed deploy, and it
      // is not an unknown one either: `writeActionResult` answers 2xx
      // ONLY for a provably clean result, so the status the code has
      // already read carries the answer. Throwing it as unknown filed a
      // permanent leak report for the one response shape that
      // guarantees nothing was left behind -- and saying "deploy
      // failed: 200" named a success status as a failure.
      if (res.ok && !parsed.ok) {
        throw new DeployError(
          "the deploy succeeded, but its result could not be read — see the Deployments page for what it created",
          "clean"
        );
      }
      if (res.ok) {
        throw new DeployError(
          "the server answered success with something this page does not recognise, so what happened is unknown",
          "unknown"
        );
      }
      throw new DeployError(
        (body as { error?: string })?.error || `deploy failed: ${res.status}`,
        startedNothing(body) ? "refused" : "unknown"
      );
    }
    // No JSON body at all, so no claim: unknown.
    throw new DeployError((await res.text()) || `deploy failed: ${res.status}`, "unknown");
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
      const parsed = await readJSON(res);
      const body = parsed.value;
      if ((res.ok || res.status === 409) && isActionResult(body)) return body;
      if (!parsed.ok) {
        throw new Error(`teardown answered ${res.status}, but its result could not be read`);
      }
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

export { DeployError };
