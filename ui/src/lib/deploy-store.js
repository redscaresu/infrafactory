import { writable, get } from "svelte/store";
import { acceptProgressEvent } from "./deployments-view.js";

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

let socket;
let watchers = 0;

// Injected rather than imported: `ws.ts` is TypeScript and the unit
// tests run under `node --test`, which resolves plain JS only.
let connect = () => () => {};

/** useConnector installs the websocket factory. */
export function useConnector(fn) {
  connect = fn;
}

function ensureSocket() {
  if (socket) return;
  socket = connect(
    (msg) => {
      // The event names its own subject, so this is a lookup rather
      // than a scan. Asking every key "is this yours?" needed a skip for
      // the `__connected` sentinel; a keyed read makes it unreachable
      // by construction instead, because the sentinel is a boolean and
      // has no `running`.
      const scenario = msg?.data?.subject;
      if (!scenario || !acceptProgressEvent(msg, scenario)) return;

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
        return { ...all, [scenario]: { ...entry, progress: [...entry.progress, msg.data.line] } };
      });
    },
    (connected) => deploys.update((all) => ({ ...all, __connected: connected }))
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

export function beginDeploy(scenario) {
  ensureSocket();
  deploys.update((all) => ({
    ...all,
    [scenario]: { running: true, progress: [], outcome: null }
  }));
}

export function finishDeploy(scenario, outcome) {
  deploys.update((all) => {
    const entry = all[scenario];
    if (!entry) return all;
    return { ...all, [scenario]: { ...entry, running: false, outcome } };
  });
  releaseSocket();
}

/**
 * refuseDeploy records an ending that never began, and DISCARDS the log.
 *
 * `running` is true from `beginDeploy` until the POST resolves, and for
 * a deploy that is about to be REFUSED that flag is true and wrong: the
 * refusal means somebody else holds the lock, so every line matching
 * this scenario during the round trip belongs to their apply. Keeping
 * them rendered another tab's live log underneath an "already
 * deploying" banner -- the adoption this store exists to refuse,
 * through the one window where the running check cannot help.
 *
 * The entry is kept rather than deleted, because the refusal itself has
 * to be reported somewhere the reader will see it, and an outcome is
 * keyed by scenario so it lands on the right page and only that page.
 */
export function refuseDeploy(scenario, outcome) {
  deploys.update((all) => {
    const entry = all[scenario];
    if (!entry) return all;
    return { ...all, [scenario]: { ...entry, running: false, progress: [], outcome } };
  });
  releaseSocket();
}

/**
 * forget clears a finished deploy once the reader has left its page.
 *
 * Not used for a refusal any more. A refused deploy is now recorded as
 * an OUTCOME like any other ending, because an outcome is keyed by
 * scenario and therefore survives navigation -- and a refusal that was
 * dropped whenever the reader had moved left the button silently
 * reverting to "Deploy…" with no log, no message and no explanation.
 *
 * Forgetting was there to stop a refused entry adopting another tab's
 * stream; the socket handler now ignores entries that are not running,
 * which closes that for every finished entry rather than for this one.
 */
export function forgetDeploy(scenario) {
  deploys.update((all) => {
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
