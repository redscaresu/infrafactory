<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/stores";
  import { api } from "$lib/api";
  import { modeSummary, normalizeRunOptions } from "$lib/scenario-run.js";
  import type { ScenarioLayer3StatusResponse, ScenarioRunModeResponse } from "$lib/types";

  let scenarioPath = "";
  let detail: any = null;
  let rawYAML = "";
  let status = "";
  let running = false;
  let runMode: ScenarioRunModeResponse | null = null;
  let layer3Status: ScenarioLayer3StatusResponse | null = null;
  let runModeError = "";
  let layer3Error = "";
  let clean = false;
  let noDestroy = false;
  let layer3Enabled = false;

  $: scenarioPath = ($page.params.path || "").toString();
  $: runModeCard = modeSummary(runMode);

  function encodeLiveURL(scenario: string, runID: string): string {
    return `/live?scenario=${encodeURIComponent(scenario)}&run_id=${encodeURIComponent(runID)}`;
  }

  async function redirectToLatestRun(scenario: string) {
    const resp = await api.getRunsForScenario(scenario);
    const runs = ((resp.runs as any[]) || []).slice();
    if (runs.length === 0) {
      throw new Error("run already in progress, but no run metadata was found");
    }

    runs.sort((a, b) => (a.run_id < b.run_id ? 1 : -1));
    const active = runs.find((run) => run.status === "running") || runs[0];
    window.location.href = encodeLiveURL(scenario, active.run_id);
  }

  async function loadDetail() {
    if (!scenarioPath) return;
    detail = await api.getScenario(scenarioPath);
    rawYAML = detail.raw_yaml;
  }

  async function loadRunMode() {
    if (!scenarioPath) return;
    runModeError = "";
    try {
      runMode = await api.getScenarioRunMode(scenarioPath);
    } catch (err) {
      runMode = null;
      runModeError = err instanceof Error ? err.message : "Run mode detection failed";
    }
  }

  async function loadLayer3Status() {
    if (!scenarioPath) return;
    layer3Error = "";
    try {
      layer3Status = await api.getScenarioLayer3Status(scenarioPath);
      if (layer3Status?.config_default_enabled) {
        layer3Enabled = true;
      }
    } catch (err) {
      layer3Status = null;
      layer3Error = err instanceof Error ? err.message : "Layer 3 status lookup failed";
    }
  }

  async function saveScenario() {
    status = "";
    try {
      await api.putScenario(scenarioPath, rawYAML);
      status = "Saved";
      await loadDetail();
      await loadRunMode();
      await loadLayer3Status();
    } catch (err) {
      status = err instanceof Error ? err.message : "Save failed";
    }
  }

  async function runScenario() {
    if (!detail?.name || running) return;
    running = true;
    status = "Starting run...";
    try {
      const resp = await api.startRun(detail.name, normalizeRunOptions({ clean, no_destroy: noDestroy, layer3_enabled: layer3Enabled }));
      status = `Run started: ${resp.run_id}`;
      window.location.href = encodeLiveURL(detail.name, resp.run_id);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Run start failed";
      if (message.includes("run already in progress")) {
        status = message;
        try {
          await redirectToLatestRun(detail.name);
          return;
        } catch (redirectErr) {
          status = redirectErr instanceof Error ? redirectErr.message : message;
        }
      } else {
        status = message;
      }
      running = false;
    }
  }

  onMount(async () => {
    await loadDetail();
    await loadRunMode();
    await loadLayer3Status();
  });
</script>

