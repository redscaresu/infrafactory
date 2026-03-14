<script lang="ts">
  import { onMount } from "svelte";
  import { api } from "$lib/api";
  import { deriveFailureHint, deriveLiveConsoleNotice, mergeConsoleLines, selectLatestRun, synthesizeLiveConsoleLines } from "$lib/run-view.js";
  import { connectWS } from "$lib/ws";

  type RunFailure = {
    stage?: string;
    check?: string;
    command?: string;
    detail?: string;
  };

  type IterationArtifact = {
    iteration?: number;
    statuses?: { layer?: string; stage?: string; status?: string }[];
    failures?: RunFailure[];
    failure_summary?: string[];
  };

  let lines: string[] = [];
  let replayLines: string[] = [];
  let scenario = "";
  let runID = "";
  let runMeta: any = null;
  let iterations: IterationArtifact[] = [];
  let pollTimer: ReturnType<typeof setInterval> | null = null;
  let statusMessage = "";
  let failureHint = "";

  $: failureCards = iterations.flatMap((iteration) =>
    (iteration.failures || []).map((failure) => ({
      iteration: iteration.iteration || 0,
      stage: failure.stage || "unknown",
      check: failure.check || "unknown",
      command: failure.command || "-",
      detail: failure.detail || "No detail provided"
    }))
  );

  $: latestStatus = runMeta?.status || "starting";
  $: mergedLines = mergeConsoleLines(replayLines, lines);
  $: consoleNotice = deriveLiveConsoleNotice(runMeta, iterations, mergedLines);
  $: consoleLines = mergedLines.length > 0 ? mergedLines : synthesizeLiveConsoleLines(runMeta, iterations, mergedLines);

  async function loadLatestRun() {
    const resp = scenario ? await api.getRunsForScenario(scenario) : await api.getRuns();
    const runs = ((resp.runs as any[]) || []).slice();
    const active = selectLatestRun(runs, scenario);
    if (!active) {
      statusMessage = scenario ? `No runs recorded yet for ${scenario}.` : "No runs recorded yet.";
      return;
    }
    scenario = active.scenario || "";
    runID = active.run_id || "";
    if (scenario && runID) {
      const url = `/live?scenario=${encodeURIComponent(scenario)}&run_id=${encodeURIComponent(runID)}`;
      window.history.replaceState({}, "", url);
      statusMessage = "";
    }
  }

  async function loadRunState() {
    if (!scenario || !runID) return;

    try {
      runMeta = await api.getRun(scenario, runID);
    } catch {
      return;
    }

    const loaded: any[] = [];
    for (let i = 1; i <= 10; i += 1) {
      try {
        const iteration = await api.getIteration(scenario, runID, i);
        loaded.push(iteration);
      } catch {
        break;
      }
    }
    iterations = loaded;
    try {
      const log = await api.getRunLog(scenario, runID);
      replayLines = log
        .split("\n")
        .map((line) => line.trim())
        .filter((line) => line.length > 0);
    } catch {
      replayLines = [];
    }
    const firstFailureDetail = loaded
      .flatMap((iteration) => iteration.failures || [])
      .map((failure) => failure.detail || "")
      .find((detail) => detail.length > 0);
    failureHint = firstFailureDetail ? deriveFailureHint(firstFailureDetail) : "";

    if (runMeta?.status && runMeta.status !== "running" && pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  }

  onMount(() => {
    const params = new URLSearchParams(window.location.search);
    scenario = params.get("scenario") || "";
    runID = params.get("run_id") || "";

    const beginPolling = () => {
      void loadRunState();
      pollTimer = setInterval(() => {
        void loadRunState();
      }, 2000);
    };

    if (scenario && runID) {
      beginPolling();
    } else {
      void loadLatestRun().then(() => {
        if (scenario && runID) {
          beginPolling();
        }
      });
    }

    const disconnect = connectWS((msg) => {
      lines = [...lines.slice(-999), JSON.stringify(msg)];
      void loadRunState();
    });

    return () => {
      disconnect();
      if (pollTimer) clearInterval(pollTimer);
    };
  });
</script>

<h1 class="text-2xl font-bold text-slate-900">Live Run</h1>
<p class="mt-2 text-slate-600">Start a run from a scenario page. Live events and latest run state appear below.</p>

{#if statusMessage}
  <p class="mt-4 text-sm text-slate-700">{statusMessage}</p>
{/if}

{#if scenario && runID}
  <div class="mt-4 rounded border border-slate-300 bg-white/70 p-4 text-sm text-slate-800">
    <div><span class="font-semibold">Scenario:</span> {scenario}</div>
    <div><span class="font-semibold">Run ID:</span> {runID}</div>
    <div><span class="font-semibold">Status:</span> {latestStatus}</div>
    <div><span class="font-semibold">Terminal reason:</span> {runMeta?.terminal_reason || "-"}</div>
  </div>
{/if}

{#if failureHint}
  <div class="mt-4 rounded border border-amber-200 bg-amber-50 p-4 text-sm text-amber-950">
    <span class="font-semibold">Failure hint:</span> {failureHint}
    <div class="mt-2">
      <a class="underline" href="/diagnostics">Open backend diagnostics</a>
    </div>
  </div>
{/if}

{#if failureCards.length > 0}
  <div class="mt-4 space-y-3">
    {#each failureCards as failure}
      <section class="rounded border border-red-200 bg-red-50 p-4 text-sm text-red-950">
        <h2 class="font-semibold">Iteration {failure.iteration}: {failure.stage}</h2>
        <p class="mt-2"><span class="font-semibold">Check:</span> {failure.check}</p>
        <p class="mt-1"><span class="font-semibold">Command:</span> {failure.command}</p>
        <p class="mt-2 whitespace-pre-wrap break-words">{failure.detail}</p>
      </section>
    {/each}
  </div>
{:else if iterations.length > 0}
  <div class="mt-4 space-y-3">
    {#each iterations as iteration}
      <section class="rounded border border-slate-300 bg-white/70 p-4 text-sm text-slate-800">
        <h2 class="font-semibold">Iteration {iteration.iteration}</h2>
        <p class="mt-2">
          <span class="font-semibold">Statuses:</span>
          {#if iteration.statuses?.length}
            {iteration.statuses.map((status) => `${status.stage || "unknown"}=${status.status || "unknown"}`).join(", ")}
          {:else}
            no status events recorded
          {/if}
        </p>
        {#if iteration.failure_summary?.length}
          <p class="mt-2 whitespace-pre-wrap break-words">{iteration.failure_summary.join("\n")}</p>
        {/if}
      </section>
    {/each}
  </div>
{/if}

<div class="mt-4 h-[420px] overflow-auto rounded border border-slate-300 bg-slate-950 p-3 font-mono text-xs text-slate-100">
  {#if consoleLines.length === 0}
    <p>{consoleNotice}</p>
  {:else}
    {#each consoleLines as line}
      <div>{line}</div>
    {/each}
  {/if}
</div>
