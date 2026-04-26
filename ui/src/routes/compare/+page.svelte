<script lang="ts">
  import { api } from "$lib/api";
  import type { RunSummary } from "$lib/types";
  import { afterNavigate } from "$app/navigation";

  type Diff = { filename: string; status: string; unified_diff?: string };

  let scenarios: string[] = [];
  let scenario = "";
  let runs: RunSummary[] = [];
  let run1 = "";
  let run2 = "";
  let diffs: Diff[] = [];
  let activeFile = "";
  let loading = false;
  let errorMessage = "";

  async function loadScenarios() {
    const resp = await api.getRuns();
    const seen = new Set<string>();
    const allRuns = (resp.runs as RunSummary[]) || [];
    for (const r of allRuns) {
      if (r.scenario) seen.add(r.scenario);
    }
    scenarios = [...seen].sort();
    if (!scenario && scenarios.length > 0) {
      scenario = scenarios[0];
    }
  }

  async function loadRunsForScenario() {
    runs = [];
    run1 = "";
    run2 = "";
    diffs = [];
    activeFile = "";
    if (!scenario) return;
    try {
      const resp = await api.getRunsForScenario(scenario);
      runs = (resp.runs as RunSummary[]) || [];
      runs.sort((a, b) => (b.run_id || "").localeCompare(a.run_id || ""));
      if (runs.length >= 2) {
        run1 = runs[1].run_id || "";
        run2 = runs[0].run_id || "";
      }
    } catch (err) {
      errorMessage = err instanceof Error ? err.message : "Failed to load runs";
    }
  }

  async function runCompare() {
    if (!scenario || !run1 || !run2) return;
    loading = true;
    errorMessage = "";
    diffs = [];
    activeFile = "";
    try {
      const resp = await api.compareRuns(scenario, run1, run2);
      diffs = resp.diffs || [];
      const firstChanged = diffs.find((d) => d.status !== "unchanged");
      activeFile = firstChanged ? firstChanged.filename : diffs[0]?.filename || "";
    } catch (err) {
      errorMessage = err instanceof Error ? err.message : "Compare request failed";
    } finally {
      loading = false;
    }
  }

  function statusBadgeClass(status: string): string {
    switch (status) {
      case "added":
        return "bg-emerald-100 text-emerald-900";
      case "removed":
        return "bg-rose-100 text-rose-900";
      case "modified":
        return "bg-amber-100 text-amber-900";
      default:
        return "bg-slate-200 text-slate-700";
    }
  }

  $: activeDiff = diffs.find((d) => d.filename === activeFile);

  // afterNavigate fires on the initial mount too, so a single call here
  // covers both the first load and subsequent client-side route changes
  // — avoids a double fetch.
  afterNavigate(loadScenarios);
  $: if (scenario) loadRunsForScenario();
</script>

<svelte:head><title>Compare Runs · infrafactory</title></svelte:head>

<section class="space-y-6 p-6" data-testid="compare-section">
  <header>
    <h1 class="text-2xl font-semibold">Compare Runs</h1>
    <p class="text-sm text-slate-500">
      Diff the generated IaC between two runs of the same scenario.
    </p>
  </header>

  {#if errorMessage}
    <p class="rounded bg-rose-50 px-3 py-2 text-rose-900" data-testid="compare-error">{errorMessage}</p>
  {/if}

  <div class="grid gap-4 sm:grid-cols-3">
    <label class="block">
      <span class="text-sm font-medium">Scenario</span>
      <select bind:value={scenario} data-testid="compare-scenario" class="mt-1 w-full rounded border border-slate-300 px-2 py-1">
        {#each scenarios as s}
          <option value={s}>{s}</option>
        {/each}
      </select>
    </label>
    <label class="block">
      <span class="text-sm font-medium">Run 1 (left)</span>
      <select bind:value={run1} data-testid="compare-run1" class="mt-1 w-full rounded border border-slate-300 px-2 py-1">
        <option value="">— select —</option>
        {#each runs as r}
          <option value={r.run_id}>{r.run_id} ({r.status})</option>
        {/each}
      </select>
    </label>
    <label class="block">
      <span class="text-sm font-medium">Run 2 (right)</span>
      <select bind:value={run2} data-testid="compare-run2" class="mt-1 w-full rounded border border-slate-300 px-2 py-1">
        <option value="">— select —</option>
        {#each runs as r}
          <option value={r.run_id}>{r.run_id} ({r.status})</option>
        {/each}
      </select>
    </label>
  </div>

  <button
    type="button"
    on:click={runCompare}
    disabled={!scenario || !run1 || !run2 || run1 === run2 || loading}
    data-testid="compare-run"
    class="rounded bg-slate-900 px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
  >
    {loading ? "Computing diff…" : "Compare"}
  </button>

  {#if diffs.length > 0}
    <div class="grid gap-4 lg:grid-cols-[16rem_1fr]">
      <aside data-testid="compare-files" class="space-y-1 rounded border border-slate-200 p-2">
        <h2 class="px-2 text-xs font-semibold uppercase tracking-wide text-slate-500">Files</h2>
        {#each diffs as d}
          <button
            type="button"
            on:click={() => (activeFile = d.filename)}
            data-testid="compare-file-{d.filename}"
            class="flex w-full items-center justify-between rounded px-2 py-1 text-left text-sm hover:bg-slate-100"
            class:bg-slate-100={activeFile === d.filename}
          >
            <span class="truncate">{d.filename}</span>
            <span
              class="ml-2 rounded-full px-2 py-0.5 text-[0.6rem] font-semibold uppercase {statusBadgeClass(d.status)}"
              data-testid="compare-status-{d.filename}"
            >
              {d.status}
            </span>
          </button>
        {/each}
      </aside>

      <article data-testid="compare-diff" class="rounded border border-slate-200">
        {#if activeDiff && activeDiff.unified_diff}
          <pre class="overflow-x-auto p-3 font-mono text-xs leading-relaxed">{activeDiff.unified_diff}</pre>
        {:else if activeDiff}
          <p class="p-3 text-sm text-slate-500">No textual change for {activeDiff.filename}.</p>
        {:else}
          <p class="p-3 text-sm text-slate-500">Pick a file to view its diff.</p>
        {/if}
      </article>
    </div>
  {/if}
</section>