{#if detail}
  <h1 class="text-2xl font-bold text-slate-900">{detail.name}</h1>
  <p class="mt-2 text-slate-700">{detail.description}</p>
  <div class="mt-4 rounded border border-slate-300 bg-white/80 p-4">
    <div class="flex items-start justify-between gap-4">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Next Run Mode</p>
        <p class="mt-2 text-lg font-semibold text-slate-900">{runModeCard.title}</p>
        <p class="mt-1 text-sm text-slate-600">{runModeCard.detail}</p>
      </div>
      <span
        class={`rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] ${
          layer3Enabled
            ? "bg-sky-100 text-sky-900"
            : runModeCard.tone === "incremental"
            ? "bg-emerald-100 text-emerald-900"
            : runModeCard.tone === "clean"
              ? "bg-amber-100 text-amber-900"
              : "bg-slate-200 text-slate-700"
        }`}
      >
        {layer3Enabled ? "mock + real" : runMode?.mode || "unknown"}
      </span>
    </div>
    {#if runMode}
      <div class="mt-4 grid gap-2 text-xs text-slate-600 md:grid-cols-3">
        <div class="rounded bg-slate-100 px-3 py-2">Mock resources: {runMode.has_mock_resources ? "yes" : "no"}</div>
        <div class="rounded bg-slate-100 px-3 py-2">terraform.tfstate: {runMode.has_tfstate ? "yes" : "no"}</div>
        <div class="rounded bg-slate-100 px-3 py-2">Previous success: {runMode.has_previous_successful_run ? "yes" : "no"}</div>
      </div>
    {/if}
    {#if runModeError}
      <p class="mt-3 text-sm text-red-700">{runModeError}</p>
    {/if}
    <div class="mt-4 rounded border border-slate-200 bg-slate-50 px-3 py-3 text-xs text-slate-700">
      <div class="flex flex-wrap items-center gap-3">
        <label class="flex items-center gap-2 rounded border border-slate-300 bg-white px-3 py-2 text-xs text-slate-800">
          <input type="checkbox" bind:checked={layer3Enabled} />
          <span>Layer 3 (Real Scaleway)</span>
        </label>
        <span class={`rounded-full px-2 py-1 font-semibold uppercase tracking-[0.16em] ${layer3Status?.ready ? "bg-emerald-100 text-emerald-900" : "bg-rose-100 text-rose-900"}`}>
          {layer3Status?.ready ? "credentials ready" : "credentials missing"}
        </span>
        {#if layer3Status?.project_id_configured}
          <span class="rounded-full bg-slate-200 px-2 py-1 font-semibold uppercase tracking-[0.16em] text-slate-700">project id configured</span>
        {/if}
      </div>
      <p class="mt-2">{layer3Status?.detail || "Layer 3 status unavailable."}</p>
      {#if layer3Status && layer3Status.missing_credentials.length > 0}
        <p class="mt-1">Missing: {layer3Status.missing_credentials.join(", ")}</p>
      {/if}
      {#if layer3Error}
        <p class="mt-2 text-red-700">{layer3Error}</p>
      {/if}
    </div>
  </div>
  <div class="mt-4 flex flex-wrap items-center gap-3">
    <label class="flex items-center gap-2 rounded border border-slate-300 bg-white px-3 py-2 text-xs text-slate-800">
      <input type="checkbox" bind:checked={noDestroy} disabled={clean} />
      <span>Keep state (`--no-destroy`)</span>
    </label>
    <label class="flex items-center gap-2 rounded border border-slate-300 bg-white px-3 py-2 text-xs text-slate-800">
      <input type="checkbox" bind:checked={clean} disabled={noDestroy} />
      <span>Force clean (`--clean`)</span>
    </label>
  </div>
  <div class="mt-4 flex gap-2">
    <button class="rounded bg-slate-900 px-3 py-1.5 text-xs text-white disabled:opacity-60" on:click={runScenario} disabled={running}>
      {running ? "Starting..." : "Run"}
    </button>
    <button class="rounded border border-slate-400 px-3 py-1.5 text-xs text-slate-900" on:click={saveScenario}>Save</button>
  </div>
  {#if status}<p class="mt-3 text-sm text-slate-700">{status}</p>{/if}
  <textarea class="mt-4 h-[460px] w-full rounded border border-slate-300 p-3 font-mono text-sm" bind:value={rawYAML}></textarea>
{/if}
