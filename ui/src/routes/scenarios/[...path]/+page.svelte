<script lang="ts">
  import { afterNavigate } from "$app/navigation";
  import { onDestroy } from "svelte";
  import { page } from "$app/stores";
  import { api } from "$lib/api";
  import { connectWS } from "$lib/ws";
  import {
    acceptProgressEvent,
    deployConfirmation,
    deployWarnings,
    teardownOutcome
  } from "$lib/deployments-view.js";
  import { modeSummary, normalizeRunOptions } from "$lib/scenario-run.js";
  import type { DeployPreview, ScenarioLayer3StatusResponse, ScenarioRunModeResponse } from "$lib/types";

  let scenarioPath = "";

  // Every async response on this page belongs to a navigation, and this
  // is how one is identified.
  //
  // Comparing `scenarioPath` is not enough: A → B → A leaves the path
  // equal to what an in-flight request for the FIRST A captured, so a
  // response that is genuinely stale passes the check. The counter is
  // monotonic, so it cannot collide that way.
  //
  // It exists because the alternative was found six times in four review
  // passes: a confirmation describing one scenario while the page had
  // moved to another, an in-flight preview reopening a dialog, a deploy
  // result landing on the wrong page, and route loads resolving out of
  // order. Guarding each occurrence produced the next one. This guards
  // the shape.
  let navigation = 0;

  /** current reports whether a response still belongs to the page. */
  const current = (token: number) => token === navigation;
  let detail: any = null;
  let rawYAML = "";
  let status = "";
  let running = false;
  let runMode: ScenarioRunModeResponse | null = null;
  let layer3Status: ScenarioLayer3StatusResponse | null = null;
  let runModeError = "";
  let layer3Error = "";
  let clean = false;
  let noDestroy = false;
  // Reflects what the SERVER decided at start time. This page reports it;
  // it cannot change it (ADR-0026).
  $: layer3Enabled = layer3Status?.server_allows_layer3 === true;
  let validationErrors: { path: string; message: string }[] = [];
  let validationState: "idle" | "checking" | "valid" | "invalid" = "idle";
  let validationTimer: ReturnType<typeof setTimeout> | null = null;
  let validationVersion = 0;

  async function runValidation(yaml: string) {
    if (!yaml.trim()) {
      validationState = "idle";
      validationErrors = [];
      return;
    }
    const myVersion = ++validationVersion;
    validationState = "checking";
    try {
      const resp = await api.validateScenarioYAML(yaml);
      if (myVersion !== validationVersion) return;
      validationErrors = resp.errors || [];
      validationState = resp.valid ? "valid" : "invalid";
    } catch (err) {
      if (myVersion !== validationVersion) return;
      validationState = "invalid";
      validationErrors = [{ path: "", message: err instanceof Error ? err.message : "Validation request failed" }];
    }
  }

  function scheduleValidation(yaml: string) {
    if (validationTimer) clearTimeout(validationTimer);
    validationTimer = setTimeout(() => runValidation(yaml), 500);
  }

  $: if (rawYAML !== undefined) scheduleValidation(rawYAML);

  // Clear the debounce timer on destroy so navigating away during the
  // 500ms window doesn't fire a stale validation against a torn-down
  // component (the validationVersion guard is per-instance).
  onDestroy(() => {
    if (validationTimer) clearTimeout(validationTimer);
    // Leaving the page must not leave a socket open. The deploy itself
    // is unaffected: it is detached from the request on the server, so
    // it finishes whether or not anybody is watching.
    resetDeployState();
  });

  $: scenarioPath = ($page.params.path || "").toString();
  $: runModeCard = modeSummary(runMode);

  const CLOUD_LABELS: Record<string, string> = {
    scaleway: "Scaleway",
    gcp: "GCP",
    aws: "AWS",
    genesys: "Genesys"
  };

  $: detailCloud = (detail?.cloud || "").toLowerCase();
  $: cloudLabel = CLOUD_LABELS[detailCloud] || (detailCloud ? detailCloud.toUpperCase() : "Unknown");
  $: layer3CloudLabel = CLOUD_LABELS[detailCloud] || "Cloud";

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
    const token = navigation;
    const loaded = await api.getScenario(scenarioPath);
    if (!current(token)) return;
    detail = loaded;
    rawYAML = loaded.raw_yaml;
  }

  async function loadRunMode() {
    if (!scenarioPath) return;
    const token = navigation;
    runModeError = "";
    try {
      const loaded = await api.getScenarioRunMode(scenarioPath);
      if (!current(token)) return;
      runMode = loaded;
    } catch (err) {
      if (!current(token)) return;
      runMode = null;
      runModeError = err instanceof Error ? err.message : "Run mode detection failed";
    }
  }

  // Deploy is a DISTINCT verb from run (ADR-0027 section 4). S153 split
  // them so that keeping infrastructure could not be reached by accident
  // from the verb that proves a change is safe, and merging them into
  // one "go" button here would undo that.
  let preview: DeployPreview | null = null;
  let previewError = "";
  let confirmingDeploy = false;
  // Only one preview may be in flight. Two clicks on Deploy left two
  // responses racing, and an older one could reopen a dialog the reader
  // had already dismissed.
  //
  // Disabling the button removes that state rather than guarding it,
  // which is the move pass 126 argued for and pass 127 had to learn
  // twice. It also gives the reader feedback that the click landed.
  let previewing = false;

  // A deploy runs for minutes, and minutes of silence reads as broken:
  // a reader cannot tell a long apply from a hung one, and the
  // difference matters when the thing running is creating billable
  // infrastructure (S163).
  //
  // Every line names the scenario it belongs to, so lines from a deploy
  // the reader has navigated away from are DISCARDED here rather than
  // appended under the wrong heading.
  let deployProgress: string[] = [];
  let disconnectWS: (() => void) | undefined;
  // Whether we are actually receiving. A dropped socket and "nothing has
  // happened yet" both produce an empty log, and rendering them the same
  // way tells the reader an apply is quiet when the truth is that it is
  // UNOBSERVED -- the same falsehood the estate page exists to avoid.
  let streamConnected = false;

  function onSocketMessage(msg: unknown) {
    if (!acceptProgressEvent(msg, deployingScenario)) return;
    const line = (msg as { data: { line: string } }).data.line;
    deployProgress = [...deployProgress, line];
  }

  // The scenario whose progress this page is currently showing. Held
  // separately from `preview` because `preview` is cleared on
  // navigation while a deploy keeps running.
  let deployingScenario = "";
  // Monotonic, so a finishing deploy can tell whether it is still the
  // one this page is showing.
  let deployGeneration = 0;

  /**
   * resetDeployState puts this page back to knowing nothing about a
   * deploy.
   *
   * ONE function, called from both navigation and destroy, because the
   * reset used to be a hand-written list in `afterNavigate` -- and the
   * moment S163 added stream state, the list did not grow with it. The
   * websocket, `deployingScenario` and `deployProgress` survived a
   * client-side route change, so progress for the previous scenario
   * kept rendering under the new one.
   *
   * `onDestroy` does not cover that: SvelteKit REUSES this `[...path]`
   * component across scenario routes, so leaving a scenario page for
   * another one destroys nothing.
   *
   * `deploying` is deliberately NOT reset here. It is the only thing
   * that stops a second deploy being started, and the apply is detached
   * from the request on the server -- so clearing it on navigation let a
   * reader move away, come back, find the button enabled, and start a
   * SECOND apply while the first was still creating billable
   * infrastructure. Two run-owned projects and two sets of resources for
   * one scenario, from a page reporting that nothing was running.
   *
   * That is the exact harm ADR-0027's streaming amendment names -- "a
   * reader who cannot tell a long apply from a hung one will do one of
   * two harmful things: kill it, or start another" -- reachable through
   * the code added to prevent it.
   *
   * The deploy itself is untouched either way. It finishes whether or
   * not this page is watching.
   */
  function resetDeployState() {
    disconnectWS?.();
    disconnectWS = undefined;
    streamConnected = false;
    deployingScenario = "";
    deployProgress = [];
    confirmingDeploy = false;
    preview = null;
    previewError = "";
    deployOutcomeMessage = "";
  }
  let deploying = false;
  let deployOutcomeMessage = "";
  let deployOk = false;

  async function openDeployConfirmation() {
    previewError = "";
    deployOutcomeMessage = "";

    // A preview takes a round trip and the reader can navigate during
    // it, so the response is discarded if it arrives after the page has
    // moved on.
    const token = navigation;
    const requestedName = detail?.name;
    if (!requestedName || previewing) return;

    previewing = true;
    try {
      const fetched = await api.getDeployPreview(requestedName);
      if (!current(token)) return;
      preview = fetched;
      confirmingDeploy = true;
    } catch (err) {
      if (!current(token)) return;
      preview = null;
      previewError = err instanceof Error ? err.message : "Could not read what this would deploy";
    } finally {
      previewing = false;
    }
  }

  async function confirmDeploy() {
    // The scenario from the PREVIEW, never the one the page currently
    // shows. This component is reused across scenario routes, so an
    // open confirmation can outlive the page it was opened on -- and
    // posting `detail.name` would let somebody read scenario A's cost,
    // lifetime and blast radius and create scenario B's infrastructure.
    //
    // A confirmation that describes one thing and does another is worse
    // than no confirmation at all: it converts a careful person into a
    // confident one.
    const target = preview?.scenario;
    if (!target) return;

    deploying = true;
    deployingScenario = target;
    // Which deploy this call owns. An earlier deploy finishing must not
    // clear a LATER one's state: without this, deploying A, navigating
    // away, deploying B, and then A's request resolving first would
    // freeze B's log mid-stream, unlock the button, and put a green
    // success message belonging to A directly beneath it.
    const owned = ++deployGeneration;
    deployProgress = [];
    if (!disconnectWS) {
      disconnectWS = connectWS(onSocketMessage, (connected) => (streamConnected = connected));
    }
    try {
      const result = await api.deployScenario(target);
      const outcome = teardownOutcome(result);
      // The outcome is always shown -- it names its scenario, so it is
      // true wherever it lands -- but it must not overwrite a NEWER
      // deploy's outcome.
      if (owned !== deployGeneration) return;
      deployOk = outcome.ok;
      // NAMED, always. An apply takes minutes and the reader can
      // navigate during it, so this message can land on a different
      // scenario's page -- and an unattributed "Deployed." there is a
      // claim about the wrong thing.
      //
      // Attributed rather than discarded: the deploy really did create
      // infrastructure, and throwing the news away because the reader
      // moved is the worse of the two failures.
      deployOutcomeMessage = outcome.ok
        ? `${target}: deployed. It is listed on the Deployments page until its TTL expires.`
        : `${target}: ${outcome.message}`;
    } catch (err) {
      if (owned !== deployGeneration) return;
      deployOk = false;
      deployOutcomeMessage =
        err instanceof Error
          ? `${target}: deploy could not be completed: ${err.message}`
          : `${target}: deploy failed.`;
    } finally {
      if (owned === deployGeneration) {
        deploying = false;
        confirmingDeploy = false;
      }
      // `deployingScenario` is NOT cleared here.
      //
      // The last progress line -- "Deployed as dep-…", the id an
      // operator needs for `live teardown` -- is broadcast before the
      // HTTP response but delivered by a separate goroutine on a
      // separate connection. It routinely arrives after this promise
      // resolves, and clearing the subject synchronously made the
      // filter discard exactly the line that mattered.
      //
      // It is cleared on navigation instead, which is when the page
      // genuinely stops being about this deploy.
    }
  }

  async function loadLayer3Status() {
    if (!scenarioPath) return;
    const token = navigation;
    layer3Error = "";
    try {
      const loaded = await api.getScenarioLayer3Status(scenarioPath);
      if (!current(token)) return;
      layer3Status = loaded;
    } catch (err) {
      if (!current(token)) return;
      layer3Status = null;
      layer3Error = err instanceof Error ? err.message : "Layer 3 status lookup failed";
    }
  }

  async function saveScenario() {
    status = "";
    try {
      await api.putScenario(scenarioPath, rawYAML);
      status = "Saved";
      await loadDetail();
      await loadRunMode();
      await loadLayer3Status();
    } catch (err) {
      status = err instanceof Error ? err.message : "Save failed";
    }
  }

  async function runScenario() {
    if (!detail?.name || running) return;
    running = true;
    status = "Starting run...";
    try {
      const resp = await api.startRun(detail.name, normalizeRunOptions({ clean, no_destroy: noDestroy }));
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

  // Reload data on every navigation (including client-side), since
  // SvelteKit reuses the component for [...path] route changes.
  // afterNavigate fires on both initial load and subsequent navigations.
  afterNavigate(() => {
    // Every response in flight now belongs to a page that no longer
    // exists.
    navigation += 1;
    scenarioPath = ($page.params.path || "").toString();
    // Belt and braces with confirmDeploy reading preview.scenario: a
    // confirmation describing the page you just left must not still be
    // on screen, even though accepting it would now be harmless.
    // Leaving it visible invites the reader to trust a dialog about
    // something they are no longer looking at.
    resetDeployState();
    // `detail` is cleared too, and that is the STRUCTURAL half of this.
    //
    // The whole page is inside `{#if detail}`, so clearing it means
    // nothing is rendered until the new scenario loads -- including the
    // Deploy button. Without this there is a window after the route
    // changes where `detail` still holds the PREVIOUS scenario, and
    // clicking Deploy in it previews and deploys that one from a URL
    // that says otherwise.
    //
    // The cost is a blink of empty page during navigation. That is a
    // fair price for making it impossible to act on a scenario the
    // address bar no longer names.
    detail = null;
    loadDetail();
    loadRunMode();
    loadLayer3Status();
  });
</script>

{#if detail}
  <div class="flex flex-wrap items-center gap-3">
    <h1 class="text-2xl font-bold text-slate-900">{detail.name}</h1>
    <span
      class="rounded-full bg-indigo-100 px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-indigo-900"
      data-testid="scenario-cloud-badge"
    >
      {cloudLabel}
    </span>
  </div>
  <p class="mt-2 text-slate-700">{detail.description}</p>
  <div class="mt-4 rounded border border-slate-300 bg-white/80 p-4">
    <div class="flex items-start justify-between gap-4">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Next Run Mode</p>
        <p class="mt-2 text-lg font-semibold text-slate-900">{runModeCard.title}</p>
        <p class="mt-1 text-sm text-slate-600">{runModeCard.detail}</p>
      </div>
      <span
        class={`rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] ${
          layer3Enabled
            ? "bg-sky-100 text-sky-900"
            : runModeCard.tone === "incremental"
            ? "bg-emerald-100 text-emerald-900"
            : runModeCard.tone === "clean"
              ? "bg-amber-100 text-amber-900"
              : "bg-slate-200 text-slate-700"
        }`}
      >
        {layer3Enabled ? "mock + real" : runMode?.mode || "unknown"}
      </span>
    </div>
    <!-- M99: this card must render the SAME NUMBER OF ELEMENTS whether or
         not the run-mode API answered. It previously rendered one grid
         tile when the API was unreachable and three when it responded,
         plus a conditional error line — so page height, and therefore
         the visual-regression baselines, depended on whether mockway
         happened to be running. Masking cannot fix that: masks hide
         content but do not constrain layout height.

         Every tile is always present (value "unavailable" when unknown)
         and `truncate` keeps each to exactly one line regardless of
         content length. -->
    <div class="mt-4 grid gap-2 text-xs text-slate-600 md:grid-cols-3">
      <div class="truncate rounded bg-slate-100 px-3 py-2" data-testid="scenario-mock-status">
        {#if runMode}
          {runMode.mock_provider || "mockway"} state: {runMode.has_mock_resources ? "yes" : "no"}
        {:else}
          {detailCloud === "gcp" ? "fakegcp" : "mockway"} state: unavailable
        {/if}
      </div>
      <div class="truncate rounded bg-slate-100 px-3 py-2" data-testid="scenario-tfstate-status">
        terraform.tfstate: {runMode ? (runMode.has_tfstate ? "yes" : "no") : "unavailable"}
      </div>
      <div class="truncate rounded bg-slate-100 px-3 py-2" data-testid="scenario-previous-run-status">
        Previous success: {runMode ? (runMode.has_previous_successful_run ? "yes" : "no") : "unavailable"}
      </div>
    </div>
    <!-- Always rendered, always one line: a conditional error paragraph
         is another height variance. Full text on hover when truncated. -->
    <p
      class="mt-3 truncate text-sm text-red-700"
      data-testid="scenario-run-mode-error"
      title={runModeError}
    >
      {runModeError || "\u00a0"}
    </p>
    <div class="mt-4 rounded border border-slate-200 bg-slate-50 px-3 py-3 text-xs text-slate-700">
      <div class="flex flex-wrap items-center gap-3">
        <!-- Deliberately NOT a checkbox. Real-cloud apply is decided when
             the server starts, by the person in the shell that holds the
             credentials, and a control here would imply this page can
             change it. It reports instead. -->
        <span
          data-testid="scenario-layer3-label"
          class={`rounded border px-3 py-2 text-xs font-semibold ${
            layer3Enabled
              ? "border-sky-300 bg-sky-50 text-sky-900"
              : "border-slate-300 bg-white text-slate-800"
          }`}
        >
          Layer 3 (Real {layer3CloudLabel}): {layer3Enabled ? "enabled by this server" : "off"}
        </span>
        <span class={`rounded-full px-2 py-1 font-semibold uppercase tracking-[0.16em] ${layer3Status?.ready ? "bg-emerald-100 text-emerald-900" : "bg-rose-100 text-rose-900"}`}>
          {layer3Status?.ready ? "credentials ready" : "credentials missing"}
        </span>
      </div>
      <!-- The API's `detail` already spells out the missing credentials
           ("Missing SCW_ACCESS_KEY, SCW_SECRET_KEY" — see
           internal/api/handlers_scenarios.go), so rendering
           missing_credentials again duplicated the same line verbatim. -->
      <p class="mt-2">{layer3Status?.detail || "Layer 3 status unavailable."}</p>
      {#if layer3Status && !layer3Enabled}
        <p class="mt-2" data-testid="scenario-layer3-how-to-enable">
          Runs from this UI will not touch real infrastructure. To allow it,
          restart the server with <code>infrafactory ui --allow-layer3</code>.
        </p>
      {/if}
      {#if layer3Error}
        <p class="mt-2 text-red-700">{layer3Error}</p>
      {/if}
    </div>
  </div>
  <div class="mt-4 flex flex-wrap items-center gap-3">
    <label class="flex items-center gap-2 rounded border border-slate-300 bg-white px-3 py-2 text-xs text-slate-800">
      <input type="checkbox" bind:checked={noDestroy} disabled={clean} />
      <span>Keep state (`--no-destroy`)</span>
    </label>
    <label class="flex items-center gap-2 rounded border border-slate-300 bg-white px-3 py-2 text-xs text-slate-800">
      <input type="checkbox" bind:checked={clean} disabled={noDestroy} />
      <span>Force clean (`--clean`)</span>
    </label>
  </div>
  <div class="mt-4 flex gap-2">
    <button class="rounded bg-slate-900 px-3 py-1.5 text-xs text-white disabled:opacity-60" on:click={runScenario} disabled={running}>
      {running ? "Starting..." : "Run"}
    </button>
    <button class="rounded border border-slate-400 px-3 py-1.5 text-xs text-slate-900" on:click={saveScenario}>Save</button>
    <!-- Deliberately a separate button, and deliberately not next to Run
         in colour or weight. `run` proves a change is safe; `deploy`
         keeps it. Merging them would undo the split S153 made. -->
    <button
      class="rounded border border-sky-500 px-3 py-1.5 text-xs font-semibold text-sky-900 disabled:opacity-50"
      data-testid="scenario-deploy"
      on:click={openDeployConfirmation}
      disabled={deploying || previewing}
      >{deploying
        ? `Deploying ${deployingScenario || "…"}`
        : previewing
          ? "Checking…"
          : "Deploy…"}</button
    >
  </div>

  {#if previewError}
    <p class="mt-3 text-sm text-rose-800" data-testid="deploy-preview-error">{previewError}</p>
  {/if}

  {#if confirmingDeploy && preview}
    <div
      class="mt-3 rounded border border-sky-300 bg-sky-50 px-4 py-3 text-sm"
      data-testid="deploy-confirm"
    >
      <p class="font-semibold text-slate-900">Deploy {preview.scenario}?</p>

      <ul class="mt-2 list-disc space-y-1 pl-5 text-slate-800">
        {#each deployConfirmation(preview) as line}
          <li>{line}</li>
        {/each}
      </ul>

      {#each deployWarnings(preview) as warning}
        <p class="mt-2 font-semibold text-rose-900" data-testid="deploy-warning">{warning}</p>
      {/each}

      {#if !preview.deployable}
        <p class="mt-2 text-rose-900" data-testid="deploy-not-deployable">{preview.reason}</p>
      {:else if !preview.deploy_allowed}
        <p class="mt-2 text-slate-700" data-testid="deploy-not-allowed">
          This server cannot deploy. Restart it with
          <code>infrafactory ui --allow-deploy</code>.
        </p>
      {/if}

      <div class="mt-3 flex gap-2">
        <button
          class="rounded bg-sky-700 px-3 py-1.5 text-xs font-semibold text-white disabled:opacity-40"
          data-testid="deploy-confirm-go"
          disabled={!preview.deployable || !preview.deploy_allowed || deploying}
          on:click={confirmDeploy}>Deploy and keep it running</button
        >
        <button
          class="rounded border border-slate-300 px-3 py-1.5 text-xs text-slate-700"
          data-testid="deploy-cancel"
          on:click={() => (confirmingDeploy = false)}>Cancel</button
        >
      </div>
    </div>
  {/if}

  <!-- Gated on `deployingScenario`, not on `deploying`.
       `deploying` is a bare boolean about no particular thing, so it
       survives navigation and would keep this panel open on a page that
       has nothing to do with the deploy still running. The
       subject-bearing state is the one to ask. -->
  {#if deployingScenario || deployProgress.length > 0}
    <div
      class="mt-3 rounded border border-slate-300 bg-slate-900 px-3 py-2 font-mono text-xs text-slate-100"
      data-testid="deploy-progress"
    >
      {#if deployProgress.length === 0 && !streamConnected}
        <!-- Not "Starting…": we are not receiving, so we do not know
             whether anything has happened. The apply is unaffected --
             it is detached from this page entirely. -->
        <p class="text-amber-300" data-testid="deploy-progress-disconnected">
          Not receiving progress — the apply is still running, but this page cannot see it.
        </p>
      {:else if deployProgress.length === 0}
        <p class="text-slate-400">Starting…</p>
      {:else}
        {#each deployProgress as line}
          <p>{line}</p>
        {/each}
      {/if}
    </div>
  {/if}

  {#if deployOutcomeMessage}
    <p
      class={`mt-3 text-sm ${deployOk ? "text-emerald-800" : "font-semibold text-rose-800"}`}
      data-testid="deploy-outcome"
    >
      {deployOutcomeMessage}
    </p>
  {/if}
  {#if status}<p class="mt-3 text-sm text-slate-700">{status}</p>{/if}
  <textarea
    class="mt-4 h-[460px] w-full rounded border border-slate-300 p-3 font-mono text-sm"
    data-testid="scenario-yaml"
    bind:value={rawYAML}
  ></textarea>
  <div class="mt-2" data-testid="scenario-validation">
    {#if validationState === "checking"}
      <p class="text-xs text-slate-500" data-testid="scenario-validation-checking">Validating…</p>
    {:else if validationState === "valid"}
      <p class="text-xs font-medium text-emerald-700" data-testid="scenario-validation-valid">Valid scenario.</p>
    {:else if validationState === "invalid"}
      <ul class="space-y-1 text-xs text-rose-700" data-testid="scenario-validation-errors">
        {#each validationErrors as err}
          <li><span class="font-mono">{err.path || "(root)"}</span>: {err.message}</li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}
