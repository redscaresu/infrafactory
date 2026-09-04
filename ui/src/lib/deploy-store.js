import { writable, get } from "svelte/store";
import { isProgressEvent } from "./deployments-view.js";

/**
 * The deploy this tab started, if any.
 *
 * # What this deliberately does NOT do
 *
 * It does not try to know about deploys started anywhere else — another
 * tab, the CLI, or this page before a reload. An earlier version did,
 * and three review rounds produced 36 findings, almost all of them in
 * that machinery: adoption, terminal-event recovery, reconnect
 * resynchronisation, ownership races between a listing and the tab that
 * issued the POST.
 *
 * None of it was about applying to a real cloud rather than a mock —
 * `tofu apply` cannot tell the difference. It was a browser state
 * machine mirroring server state, and every bug came from the mirror
 * disagreeing with the thing it mirrored.
 *
 * **The estate page already answers "what has been deployed", from the
 * server, and survives a reload by construction.** A page that has lost
 * track says so and points there. That is less convenient, and it
 * cannot disagree with the live store the way a mirror can.
 *
 * It is not omniscient, and the scope note on the scenario page says
 * so: a deploy still applying is held in ONE process's in-memory lock,
 * so a CLI deploy, or one that was in flight when the server restarted,
 * is invisible until it finishes and writes its record. What the estate
 * page cannot do is be WRONG about a record that exists.
 *
 * What survives here is only what a tab genuinely knows because it did
 * it: the deploy it started, its log, and how it ended. That outlives
 * navigation between scenarios, which is the one case worth keeping,
 * because the reader is watching something they started moments ago.
 */
export const deploys = writable({});

/**
 * Deploy reports, in their OWN store.
 *
 * A report says infrastructure may exist with no record of it. It
 * changes twice a session — when a deploy ends badly, and when somebody
 * dismisses it — while `deploys` is written once per progress line,
 * thousands of times during a real apply. Reading reports out of
 * `deploys` subscribed the root layout, mounted on every route, to all
 * of that: a fresh array and a keyed re-diff per line, for data that had
 * not changed.
 *
 * Separating them removes the coupling rather than optimising it. Shape:
 * `{ [scenario]: [{ id, message, opening }] }`.
 */
export const reports = writable({});

let socket;
let watchers = 0;
let generation = 0;
let reportSeq = 0;

/** nextReportID mints an identity that does not move when a sibling goes. */
function nextReportID() {
  reportSeq += 1;
  return `report-${reportSeq}`;
}

// Matches the Live Run page's cap. Deliberately generous: a deploy log
// is read to find out what a long apply is doing, and truncating it
// hard would lose the stage boundaries a reader navigates by.
const MAX_PROGRESS_LINES = 999;

// The opening lines are kept whatever else is dropped: `deploy` writes
// the scenario, its ref, its TTL and its workdir first, and they are the
// only place those appear when the request never returns an
// ActionResult -- a dropped connection mid-apply leaves the generic
// "may still be running" message and nothing else. Truncating from the
// front discarded exactly the identifying half of the log.
export const KEPT_OPENING_LINES = 20;

// How far past the cap the log may grow before it is trimmed. Trimming
// costs a full array rebuild, so doing it once per slack rather than
// once per line makes it amortised.
const TRIM_SLACK = 100;

// Injected rather than imported: `ws.ts` is TypeScript and the unit
// tests run under `node --test`, which resolves plain JS only.
let connect = () => () => {};

/** useConnector installs the websocket factory. */
export function useConnector(fn) {
  connect = fn;
}

function ensureSocket() {
  if (socket) return;
  // Every connection gets a generation, and both its callbacks check
  // theirs before touching the store.
  //
  // Disposing a socket does not silence it: the close handshake is
  // asynchronous, so an old connection's `onclose` can fire AFTER a
  // replacement has opened. It then wrote `__connected: false` over a
  // healthy socket, nothing re-fired `onopen`, and every deploy for the
  // rest of the session rendered "Not receiving progress -- the apply
  // is still running, but this page cannot see it." over a stream that
  // was working.
  generation += 1;
  const mine = generation;
  socket = connect(
    (msg) => {
      if (mine !== generation) return;
      // The event names its own subject, so this is a lookup rather
      // than a scan. Asking every key "is this yours?" needed a skip for
      // the `__connected` sentinel; a keyed read makes it unreachable
      // by construction instead, because the sentinel is a boolean and
      // has no `running`.
      if (!isProgressEvent(msg)) return;
      const scenario = msg.data.subject;
      if (!scenario) return;

      // Checked BEFORE `update`, because `writable.update` notifies
      // every subscriber whether or not the value changed. Returning
      // `all` unchanged from inside it still re-ran the scenario page's
      // derivations and the layout's `pendingReports` -- thousands of
      // times, for a foreign apply whose lines this store discards.
      if (!get(deploys)[scenario]?.running) return;

      deploys.update((all) => {
        const entry = all[scenario];
        // Only a deploy this tab is still WATCHING absorbs lines.
        //
        // A finished entry stays until the reader navigates away, and
        // the stream is keyed by scenario -- so without this, a second
        // deploy of the same scenario from anywhere else (another tab,
        // the CLI) appended into it, and the page rendered a live,
        // growing log of an apply it did not start underneath a
        // completed-outcome banner. That is the adoption this slice
        // removed, arriving through the door its own comments name.
        if (!entry?.running) return all;
        return { ...all, [scenario]: appendProgress(entry, msg.data.line) };
      });
    },
    (connected) => {
      if (mine !== generation) return;
      deploys.update((all) => ({ ...all, __connected: connected }));
    }
  );
}

