<script lang="ts">
  import { afterNavigate } from "$app/navigation";
  import { onDestroy, onMount } from "svelte";
  import { get } from "svelte/store";
  import { page } from "$app/stores";
  import { api, DeployError } from "$lib/api";
  import { connectWS } from "$lib/ws";
  import {
    deployConfirmation,
    deployWarnings,
    deployOutcome as toDeployOutcome
  } from "$lib/deployments-view.js";
  import {
    beginDeploy,
    deploys,
    retireDeploy,
    finishDeploy,
    refuseDeploy,
    isConnected,
    isRunning,
    useConnector,
    watch as watchDeploys,
    KEPT_OPENING_LINES
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
  let detailError = "";
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

  // The shared socket stays open while this page is mounted, and the
  // store keeps it open beyond that if a deploy this tab started is
  // still running -- so leaving and returning finds the log intact
  // rather than frozen.
  //
  // Nothing here asks the server what else might be deploying. An
  // earlier version did -- adoption, terminal-event recovery, reconnect
  // resynchronisation -- and three review rounds produced 36 findings,
  // almost all of them in that machinery. It was a browser mirror of
  // server state, and every bug was the mirror disagreeing with the
  // thing it mirrored.
  //
  // A reloaded page simply does not know, and says so.
  onMount(() => watchDeploys());

  onDestroy(() => {
    // Clear the debounce timer so navigating away during the 500ms
    // window does not fire a stale validation against a torn-down
    // component (the validationVersion guard is per-instance).
    if (validationTimer) clearTimeout(validationTimer);
    resetDeployState();
    // Also here, not only in afterNavigate.
    //
    // afterNavigate does not fire for a component being DESTROYED, so
    // leaving the scenarios section entirely -- the case the store's own
    // doc names -- left a finished deploy's success banner to reappear
    // on the next visit. A claim about infrastructure whose TTL may have
    // expired.
    retireOnLeave();
  });

  /**
   * retireOnLeave ends this scenario's deploy display when the reader
   * goes.
   *
   * "Retire", not "forget", because `retireDeploy` KEEPS what the
   * deploy has to report -- three names for one operation, one of them
   * asserting the opposite of what the delegate does, sent a reader
   * tracing a surviving report through two functions that both claimed
   * it had been forgotten.
   *
   * What goes is the BANNER, and only ever the banner. "The reader saw
   * it, so it can go" was once the rule for everything, and it is wrong
   * for a report: the failure message says "check the Deployments page
   * before starting another", the Deployments link sits directly
   * beneath the button, and following the instruction was what deleted
   * the project id the instruction was about.
   */
  function retireOnLeave() {
    if (detail?.name) retireDeploy(detail.name);
  }

  // Arriving drops a finished deploy that has nothing left to report.
  //
  // The leave-hooks cannot: a deploy still running when the reader left
  // finishes afterwards, and nothing was left to drop it -- so it lived
  // forever, greeting every later visit with "deployed. It is listed on
  // the Deployments page until its TTL expires." for infrastructure
  // whose TTL may long since have gone.
  //
  // Only a success. A FAILURE is not a stale claim, it is an unread
  // report: "it may have created resources that are still running",
  // carrying the project id that has to be removed by hand. A deploy
  // that fails before registration has no live record either, so this
  // banner is the ONLY place it is ever said. Dropping it because the
  // reader happened to look away is how the leak goes unnoticed --
  // which is the failure this whole arc exists to prevent, arriving
  // through a cleanup.
  //
  // Keyed on the navigation counter rather than on `detail`, because
  // `loadDetail` also runs after a save and a banner must not vanish
  // mid-visit.
  // Only what was ALREADY finished when the navigation began.
  //
  // `loadDetail` is a round trip, and a deploy started just before it
  // can END during that window -- a 423 lock refusal returns at once.
  // Retiring by "is it finished when detail arrives?" then deleted a
  // refusal before it had ever rendered, and the button reverted to
  // "Deploy…" as though the click had not landed. That is the defect
  // moving refusals into the store was supposed to close, reopened by
  // the hook that cleans them up.
  //
  // The snapshot is taken synchronously in `afterNavigate`, before any
  // response can arrive.
  let retirableOnArrival = new Set<string>();
  let arrivalHandled = -1;
  $: if (detail?.name && arrivalHandled !== navigation) {
    arrivalHandled = navigation;
    if (retirableOnArrival.has(detail.name)) retireDeploy(detail.name);
  }

  /** finishedNow names the deploys that have already ended. */
  function finishedNow(): Set<string> {
    const all = get(deploys);
    return new Set(
      Object.keys(all).filter((k) => k !== "__connected" && all[k] && !all[k].running)
    );
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

  /** sentence ends a message so the next one does not run into it. */
  function sentence(text: string): string {
    const trimmed = text.trim();
    return /[.!?]$/.test(trimmed) ? trimmed : `${trimmed}.`;
  }

  /**
   * attributed is the scenario prefix, applied at RENDER time.
   *
   * Not baked into the stored message. The layout renders reports under
   * a scenario heading of their own, so a prefixed message printed the
   * name twice; this slot has no heading, so it needs one. Deciding at
   * render lets each place ask for what it needs from one string.
   *
   * Skipped when the message already leads with the name. Anchored,
   * because a bare `includes` lets a scenario named `json` match
   * "invalid json body" and render unattributed -- the defect the
   * prefix exists to prevent, reintroduced by the check meant to stop
   * it doubling.
   *
   * Space OR colon. Only the space form is reachable today: this slot
   * renders successes and refusals, and the one refusal that names a
   * scenario formats it `"%s is already deploying"`. The colon arm is
   * for the next message that leads with `"<scenario>: …"` -- a shape
   * the deploy pipeline's own errors already use, and one that would
   * otherwise render the name twice on the screen this slice exists to
   * make trustworthy.
   */
  function attributed(scenario: string, message: string): string {
    if (message.startsWith(`${scenario} `) || message.startsWith(`${scenario}:`)) return message;
    return `${scenario}: ${message}`;
  }

  async function loadDetail() {
    // Cleared BEFORE the guard. Returning early on an empty path left a
    // previous scenario's failure on screen for a page that was never
    // asked about.
    detailError = "";
    if (!scenarioPath) return;
    const token = navigation;
    try {
      const loaded = await api.getScenario(scenarioPath);
      if (!current(token)) return;
      detail = loaded;
      rawYAML = loaded.raw_yaml;
    } catch (err) {
      // Its two siblings, loadRunMode and loadLayer3Status, both catch
      // and report. This one did not, and it is called unawaited from
      // afterNavigate -- which has just set `detail = null`, and the
      // whole page is inside `{#if detail}`. So a 500 or a dropped
      // connection rejected into nothing and left a blank screen with
      // no message, no retry, and no sign that anything had failed.
      if (!current(token)) return;
      // `detail` is cleared only if there was nothing there. A save
      // calls this to refresh, and nulling on a transient failure
      // unmounted the whole `{#if detail}` block -- the title, the
      // buttons, the "Saved" status and the textarea holding the
      // reader's YAML -- after a PUT that had actually succeeded.
      detailError = err instanceof Error ? err.message : "Could not read this scenario";
      if (!detail) return;
      status = detailError;
      detailError = "";
    }
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
  $: deployDropped = deployEntry?.dropped ?? 0;
  $: deploying = detail?.name ? isRunning($deploys, detail.name) : false;
  $: deployOutcome = deployEntry?.outcome ?? null;
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

    confirmingDeploy = false;

    // The entry is created BEFORE the POST, because streaming progress
    // during the apply is the entire point and the response does not
    // arrive for minutes.
    beginDeploy(target);

    try {
      finishDeploy(target, toDeployOutcome(await api.deployScenario(target)));
    } catch (err) {
      const message = err instanceof Error ? err.message : "The deploy failed.";

      // EVERY ending goes into the store, including a refusal.
      //
      // The refusal used to be a component variable, and that one
      // difference produced three defects: it rendered on whichever
      // scenario the reader had navigated to, it outlived the attempt
      // that caused it so a successful retry showed "already deploying"
      // and "deployed" together, and once it was guarded by a
      // navigation token a refusal arriving after any navigation
      // deleted the entry and said nothing at all -- the button
      // silently reverting to "Deploy…" as though the click had never
      // happened.
      //
      // An outcome is keyed by scenario, so it lands on the right page
      // and only that page, with no token and no clearing rule. Two
      // scopings of one request's two possible answers was the defect;
      // one scoping is the fix.
      //
      // Nothing is forgotten here. A refused entry used to be removed
      // so it could not adopt another tab's stream, and the socket
      // handler now ignores every entry that is not running -- which
      // closes that for finished deploys too, not just refused ones.
      //
      // A rejected promise does NOT mean nothing happened: the server
      // detaches the apply from the request, so a dropped connection
      // leaves it running and creating billable infrastructure. Only
      // `DeployError.startedNothing` is the server's own word that it
      // refused before anything ran.
      const startedNothing = err instanceof DeployError && err.startedNothing;
      if (startedNothing) {
        // The log goes with it. The entry was created before the POST,
        // so it was running -- and therefore collecting -- for the whole
        // round trip, while the reason for the refusal is that somebody
        // else's apply of this scenario holds the lock. Those lines are
        // theirs.
        // Attributed unless the server already did it. Only the lock
        // refusal names the scenario ("a bare refusal leaves a reader
        // wondering which of their tabs is responsible"); "invalid json
        // body", "method not allowed", the origin guard's message and
        // the no---allow-deploy 404 do not. Keying the skip off the
        // whole refusal class prefixed none of them, so four of the five
        // rendered unattributed in a slot shared with every scenario.
        refuseDeploy(target, { ok: false, mayHaveCreated: false, message });
        return;
      }
      finishDeploy(
        target,
        ({
          ok: false,
          mayHaveCreated: true,
          // Punctuated. Server error strings and `deploy failed: 502`
          // never end in a full stop, so the two sentences ran together.
          message: `${sentence(message)} The deploy may still be running on the server — check the Deployments page before starting another.`
        })
      );
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
    retirableOnArrival = finishedNow();
    // A FINISHED deploy's banner is dropped when the reader leaves the
    // scenario it belongs to. Without this it reappeared on every later
    // visit for the rest of the session, long after the TTL had expired
    // -- a success message for something that may no longer exist.
    retireOnLeave();
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

  <!-- This page only knows about a deploy IT started. After a reload it
       does not, and the estate page is the thing that does -- from the
       live store, which every finished deploy writes to whatever
       started it. Saying so is less convenient than mirroring server
       state here; mirroring it produced 36 review findings across three
       rounds.

       "Everything RECORDED" rather than "everything running", and the
       difference is real: the in-progress banner there comes from one
       process's in-memory lock, so a `infrafactory deploy` in a
       terminal, or an apply that was in flight when the server
       restarted, appears in neither the table nor the banner until it
       finishes and writes its record. -->
  <p class="mt-2 text-xs text-slate-500" data-testid="deploy-scope-note">
    This page shows deploys started from it. <a class="underline" href="/deployments">Deployments</a>
    lists every deployment that has been recorded.
  </p>

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
        {#each deployProgress as line, i}
          {#if i === KEPT_OPENING_LINES && deployDropped > 0}
            <!-- A truncated log with no marker reads as a whole one,
                 so its first visible line looks like the start of the
                 apply when it is not. -->
            <p class="text-amber-300" data-testid="deploy-progress-truncated">
              … {deployDropped} lines omitted …
            </p>
          {/if}
          <p>{line}</p>
        {/each}
      {/if}
    </div>
  {/if}

  <!-- Named, always. An apply takes minutes and the reader can navigate
       during it, so an unattributed "deployed." is a claim about the
       wrong thing. The store keys outcomes by scenario, so this one is
       this scenario's by construction. -->
  <!-- A REPORT is not repeated here -- the layout renders those, and
       printing it twice put the scenario name on screen three times --
       but the log must not simply stop with nothing said. A reader
       watching it needs a terminal line, and the full account is above
       the fold. -->
  {#if deployOutcome?.mayHaveCreated}
    <p class="mt-3 text-sm font-semibold text-rose-800" data-testid="deploy-outcome-pointer">
      This deploy did not finish cleanly. What it may have left behind is reported at the top of the
      page.
    </p>
  {/if}

  {#if deployOutcome && !deployOutcome.mayHaveCreated}
    <p
      class={`mt-3 text-sm ${deployOutcome.ok ? "text-emerald-800" : "font-semibold text-rose-800"}`}
      data-testid="deploy-outcome"
    >
      {attributed(detail.name, deployOutcome.message)}
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
{:else if detailError}
  <!-- Without this the page was simply blank: `detail` is null both
       while loading and after a failed load, and nothing distinguished
       them. An empty screen reads as "there is nothing here", which is
       the one thing it must not say when the truth is "we could not
       find out". -->
  <p class="text-sm font-semibold text-rose-800" data-testid="scenario-load-error">
    This scenario could not be read: {detailError}
  </p>

{/if}
