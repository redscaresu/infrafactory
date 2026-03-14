<script lang="ts">
  import { onMount } from "svelte";
  import { api } from "$lib/api";
  import type { RunSummary } from "$lib/types";
  import { filterRuns, formatRunDate } from "$lib/run-view.js";

  let runs: RunSummary[] = [];
  let search = "";
  let statusFilter = "all";
  let errorMessage = "";

  onMount(async () => {
    try {
      const resp = await api.getRuns();
      runs = (resp.runs as RunSummary[]) || [];
    } catch (err) {
      runs = [];
      errorMessage = err instanceof Error ? err.message : "Failed to load run history";
    }
  });

  $: filteredRuns = filterRuns(runs, search, statusFilter);
</script>

<h1 class="text-2xl font-bold text-slate-900">Run History</h1>
<div class="mt-4 flex flex-wrap gap-3">
  <input
    class="min-w-[260px] rounded border border-slate-300 bg-white px-3 py-2 text-sm"
    bind:value={search}
    placeholder="Filter by scenario, run ID, or terminal reason"
  />
  <select class="rounded border border-slate-300 bg-white px-3 py-2 text-sm" bind:value={statusFilter}>
    <option value="all">All statuses</option>
    <option value="success">Success</option>
    <option value="failed">Failed</option>
    <option value="running">Running</option>
  </select>
</div>
{#if errorMessage}
  <p class="mt-3 text-sm text-red-700">{errorMessage}</p>
{/if}
<table class="mt-4 w-full border-collapse text-sm">
  <thead>
    <tr class="text-left text-slate-500">
      <th class="pb-2">Scenario</th>
      <th class="pb-2">Run ID</th>
      <th class="pb-2">Started</th>
      <th class="pb-2">Status</th>
      <th class="pb-2">Terminal reason</th>
      <th class="pb-2">Actions</th>
    </tr>
  </thead>
  <tbody>
    {#each filteredRuns as run}
      <tr class="border-t border-slate-200">
        <td class="py-2"><a class="text-slate-900 underline" href={`/runs/${run.scenario}/${run.run_id}`}>{run.scenario}</a></td>
        <td class="py-2"><a class="text-slate-900 underline" href={`/runs/${run.scenario}/${run.run_id}`}>{run.run_id}</a></td>
        <td class="py-2">{formatRunDate(run.started_at)}</td>
        <td class="py-2">{run.status}</td>
        <td class="py-2">{run.terminal_reason || "-"}</td>
        <td class="py-2">
          <div class="flex gap-3">
            <a class="underline" href={`/runs/${run.scenario}/${run.run_id}`}>IaC</a>
            <a class="underline" href={`/live?scenario=${encodeURIComponent(run.scenario)}&run_id=${encodeURIComponent(run.run_id)}`}>Live</a>
          </div>
        </td>
      </tr>
    {/each}
  </tbody>
</table>
{#if !errorMessage && filteredRuns.length === 0}
  <p class="mt-4 text-sm text-slate-600">No runs match the current filters.</p>
{/if}