function releaseSocket() {
  // Kept open while a deploy this tab started is still running, even if
  // nobody is looking at its page — otherwise returning to it finds a
  // frozen log.
  const current = get(deploys);
  const running = Object.keys(current).some((k) => k !== "__connected" && current[k]?.running);
  if (running || watchers > 0) return;
  socket?.();
  socket = undefined;
}

/** watch keeps the shared socket alive while a page is mounted. */
export function watch() {
  watchers += 1;
  ensureSocket();
  return () => {
    watchers -= 1;
    releaseSocket();
  };
}

/**
 * appendProgress adds one line, dropping from the MIDDLE when capped.
 *
 * Capped at all because a real Scaleway apply emits thousands of lines
 * over minutes, and every one of them copies the whole array, notifies
 * the store and re-renders the whole `{#each}` -- so an uncapped log
 * grows quadratically and the tab stalls during the apply it exists to
 * make watchable.
 *
 * One array, so a short log is exactly what arrived. When it grows past
 * the cap what goes is the MIDDLE: the opening names the scenario, its
 * ref, its TTL and its workdir, and the tail is the last thing that
 * happened. A plain `slice(-N)` keeps only the second. `dropped` counts
 * what went so the panel can say so, rather than presenting a truncated
 * log as a whole one.
 */
function appendProgress(entry, line) {
  const progress = [...entry.progress, line];
  if (progress.length <= MAX_PROGRESS_LINES + TRIM_SLACK) return { ...entry, progress };
  // Trimmed in BATCHES, down to the cap, so the rebuild happens once
  // per TRIM_SLACK lines rather than on every line past the cap. The
  // cap exists for responsiveness during a long apply, and trimming a
  // thousand-element array on each of several thousand lines only
  // bounds the stall it was meant to remove.
  const keptTail = MAX_PROGRESS_LINES - KEPT_OPENING_LINES;
  return {
    ...entry,
    progress: [...progress.slice(0, KEPT_OPENING_LINES), ...progress.slice(-keptTail)],
    dropped: (entry.dropped || 0) + (progress.length - MAX_PROGRESS_LINES)
  };
}

export function beginDeploy(scenario) {
  // Refused if one is already running, and the refusal lives HERE
  // rather than only in the button's `disabled`. Returns whether the
  // deploy may proceed -- the caller MUST honour it, because the store
  // can decline to record a second start but cannot stop the POST, and
  // the 423 that came back was then applied to the FIRST deploy.
  //
  // Starting over a live apply reset its log and its outcome; the
  // second POST was then refused with 423, `refuseDeploy` marked the
  // entry finished and cleared the log, and the socket handler began
  // discarding the ORIGINAL apply's lines because the entry was no
  // longer running -- a deploy still creating billable infrastructure
  // with its log and its ending both lost. The store is the thing that
  // outlives the page, so the rule belongs in it.
  if (get(deploys)[scenario]?.running) return false;
  ensureSocket();
  deploys.update((all) => {
    // Reports are untouched: they live in their own store, so a retry
    // cannot delete what the last attempt leaked, and two failed
    // attempts accumulate two of them.
    return {
      ...all,
      [scenario]: { running: true, progress: [], dropped: 0, outcome: null }
    };
  });
  return true;
}

/**
 * fileReport records what a deploy may have left behind.
 *
 * Called BESIDE `deploys.update`, never inside its updater. An updater
 * is contracted to be a pure value producer; writing another store from
 * within one notifies that store's subscribers while `deploys` still
 * holds the previous value -- so the layout rendered a new report
 * against an entry that was still marked running -- and any retry of
 * the updater would file the report twice.
 */
function fileReport(scenario, outcome, progress) {
  if (!outcome?.mayHaveCreated) return;
  reports.update((all) => ({
    ...all,
    [scenario]: [
      ...(all[scenario] ?? []),
      {
        // A stable ID, minted here. Dismissing used to name a POSITION,
        // and positions move: two clicks landing before a re-render
        // deleted two different reports, the second of them a leak the
        // operator had never read.
        id: nextReportID(),
        message: outcome.message,
        // A report carries its own OPENING LINES. When the request
        // never returns an ActionResult -- a dropped connection
        // mid-apply -- the message is generic and the head of the log
        // is the only thing naming the run's project and workdir, while
        // `retireDeploy` and a retry both clear it.
        opening: (progress ?? []).slice(0, KEPT_OPENING_LINES)
      }
    ]
  }));
}

