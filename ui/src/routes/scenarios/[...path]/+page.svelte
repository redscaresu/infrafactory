<script lang="ts">
  import { afterNavigate } from "$app/navigation";
  import { onDestroy, onMount } from "svelte";
  import { page } from "$app/stores";
  import { api } from "$lib/api";
  import { connectWS } from "$lib/ws";
  import {
    deployConfirmation,
    deployWarnings,
    teardownOutcome
  } from "$lib/deployments-view.js";
  import {
    adoptInFlight,
    beginDeploy,
    deploys,
    forgetDeploy,
    finishDeploy,
    isConnected,
    isRunning,
    useConnector,
    watch as watchDeploys
  } from "$lib/deploy-store.js";
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
  // The shared socket stays open while this page is mounted, and the
  // store keeps it open beyond that if a deploy is still running -- so
  // leaving and returning finds the log intact rather than frozen.
  onMount(() => {
    const fetchDeploying = async () => (await api.getDeployments())?.deploying;

    // A reload wipes the store, so ask the server what is running. The
    // refusal is server-side either way; this stops the reader being
    // shown a button that would be refused.
    void fetchDeploying()
      .then(adoptInFlight)
      .catch(() => {
        // Unreachable estate. The Deploy button stays enabled and the
        // server refuses if it must -- better than blocking deploys
        // because a listing call failed.
      });

    // An adopted deploy finishes on the `deploy_complete` event the
    // server broadcasts, so nothing here polls.
    return watchDeploys();
  });

  onDestroy(() => {
    if (validationTimer) clearTimeout(validationTimer);
    resetDeployState();
    // Also here, not only in afterNavigate.
    //
    // afterNavigate does not fire for a component being DESTROYED, so
    // leaving the scenarios section entirely -- the case the store's own
    // doc names -- left a finished deploy's success banner to reappear
    // on the next visit. A claim about infrastructure whose TTL may have
    // expired.
    forgetFinishedDeploy();
  });

  /** forgetFinishedDeploy drops a completed deploy's banner. */
  function forgetFinishedDeploy() {
    if (detail?.name && deployEntry && !deployEntry.running) {
      forgetDeploy(detail.name);
    }
  }

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
  // The in-flight state lives in a MODULE-LEVEL store, not here.
  //
  // This component is reused between scenarios and destroyed outright
  // when you leave the section, while a deploy runs for minutes on the
  // server. Holding the state here produced two review findings:
  // navigating away and back showed a real, billable apply as an
  // unlabelled disabled button with no log and no warning, and leaving
  // the section entirely let a second deploy be started.
  //
  // The REFUSAL is server-side -- `LiveDeployer` holds a per-scenario
  // lock and answers 423 -- so this store is advisory. An earlier
  // version of this comment said the server had no lock, which was
  // false from the moment it was written and is exactly the
  // false-explanation defect this work keeps finding.
  //
  // Everything below is scoped to the scenario ON SCREEN, so another
  // scenario's deploy never renders here and this scenario's deploy
  // renders whether or not this component was alive when it began.
  useConnector(connectWS);

  $: deployEntry = detail?.name ? $deploys[detail.name] : undefined;
  $: deployProgress = deployEntry?.progress ?? [];
  $: deploying = detail?.name ? isRunning($deploys, detail.name) : false;
  $: deployOutcome = deployEntry?.outcome ?? null;
  // An ADOPTED deploy was already running when this page loaded, so its
  // earlier output is unrecoverable -- the hub does not replay. Saying
  // "Starting…" would claim nothing has happened yet when minutes of it
  // has, which is the same falsehood the disconnected banner exists to
  // avoid one state over.
  $: deployAdopted = deployEntry?.adopted === true;
  // A dropped socket and "nothing has happened yet" both produce an
  // empty log, and rendering them the same way tells the reader an apply
  // is quiet when the truth is that it is UNOBSERVED.
  $: streamConnected = isConnected($deploys);

  /**
   * resetDeployState clears what belongs to THIS page's confirmation
   * dialog, and nothing else.
   *
   * It deliberately does not touch the deploy stream. That lives in the
   * store, survives navigation and component destruction, and is scoped
   * by scenario -- so a page showing B reads nothing for B while A keeps
   * applying, and coming back to A finds its log still there.
   *
   * The previous version cleared it here, which is what produced the
   * "running apply rendered as an unlabelled disabled button" and
   * "second deploy startable" findings.
   */
  function resetDeployState() {
    confirmingDeploy = false;
    preview = null;
    previewError = "";
  }

  async function openDeployConfirmation() {
    previewError = "";

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

    // The store owns everything about an in-flight deploy, so it
    // survives this component being reused or destroyed. It is keyed by
    // scenario, which is what the progress events carry.
    beginDeploy(target);
    confirmingDeploy = false;

    try {
      finishDeploy(target, teardownOutcome(await api.deployScenario(target)));
    } catch (err) {
      finishDeploy(target, {
        ok: false,
        message:
          err instanceof Error ? `deploy could not be completed: ${err.message}` : "deploy failed."
      });
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
    // A FINISHED deploy's banner is dropped when the reader leaves the
    // scenario it belongs to. Without this it reappeared on every later
    // visit for the rest of the session, long after the TTL had expired
    // -- a success message for something that may no longer exist.
    forgetFinishedDeploy();
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
        ? `Deploying ${detail?.name ?? "…"}`
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

  <!-- Read from the store, scoped to the scenario on screen. Another
       scenario's deploy never renders here, and this scenario's deploy
       renders whether or not this component was alive when it began. -->
  {#if deploying || deployProgress.length > 0}
    <div
      class="mt-3 rounded border border-slate-300 bg-slate-900 px-3 py-2 font-mono text-xs text-slate-100"
      data-testid="deploy-progress"
    >
      {#if deployProgress.length === 0 && deployAdopted && streamConnected}
        <!-- Adopted: it was already running when this page loaded, so
             its earlier output is gone. Saying "Starting…" would claim
             nothing has happened yet. -->
        <p class="text-slate-300" data-testid="deploy-progress-adopted">
          Already running when this page loaded — earlier output is not recoverable. New lines
          will appear here.
        </p>
      {:else if deployProgress.length === 0 && !streamConnected}
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

  <!-- Named, always. An apply takes minutes and the reader can navigate
       during it, so an unattributed "deployed." is a claim about the
       wrong thing. The store keys outcomes by scenario, so this one is
       this scenario's by construction. -->
  {#if deployOutcome}
    <p
      class={`mt-3 text-sm ${deployOutcome.ok ? "text-emerald-800" : "font-semibold text-rose-800"}`}
      data-testid="deploy-outcome"
    >
      {detail?.name}: {deployOutcome.ok
        ? "deployed. It is listed on the Deployments page until its TTL expires."
        : deployOutcome.message}
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
