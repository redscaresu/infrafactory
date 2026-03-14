<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/stores";
  import { api } from "$lib/api";
  import {
    buildLineDiff,
    buildRunArtifactsURL,
    buildRunBundleURL,
    buildSnapshotLabel,
    buildSnapshotOptions,
    getDefaultCompareSnapshot,
    highlightHCL
  } from "$lib/iac-view";

  let runMeta: any = null;
  let iterations: number[] = [];
  let iterationDetails: Record<number, any> = {};
  let selectedSnapshot: "final" | number = "final";
  let compareSnapshot: "final" | number | null = null;
  let files: string[] = [];
  let compareFiles: string[] = [];
  let selectedFile = "";
  let content = "";
  let compareContent = "";
  let loading = true;
  let fallbackNotice = "";

  $: snapshotOptions = buildSnapshotOptions(iterations);
  $: highlighted = highlightHCL(content || "");
  $: selectedIteration = typeof selectedSnapshot === "number" ? iterationDetails[selectedSnapshot] ?? null : null;
  $: bundleURL = runMeta ? buildRunBundleURL(runMeta.scenario, runMeta.run_id) : "";
  $: artifactsURL = runMeta ? buildRunArtifactsURL(runMeta.scenario, runMeta.run_id) : "";
  $: diffRows = buildLineDiff(compareContent || "", content || "");

  async function load() {
    loading = true;
    const scenario = $page.params.scenario;
    const runID = $page.params.runID;
    runMeta = await api.getRun(scenario, runID);

    try {
      const iterationResp = await api.getIterations(scenario, runID);
      iterations = iterationResp.iterations || [];
    } catch {
      iterations = [];
    }

    iterationDetails = {};
    for (const iteration of iterations) {
      try {
        iterationDetails[iteration] = await api.getIteration(scenario, runID, iteration);
      } catch {
        iterationDetails[iteration] = null;
      }
    }

    selectedSnapshot = iterations.length > 0 ? iterations[iterations.length - 1] : "final";
    compareSnapshot = getDefaultCompareSnapshot(selectedSnapshot, iterations);
    await loadSnapshot();
    loading = false;
  }

  async function loadSnapshot() {
    const scenario = $page.params.scenario;
    const runID = $page.params.runID;
    selectedFile = "";
    content = "";
    compareContent = "";
    fallbackNotice = "";

    try {
      files = await loadFilesForSnapshot(scenario, runID, selectedSnapshot);
      compareFiles = compareSnapshot === null ? [] : await loadFilesForSnapshot(scenario, runID, compareSnapshot);
    } catch {
      files = [];
      compareFiles = [];
    }

    if (selectedSnapshot === "final" && files.length === 0) {
      try {
        const fallback = await api.getOutputFiles(scenario);
        files = fallback.files || [];
        if (files.length > 0) {
          fallbackNotice = "Historical IaC was not stored for this run. Showing the current scenario output directory instead.";
        }
      } catch {
        // Keep empty state when no scenario output exists either.
      }
    }

    if (files.length > 0) {
      await openFile(files[0]);
    }
  }

  async function loadFilesForSnapshot(scenario: string, runID: string, snapshot: "final" | number) {
    if (snapshot === "final") {
      const resp = await api.getRunFiles(scenario, runID);
      return resp.files || [];
    }
    const resp = await api.getIterationFiles(scenario, runID, snapshot);
    return resp.files || [];
  }

  async function readFileForSnapshot(scenario: string, runID: string, snapshot: "final" | number, file: string) {
    if (snapshot === "final") {
      return api.getRunFile(scenario, runID, file);
    }
    return api.getIterationFile(scenario, runID, snapshot, file);
  }

  async function openFile(file: string) {
    const scenario = $page.params.scenario;
    const runID = $page.params.runID;
    selectedFile = file;
    if (fallbackNotice && selectedSnapshot === "final") {
      content = await api.getOutputFile(scenario, file);
    } else {
      content = await readFileForSnapshot(scenario, runID, selectedSnapshot, file);
    }
    if (compareSnapshot !== null && compareFiles.includes(file)) {
      compareContent = await readFileForSnapshot(scenario, runID, compareSnapshot, file);
    } else {
      compareContent = "";
    }
  }

  async function chooseSnapshot(snapshot: "final" | number) {
    selectedSnapshot = snapshot;
    compareSnapshot = getDefaultCompareSnapshot(snapshot, iterations);
    await loadSnapshot();
  }

  async function chooseCompareSnapshot(snapshotValue: string) {
    compareSnapshot = snapshotValue === "none" ? null : snapshotValue === "final" ? "final" : Number(snapshotValue);
    await loadSnapshot();
  }

  onMount(load);
