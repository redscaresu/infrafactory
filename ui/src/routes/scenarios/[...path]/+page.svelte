<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/stores";
  import { api } from "$lib/api";

  let scenarioPath = "";
  let detail: any = null;
  let rawYAML = "";
  let status = "";
  let running = false;

  $: scenarioPath = ($page.params.path || "").toString();

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

  async function saveScenario() {
    status = "";
    try {
      await api.putScenario(scenarioPath, rawYAML);
      status = "Saved";
      await loadDetail();
    } catch (err) {
      status = err instanceof Error ? err.message : "Save failed";
    }
  }

  async function runScenario() {
    if (!detail?.name || running) return;
    running = true;
    status = "Starting run...";
    try {
      const resp = await api.startRun(detail.name);
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

  onMount(loadDetail);
</script>

{#if detail}
  <h1 class="text-2xl font-bold text-slate-900">{detail.name}</h1>
  <p class="mt-2 text-slate-700">{detail.description}</p>
  <div class="mt-4 flex gap-2">
    <button class="rounded bg-slate-900 px-3 py-1.5 text-xs text-white disabled:opacity-60" on:click={runScenario} disabled={running}>
      {running ? "Starting..." : "Run"}
    </button>
    <button class="rounded border border-slate-400 px-3 py-1.5 text-xs text-slate-900" on:click={saveScenario}>Save</button>
  </div>
  {#if status}<p class="mt-3 text-sm text-slate-700">{status}</p>{/if}
  <textarea class="mt-4 h-[460px] w-full rounded border border-slate-300 p-3 font-mono text-sm" bind:value={rawYAML}></textarea>
{/if}
