import { fetchPitfalls, fetchSavePitfalls } from "$lib/pitfalls-api.js";
import type {
  Pitfall,
  PitfallsResponse,
  SavePitfallsResponse,
  ScenarioLayer3StatusResponse,
  ScenarioRunModeResponse,
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
  getOutputFiles: (scenario: string) => request<{ files: string[] }>(`/api/output/${scenario}`),
  getOutputFile: (scenario: string, file: string) => request<string>(withFormat(`/api/output/${scenario}/${file}`)),
  getConfig: () => request("/api/config"),
  getPitfalls: (): Promise<PitfallsResponse> => fetchPitfalls() as Promise<PitfallsResponse>,
  savePitfalls: (provider: string, pitfalls: Pitfall[]): Promise<SavePitfallsResponse> =>
    fetchSavePitfalls(provider, pitfalls) as Promise<SavePitfallsResponse>
};