</script>

{#if loading}
  <p class="text-sm text-slate-600">Loading run details...</p>
{:else if runMeta}
  <h1 class="text-2xl font-bold text-slate-900">Run {runMeta.run_id}</h1>
  <p class="mt-2 text-sm text-slate-700">Scenario: {runMeta.scenario} | Status: {runMeta.status} | Terminal: {runMeta.terminal_reason}</p>
  <div class="mt-4 flex flex-wrap gap-2 text-xs">
    <a class="rounded border border-slate-300 bg-white px-3 py-2 font-medium text-slate-800 hover:border-slate-900 hover:text-slate-900" href={`/live?scenario=${encodeURIComponent(runMeta.scenario)}&run_id=${encodeURIComponent(runMeta.run_id)}`}>Open Live View</a>
    <a class="rounded border border-slate-300 bg-white px-3 py-2 font-medium text-slate-800 hover:border-slate-900 hover:text-slate-900" href={bundleURL}>Download IaC Bundle</a>
    <a class="rounded border border-slate-300 bg-white px-3 py-2 font-medium text-slate-800 hover:border-slate-900 hover:text-slate-900" href={artifactsURL}>Download Full Run Archive</a>
  </div>
  <div class="mt-4 grid gap-4 lg:grid-cols-[260px_1fr]">
    <aside class="rounded border border-slate-300 bg-white/70 p-3">
      <h2 class="text-sm font-semibold text-slate-900">Snapshots</h2>
      <div class="mt-2 space-y-1">
        {#each snapshotOptions as snapshot}
          <button
            class={`block w-full rounded px-2 py-1 text-left text-xs ${selectedSnapshot === snapshot ? "bg-slate-900 text-white" : "hover:bg-slate-100 text-slate-800"}`}
            on:click={() => chooseSnapshot(snapshot)}
          >
            {buildSnapshotLabel(snapshot)}
          </button>
        {/each}
      </div>

      <label class="mt-4 block text-sm font-semibold text-slate-900" for="compare-snapshot">Diff against</label>
      <select
        id="compare-snapshot"
        class="mt-2 w-full rounded border border-slate-300 bg-white px-2 py-1 text-xs text-slate-800"
        value={compareSnapshot === null ? "none" : String(compareSnapshot)}
        on:change={(event) => chooseCompareSnapshot((event.currentTarget as HTMLSelectElement).value)}
      >
        <option value="none">No diff</option>
        {#each snapshotOptions.filter((snapshot) => snapshot !== selectedSnapshot) as snapshot}
          <option value={String(snapshot)}>{buildSnapshotLabel(snapshot)}</option>
        {/each}
      </select>

      <h2 class="mt-4 text-sm font-semibold text-slate-900">Files</h2>
      {#if files.length === 0}
        <p class="mt-2 text-xs text-slate-600">No generated files stored for this snapshot.</p>
      {:else}
        <div class="mt-2 space-y-1">
          {#each files as file}
            <button
              class={`block w-full rounded px-2 py-1 text-left text-xs ${selectedFile === file ? "bg-slate-900 text-white" : "hover:bg-slate-100 text-slate-800"}`}
              on:click={() => openFile(file)}
            >
              {file}
            </button>
          {/each}
        </div>
      {/if}
    </aside>
    <section class="space-y-4">
      <div class="rounded border border-slate-300 bg-white/70 p-3">
        <div class="flex items-center justify-between gap-3">
          <div>
            <h2 class="text-sm font-semibold text-slate-900">IaC Preview</h2>
            <p class="mt-1 text-xs text-slate-600">{buildSnapshotLabel(selectedSnapshot)}{selectedFile ? ` · ${selectedFile}` : ""}</p>
          </div>
        </div>
        {#if fallbackNotice}
          <p class="mt-2 rounded border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900">{fallbackNotice}</p>
        {/if}
        <div class="mt-3 overflow-auto rounded border border-slate-200 bg-slate-950">
          <pre class="min-h-[360px] p-3 text-xs text-slate-100"><code>
{#if content}
{#each highlighted as line, index}
<div class="grid grid-cols-[3rem_1fr] gap-3 whitespace-pre"><span class="select-none text-right text-slate-500">{index + 1}</span><span>{#each line as token}<span class={token.className}>{token.text}</span>{/each}</span></div>
{/each}
{:else}
<span class="text-slate-400">No file selected.</span>
{/if}
          </code></pre>
        </div>
      </div>

      {#if compareSnapshot !== null}
        <div class="rounded border border-slate-300 bg-white/70 p-3">
          <h2 class="text-sm font-semibold text-slate-900">Diff</h2>
          <p class="mt-1 text-xs text-slate-600">{buildSnapshotLabel(compareSnapshot)} → {buildSnapshotLabel(selectedSnapshot)}{selectedFile ? ` · ${selectedFile}` : ""}</p>
          <div class="mt-3 overflow-auto rounded border border-slate-200 bg-slate-950">
            <pre class="min-h-[240px] p-3 text-xs text-slate-100"><code>
{#if selectedFile}
{#each diffRows as row}
<div class={`grid grid-cols-[3rem_3rem_1.5rem_1fr] gap-3 whitespace-pre ${row.type === "add" ? "diff-add" : row.type === "remove" ? "diff-remove" : "diff-context"}`}>
  <span class="select-none text-right text-slate-500">{row.beforeLine}</span>
  <span class="select-none text-right text-slate-500">{row.afterLine}</span>
  <span>{row.type === "add" ? "+" : row.type === "remove" ? "-" : " "}</span>
  <span>{row.type === "add" ? row.after : row.type === "remove" ? row.before : row.after}</span>
</div>
{/each}
{:else}
<span class="text-slate-400">Select a file to compare snapshots.</span>
{/if}
            </code></pre>
          </div>
        </div>
      {/if}

      {#if selectedIteration}
        <div class="rounded border border-slate-300 bg-white/70 p-3">
          <h2 class="text-sm font-semibold text-slate-900">Iteration {selectedSnapshot} Artifact</h2>
          <pre class="mt-2 overflow-auto text-xs">{JSON.stringify(selectedIteration, null, 2)}</pre>
        </div>
      {/if}
    </section>
  </div>
{/if}

<style>
  .token-comment { color: #7dd3fc; }
  .token-string { color: #f9a8d4; }
  .token-keyword { color: #fcd34d; font-weight: 600; }
  .token-number { color: #fdba74; }
  .token-boolean { color: #86efac; font-weight: 600; }
  .token-ident { color: #e2e8f0; }
  .token-function { color: #c4b5fd; }
  .token-attribute { color: #93c5fd; }
  .token-interpolation { color: #fca5a5; }
  .token-heredoc { color: #f9a8d4; font-style: italic; }
  .token-punct { color: #94a3b8; }
  .token-space { color: inherit; }
  .diff-add { background: rgba(34, 197, 94, 0.12); }
  .diff-remove { background: rgba(248, 113, 113, 0.12); }
  .diff-context { background: transparent; }
</style>
