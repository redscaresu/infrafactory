<script lang="ts">
  import { onMount } from "svelte";
  import { api } from "$lib/api";
  import { deriveCurrentIteration, deriveCurrentStage, deriveFailureHint, deriveLiveConsoleNotice, formatBaselineState, mergeConsoleLines, needsFinalReload, selectLatestRun, synthesizeLiveConsoleLines } from "$lib/run-view.js";
  import { connectWS } from "$lib/ws";

  type RunFailure = {
    layer?: string;
    stage?: string;
    check?: string;
    command?: string;
    detail?: string;
  };

  type IterationArtifact = {
    iteration?: number;
    stages?: { layer?: string; stage?: string; status?: string; detail?: string }[];
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
  let planText = "";
  let baselineState = "";
  let baselineOpen = false;
  let planOpen = false;
  let finalReloadDone = false;
  let elapsed = "";
  let elapsedTimer: ReturnType<typeof setInterval> | null = null;

  function startElapsedTimer() {
    if (elapsedTimer) return;
    const start = Date.now();
    const tick = () => {
      const secs = Math.floor((Date.now() - start) / 1000);
      const m = Math.floor(secs / 60);
      const s = secs % 60;
      elapsed = m > 0 ? `${m}m ${s}s` : `${s}s`;
    };
    tick();
    elapsedTimer = setInterval(tick, 1000);
  }

  function stopElapsedTimer() {
    if (elapsedTimer) {
      clearInterval(elapsedTimer);
      elapsedTimer = null;
    }
  }

  $: if (latestStatus === "running") {
    startElapsedTimer();
  } else {
    stopElapsedTimer();
  }

  $: currentIteration = deriveCurrentIteration(runMeta, iterations);

  $: currentStage = deriveCurrentStage(mergedLines);

  $: failureCards = iterations.flatMap((iteration) =>
    (iteration.failures || []).map((failure) => ({
      iteration: iteration.iteration || 0,
      layer: failure.layer || "unknown",
      stage: failure.stage || "unknown",
      check: failure.check || "unknown",
      command: failure.command || "-",
      detail: failure.detail || "No detail provided"
    }))
  );
  $: layer3Stages = iterations.flatMap((iteration) =>
    (iteration.stages || []).filter((stage) => stage.layer === "sandbox_deploy").map((stage) => ({
      iteration: iteration.iteration || 0,
      stage: stage.stage || "unknown",
      status: stage.status || "unknown",
      detail: stage.detail || ""
    }))
  );
  $: realProbeCards = failureCards.filter((failure) => failure.layer === "sandbox_deploy" && ["connectivity", "http_probe", "dns_resolution", "real_probe"].includes(failure.check));

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
    try {
      planText = await api.getRunPlan(scenario, runID);
    } catch {
      planText = "";
    }
    try {
      baselineState = formatBaselineState(await api.getRunBaseline(scenario, runID));
    } catch {
      baselineState = "";
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
    if (needsFinalReload(runMeta, finalReloadDone)) {
      finalReloadDone = true;
      setTimeout(async () => {
        try { runMeta = await api.getRun(scenario, runID); } catch {}
        const reloaded: any[] = [];
        for (let i = 1; i <= 20; i += 1) {
          try { reloaded.push(await api.getIteration(scenario, runID, i)); } catch { break; }
        }
        if (reloaded.length > iterations.length) iterations = reloaded;
        try {
          const log = await api.getRunLog(scenario, runID);
          replayLines = log.split("\n").map((l) => l.trim()).filter((l) => l.length > 0);
        } catch {}
      }, 500);
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
      stopElapsedTimer();
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
    <div><span class="font-semibold">Mode:</span> {runMeta?.incremental ? "incremental" : "clean"}</div>
    <div><span class="font-semibold">Layer 3:</span> {runMeta?.layer3_enabled ? "enabled" : "disabled"}</div>
  </div>
{/if}

{#if latestStatus === "running" && currentIteration > 0}
  <div class="mt-4 rounded-lg border-2 border-indigo-300 bg-indigo-50 p-5 text-indigo-950 shadow-sm">
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-3">
        <div class="relative flex h-4 w-4 items-center justify-center">
          <div class="absolute h-4 w-4 animate-ping rounded-full bg-indigo-400 opacity-75"></div>
          <div class="h-3 w-3 rounded-full bg-indigo-600"></div>
        </div>
        <span class="text-xl font-bold">Iteration {currentIteration}</span>
        {#if currentStage}
          <span class="animate-pulse rounded-md bg-indigo-200 px-3 py-1 text-sm font-semibold uppercase tracking-wider text-indigo-800">{currentStage}</span>
        {/if}
      </div>
      <div class="text-right">
        <div class="font-mono text-lg font-bold text-indigo-700">{elapsed}</div>
        <div class="text-xs text-indigo-500">elapsed</div>
      </div>
    </div>
    {#if iterations.length > 0}
      <div class="mt-2 text-sm text-indigo-600">{iterations.length} iteration{iterations.length !== 1 ? "s" : ""} completed{iterations.flatMap(i => i.failures || []).length > 0 ? ` with ${iterations.flatMap(i => i.failures || []).length} failure(s)` : ""}</div>
    {/if}
  </div>
{:else if latestStatus && latestStatus !== "running" && latestStatus !== "starting"}
  <div class="mt-4 flex items-center gap-3 rounded border p-4 text-sm {latestStatus === 'success' ? 'border-emerald-200 bg-emerald-50 text-emerald-950' : 'border-red-200 bg-red-50 text-red-950'}">
    <span class="text-lg font-bold">{latestStatus === "success" ? "Run succeeded" : "Run failed"}</span>
    {#if runMeta?.terminal_reason}
      <span class="rounded bg-white/60 px-2 py-0.5 text-xs font-medium">{runMeta.terminal_reason}</span>
    {/if}
    <span class="text-xs">({iterations.length} iteration{iterations.length !== 1 ? "s" : ""})</span>
  </div>
{/if}

{#if planText || baselineState}
  <div class="mt-4 grid gap-4 lg:grid-cols-2">
    <section class="rounded border border-slate-300 bg-white/70 p-4 text-sm text-slate-800">
      <div class="flex items-center justify-between gap-3">
        <h2 class="font-semibold">Plan Diff</h2>
        <button class="rounded border border-slate-300 px-2 py-1 text-xs" on:click={() => (planOpen = !planOpen)}>
          {planOpen ? "Hide" : "Show"}
        </button>
      </div>
      {#if planOpen}
        <pre class="mt-3 max-h-80 overflow-auto rounded bg-slate-950 p-3 text-xs text-slate-100">{planText || "No plan artifact recorded."}</pre>
      {/if}
    </section>
    <section class="rounded border border-slate-300 bg-white/70 p-4 text-sm text-slate-800">
      <div class="flex items-center justify-between gap-3">
        <h2 class="font-semibold">Baseline State</h2>
        <button class="rounded border border-slate-300 px-2 py-1 text-xs" on:click={() => (baselineOpen = !baselineOpen)}>
          {baselineOpen ? "Hide" : "Show"}
        </button>
      </div>
      {#if baselineOpen}
        <pre class="mt-3 max-h-80 overflow-auto rounded bg-slate-950 p-3 text-xs text-slate-100">{baselineState || "No baseline artifact recorded."}</pre>
      {/if}
    </section>
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

{#if layer3Stages.length > 0}
  <div class="mt-4 rounded border border-sky-200 bg-sky-50 p-4 text-sm text-sky-950">
    <h2 class="font-semibold">Layer 3 Progress</h2>
    <div class="mt-2 space-y-2">
      {#each layer3Stages as stage}
        <div>Iteration {stage.iteration}: {stage.stage} = {stage.status}{stage.detail ? ` (${stage.detail})` : ""}</div>
      {/each}
    </div>
  </div>
{/if}

{#if realProbeCards.length > 0}
  <div class="mt-4 space-y-3">
    {#each realProbeCards as failure}
      <section class="rounded border border-sky-200 bg-sky-50 p-4 text-sm text-sky-950">
        <h2 class="font-semibold">Layer 3 Probe: iteration {failure.iteration}</h2>
        <p class="mt-2"><span class="font-semibold">Check:</span> {failure.check}</p>
        <p class="mt-1"><span class="font-semibold">Stage:</span> {failure.stage}</p>
        <p class="mt-2 whitespace-pre-wrap break-words">{failure.detail}</p>
      </section>
    {/each}
  </div>
{/if}

{#if iterations.length > 0}
  <h2 class="mt-6 text-lg font-bold text-slate-900">Iteration Timeline</h2>
  <div class="mt-2 space-y-3">
    {#each iterations as iteration}
      {@const hasFails = (iteration.failures?.length || 0) > 0}
      {@const iterNum = iteration.iteration || 0}
      <section class="rounded border p-4 text-sm {hasFails ? 'border-red-200 bg-red-50 text-red-950' : 'border-emerald-200 bg-emerald-50 text-emerald-950'}">
        <div class="flex items-center gap-2">
          <span class="text-base font-bold">Iteration {iterNum}</span>
          <span class="rounded px-2 py-0.5 text-xs font-semibold uppercase {hasFails ? 'bg-red-200 text-red-800' : 'bg-emerald-200 text-emerald-800'}">
            {hasFails ? `${iteration.failures.length} failure${iteration.failures.length !== 1 ? 's' : ''}` : 'passed'}
          </span>
        </div>

        {#if iteration.stages?.length}
          <div class="mt-2 flex flex-wrap gap-1">
            {#each iteration.stages as stage}
              <span class="rounded px-1.5 py-0.5 text-xs {stage.status === 'pass' ? 'bg-emerald-100 text-emerald-700' : stage.status === 'fail' ? 'bg-red-100 text-red-700' : 'bg-slate-100 text-slate-600'}">
                {stage.stage || 'unknown'}: {stage.status || 'unknown'}
              </span>
            {/each}
          </div>
        {/if}

        {#if hasFails}
          <div class="mt-3 space-y-2">
            <p class="text-xs font-semibold uppercase tracking-wide text-red-700">Retry reason:</p>
            {#each iteration.failures as failure}
              <div class="rounded border border-red-200 bg-white/60 p-2 text-xs">
                <span class="font-semibold">{failure.check || failure.stage || 'unknown'}</span>
                {#if failure.command}
                  <span class="text-red-600"> ({failure.command})</span>
                {/if}
                <p class="mt-1 whitespace-pre-wrap break-words text-red-800">{failure.detail || 'No detail'}</p>
              </div>
            {/each}
          </div>
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
