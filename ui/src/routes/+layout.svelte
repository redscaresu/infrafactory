<script lang="ts">
  import "../app.css";
  import { onMount } from "svelte";
  import { api } from "$lib/api";
  import type { ConfigResponse, ScenarioGroup } from "$lib/types";

  let groups: ScenarioGroup[] = [];
  let backendVersion = "";
  let agentType = "";
  let uiMode = "";
  let backendSessionID = "";
  let backendStartedAt = "";

  onMount(async () => {
    uiMode = window.location.port === "5173" ? "UI dev" : "Embedded UI";
    try {
      const payload = await api.getScenarios();
      groups = (payload.groups as ScenarioGroup[]) || [];
    } catch {
      groups = [];
    }

    try {
      const cfg = (await api.getConfig()) as ConfigResponse;
      backendVersion = cfg.version || "";
      agentType = cfg.agent.type || "";
    } catch {
      backendVersion = "";
      agentType = "";
    }

    try {
      const diagnostics = await api.getDiagnostics();
      backendSessionID = diagnostics.session_id || "";
      backendStartedAt = diagnostics.started_at || "";
    } catch {
      backendSessionID = "";
      backendStartedAt = "";
    }
  });
</script>

<div class="min-h-screen grid grid-cols-[280px_1fr]">
  <aside class="border-r border-slate-300/70 bg-white/70 p-4 backdrop-blur-sm">
    <a href="/" class="block text-xl font-bold text-slate-900">InfraFactory</a>
    <div class="mt-6 space-y-5">
      {#each groups as group}
        <section>
          <h2 class="text-xs uppercase tracking-wider text-slate-500">{group.name}</h2>
          <ul class="mt-2 space-y-1">
            {#each group.scenarios as sc}
              <li>
                <a class="text-sm text-slate-700 hover:text-slate-900" href={`/scenarios/${sc.path}`}>{sc.name}</a>
              </li>
            {/each}
          </ul>
        </section>
      {/each}
    </div>
    <nav class="mt-8 space-y-2 text-sm">
      <a class="block text-slate-700 hover:text-slate-900" href="/runs">Runs</a>
      <a class="block text-slate-700 hover:text-slate-900" href="/live">Live</a>
      <a class="block text-slate-700 hover:text-slate-900" href="/compare">Compare</a>
      <a class="block text-slate-700 hover:text-slate-900" href="/pitfalls">Pitfalls</a>
      <a class="block text-slate-700 hover:text-slate-900" href="/diagnostics">Diagnostics</a>
    </nav>
    <div class="mt-8 rounded border border-slate-300 bg-slate-50 p-3 text-xs text-slate-600">
      <div><span class="font-semibold">UI mode:</span> {uiMode || "unknown"}</div>
      <div><span class="font-semibold">Backend version:</span> {backendVersion || "unknown"}</div>
      <div><span class="font-semibold">Backend session:</span> {backendSessionID || "unknown"}</div>
      <div><span class="font-semibold">Backend started:</span> {backendStartedAt || "unknown"}</div>
      <div><span class="font-semibold">Agent:</span> {agentType || "unknown"}</div>
    </div>
  </aside>
  <main class="p-6">
    <slot />
  </main>
</div>
