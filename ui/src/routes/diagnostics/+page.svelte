<script lang="ts">
  import { onMount } from "svelte";
  import { api } from "$lib/api";
  import type { DiagnosticsResponse } from "$lib/types";

  let diagnostics: DiagnosticsResponse | null = null;
  let errorMessage = "";

  onMount(async () => {
    try {
      diagnostics = (await api.getDiagnostics()) as DiagnosticsResponse;
    } catch (err) {
      errorMessage = err instanceof Error ? err.message : "Failed to load diagnostics";
    }
  });
</script>

<h1 class="text-2xl font-bold text-slate-900">Backend Diagnostics</h1>
<p class="mt-2 text-slate-600">Generator runtime checks from the active backend process.</p>

{#if errorMessage}
  <p class="mt-4 text-sm text-red-700">{errorMessage}</p>
{:else if diagnostics}
  <div class="mt-4 rounded border border-slate-300 bg-white/70 p-4 text-sm text-slate-800">
    <div><span class="font-semibold">Agent type:</span> {diagnostics.agent_type}</div>
    <div><span class="font-semibold">Backend session:</span> {diagnostics.session_id || "unknown"}</div>
    <div><span class="font-semibold">Backend started:</span> {diagnostics.started_at || "unknown"}</div>
    <div><span class="font-semibold">Ready:</span> {diagnostics.ready ? "yes" : "no"}</div>
    <div class="mt-2"><span class="font-semibold">Summary:</span> {diagnostics.summary}</div>
  </div>

  <div class="mt-4 space-y-3">
    {#each diagnostics.checks as check}
      <section class={`rounded border p-4 text-sm ${check.status === "pass" ? "border-emerald-200 bg-emerald-50 text-emerald-950" : "border-red-200 bg-red-50 text-red-950"}`}>
        <div class="font-semibold">{check.name}</div>
        <div class="mt-1">Status: {check.status}</div>
        <div class="mt-1 break-words">{check.detail}</div>
      </section>
    {/each}
  </div>

  {#if diagnostics.limitations?.length}
    <div class="mt-4 rounded border border-amber-200 bg-amber-50 p-4 text-sm text-amber-950">
      <div class="font-semibold">Limitations</div>
      <ul class="mt-2 list-disc pl-5">
        {#each diagnostics.limitations as limitation}
          <li>{limitation}</li>
        {/each}
      </ul>
    </div>
  {/if}
{/if}
