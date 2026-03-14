<script lang="ts">
  import { onMount } from "svelte";
  import { api } from "$lib/api";
  import type { ScenarioGroup } from "$lib/types";

  let groups: ScenarioGroup[] = [];

  onMount(async () => {
    try {
      const payload = await api.getScenarios();
      groups = (payload.groups as ScenarioGroup[]) || [];
    } catch {
      groups = [];
    }
  });
</script>

<h1 class="text-3xl font-bold text-slate-900">InfraFactory Dashboard</h1>
<p class="mt-2 text-slate-600">Scenario overview and quick run actions.</p>

<div class="mt-6 grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
  {#each groups as group}
    {#each group.scenarios as sc}
      <article class="rounded-2xl border border-slate-300 bg-white/80 p-4">
        <p class="text-xs uppercase tracking-wide text-slate-500">{group.name}</p>
        <h2 class="mt-1 text-lg font-semibold text-slate-900">{sc.name}</h2>
        <p class="mt-2 text-sm text-slate-700">{sc.description}</p>
        <div class="mt-4 flex gap-2">
          <a class="rounded bg-slate-900 px-3 py-1.5 text-xs text-white" href={`/scenarios/${sc.path}`}>Open</a>
          <a class="rounded border border-slate-400 px-3 py-1.5 text-xs text-slate-800" href={`/output/${sc.name}`}>Output</a>
        </div>
      </article>
    {/each}
  {/each}
</div>