/** endDeploy marks a deploy finished, keeping or clearing its log. */
function endDeploy(all, scenario, outcome, keepProgress) {
  const entry = all[scenario];
  if (!entry) return all;
  return {
    ...all,
    [scenario]: {
      ...entry,
      running: false,
      outcome,
      ...(keepProgress ? {} : { progress: [], dropped: 0 })
    }
  };
}

export function finishDeploy(scenario, outcome) {
  fileReport(scenario, outcome, get(deploys)[scenario]?.progress);
  deploys.update((all) => endDeploy(all, scenario, outcome, true));
  releaseSocket();
}

/**
 * refuseDeploy records an ending that never began, and DISCARDS the log.
 *
 * `running` is true from `beginDeploy` until the POST resolves, and for
 * a deploy the server REFUSED that flag was true and wrong for the
 * whole round trip. Nothing of ours started, so no line collected in
 * that window was ours.
 *
 * It matters most for the lock refusal, because that is the one where
 * lines actually arrive: the refusal's own reason is that another apply
 * of this scenario is running and broadcasting. Keeping them rendered
 * that apply's live log underneath an "already deploying" banner -- the
 * adoption this store exists to refuse, through the one window where
 * the running check cannot help. For the other pre-flight refusals
 * (no such scenario, no --allow-deploy, a malformed request) the log is
 * empty and discarding it is a no-op; the rule is stated once rather
 * than conditioned on which refusal arrived.
 *
 * The entry is kept rather than deleted, because the refusal itself has
 * to be reported somewhere the reader will see it, and an outcome is
 * keyed by scenario so it lands on the right page and only that page.
 */
export function refuseDeploy(scenario, outcome) {
  fileReport(scenario, outcome, get(deploys)[scenario]?.progress);
  deploys.update((all) => endDeploy(all, scenario, outcome, false));
  releaseSocket();
}

/**
 * retireDeploy drops a finished deploy's banner and log.
 *
 * Reports are untouched, because they are not in this store. The banner
 * describing how the last attempt ended goes stale -- "Deployed. It is
 * listed on the Deployments page until its TTL expires." is false once
 * the TTL has passed. A report is a statement that infrastructure may
 * exist with no record of it, and it stays until somebody says
 * otherwise. Two lifetimes, two stores.
 */
export function retireDeploy(scenario) {
  deploys.update((all) => {
    const entry = all[scenario];
    if (!entry || entry.running) return all;
    const next = { ...all };
    delete next[scenario];
    return next;
  });
  releaseSocket();
}

/**
 * dismissReport drops one report, because an alarm nobody can silence
 * is an alarm everybody learns to ignore.
 *
 * The operator reads the project id, removes the project by hand, and
 * says so. Nothing else can: the deploy failed before registration, so
 * there is no live record for a reaper or a listing to retire. Keeping
 * it on screen for the rest of the session trains the reader past
 * exactly the message this arc says must never be lost.
 */
export function dismissReport(scenario, id) {
  reports.update((all) => {
    const left = (all[scenario] ?? []).filter((r) => r.id !== id);
    const next = { ...all };
    if (left.length) next[scenario] = left;
    else delete next[scenario];
    return next;
  });

  // Only once the scenario has NO reports left.
  //
  // An outcome that is itself a report is not rendered anywhere on its
  // own -- the page shows a pointer at the report instead -- so an
  // entry whose last report has gone would be a finished deploy with
  // its ending stated nowhere. But dismissing ONE of several deleted
  // the pointer and the whole apply log while a sibling report, naming
  // a project that is still live, was on screen beside it.
  const left = get(reports)[scenario]?.length ?? 0;
  if (left > 0) {
    releaseSocket();
    return;
  }
  deploys.update((all) => {
    const entry = all[scenario];
    if (!entry || entry.running || !entry.outcome?.mayHaveCreated) return all;
    const next = { ...all };
    delete next[scenario];
    return next;
  });
  releaseSocket();
}

/** isRunning answers the one question the Deploy button needs. */
export function isRunning(all, scenario) {
  return all?.[scenario]?.running === true;
}

/** isConnected distinguishes "no output yet" from "we cannot see it". */
export function isConnected(all) {
  return all?.__connected === true;
}

/**
 * pendingReports lists every deploy report this tab is still holding.
 *
 * A report is an outcome that MAY DESCRIBE INFRASTRUCTURE nobody has a
 * record of — "it may have created resources that are still running",
 * carrying the project id somebody has to remove by hand. A deploy that
 * fails before registration has no live record either, so this is the
 * only place it is ever said.
 *
 * Exported so it can be rendered somewhere that outlives one scenario's
 * page. It used to be visible only on the page it belonged to, which
 * meant navigating away hid the one message the code says must not be
 * lost — and briefly it was visible only when an UNRELATED scenario
 * fetch happened to fail, which made it a matter of luck.
 */
export function pendingReports(all) {
  const out = [];
  for (const [scenario, filed] of Object.entries(all || {})) {
    // The ID travels with the report, because dismissing one of several
    // identical failures has to name which -- two attempts can fail the
    // same way, and they leak two projects. A position would move when
    // a sibling went.
    for (const report of filed ?? []) {
      out.push({ scenario, id: report.id, message: report.message, opening: report.opening ?? [] });
    }
  }
  return out;
}
