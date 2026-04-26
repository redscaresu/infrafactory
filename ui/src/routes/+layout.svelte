<script lang="ts">
  import "../app.css";
  import { onMount } from "svelte";
  import { api } from "$lib/api";
  import type { ConfigResponse, ScenarioGroup, Scenario } from "$lib/types";

  type CloudGroup = { cloud: string; label: string; scenarios: Scenario[] };

  const CLOUD_LABELS: Record<string, string> = {
    scaleway: "SCALEWAY",
    gcp: "GCP"
  };

  let cloudGroups: CloudGroup[] = [];
  let backendVersion = "";
  let agentType = "";
  let uiMode = "";
  let backendSessionID = "";
  let backendStartedAt = "";

  function regroupByCloud(groups: ScenarioGroup[]): CloudGroup[] {
    const buckets = new Map<string, Scenario[]>();
    for (const group of groups) {
      for (const sc of group.scenarios) {
        const cloud = (sc.cloud || "").toLowerCase();
        const key = cloud in CLOUD_LABELS ? cloud : "other";
        const existing = buckets.get(key) || [];
        existing.push(sc);
        buckets.set(key, existing);
      }
    }
    const order = ["scaleway", "gcp", "other"];
    return order
      .filter((k) => buckets.has(k))
      .concat([...buckets.keys()].filter((k) => !order.includes(k)).sort())
      .map((cloud) => ({
        cloud,
        label: CLOUD_LABELS[cloud] || cloud.toUpperCase() || "OTHER",
        scenarios: (buckets.get(cloud) || []).slice().sort((a, b) => a.path.localeCompare(b.path))
      }));
  }

  onMount(async () => {
    uiMode = window.location.port === "5173" ? "UI dev" : "Embedded UI";
    try {
      const payload = await api.getScenarios();
      const groups = (payload.groups as ScenarioGroup[]) || [];
      cloudGroups = regroupByCloud(groups);
    } catch {
      cloudGroups = [];
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
      {#each cloudGroups as group (group.cloud)}
        <section data-testid="sidebar-cloud-{group.cloud}">
          <h2 class="text-xs uppercase tracking-wider text-slate-500" data-testid="sidebar-cloud-label">{group.label}</h2>
          <ul class="mt-2 space-y-1">
            {#each group.scenarios as sc (sc.path)}
              <li>
                <a class="text-sm text-slate-700 hover:text-slate-900" href={`/scenarios/${sc.path}`} data-testid={`sidebar-scenario-${sc.path}`}>{sc.name}</a>
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
