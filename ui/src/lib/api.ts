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
    // prove itself clean. 423 is a refusal -- nothing ran -- and carries
    // an error, so it must NOT be parsed as a result: doing so found no
    // `clean` field and told the reader resources might still be
    // running after a request that never touched the cloud.
    if ((res.ok || res.status === 409) && ctype.includes("application/json")) {
      return (await res.json()) as ActionResult;
    }
    if (ctype.includes("application/json")) {
      const payload = (await res.json()) as { error?: string };
      throw new Error(payload.error || `deploy failed: ${res.status}`);
    }
    throw new Error((await res.text()) || `deploy failed: ${res.status}`);
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
    if ((res.ok || res.status === 409) && ctype.includes("application/json")) {
      return (await res.json()) as ActionResult;
    }
    if (ctype.includes("application/json")) {
      const payload = (await res.json()) as { error?: string };
      throw new Error(payload.error || `teardown failed: ${res.status}`);
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
