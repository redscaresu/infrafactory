import { writable, get } from "svelte/store";
import { acceptProgressEvent } from "./deployments-view.js";

/**
 * Deploys in flight, keyed by scenario name.
 *
 * # Why this is a module-level store and not component state
 *
 * A deploy runs for minutes on the server, detached from the request. The
 * scenario page that started it does not: SvelteKit reuses the route
 * component between scenarios and destroys it entirely when you navigate
 * to another section.
 *
 * While this lived in the component, two things followed and review found
 * both:
 *
 *   - navigating away and back during a deploy left the page with no log,
 *     no warning and a disabled button labelled "Deploying " with a blank
 *     subject — a real, billable apply rendered as an unexplained frozen
 *     control;
 *   - navigating to another SECTION unmounted the component, so coming
 *     back gave a fresh one that believed nothing was running and
 *     happily started a SECOND deploy of the same scenario. Two
 *     run-owned projects, double billing, one of which the operator does
 *     not know exists.
 *
 * This store is ADVISORY, and saying otherwise here would be the same
 * false-explanation defect the rest of this work keeps finding. The
 * refusal lives on the server: `LiveDeployer` holds a per-scenario lock
 * and answers 423 to a second deploy, so a reader who bypasses this
 * module entirely still cannot start two.
 *
 * What this buys is that the reader is not INVITED to try -- the button
 * is not offered for something the server would refuse -- and that a log
 * survives a page they navigated away from.
 *
 * Keyed by SCENARIO because that is what the progress events carry: the
 * deployment id is minted inside the command, after the request is
 * accepted.
 */
export const deploys = writable({});

let socket;
let watchers = 0;

// The connector is INJECTED rather than imported.
//
// `ws.ts` is TypeScript, and the unit tests run under `node --test`,
// which resolves plain JS only -- importing it here would make this
// module untestable, and the socket wiring is exactly the part worth
// testing. The component supplies the real one at mount.
let connect = () => () => {};

/** useConnector installs the websocket factory. */
export function useConnector(fn) {
  connect = fn;
}

/** shape of one entry, for readers: { progress: string[], outcome: {ok, message} | null } */

function ensureSocket() {
  if (socket) return;
  socket = connect(
    (msg) => {
      const current = get(deploys);
      for (const scenario of Object.keys(current)) {
        if (!acceptProgressEvent(msg, scenario)) continue;
        const line = msg.data.line;
        deploys.update((all) => {
          const entry = all[scenario];
          if (!entry) return all;
          return { ...all, [scenario]: { ...entry, progress: [...entry.progress, line] } };
        });
      }
    },
    (connected) => deploys.update((all) => ({ ...all, __connected: connected }))
  );
}

function releaseSocket() {
  // Closed only when nothing is in flight AND nobody is looking. A
  // deploy that is still running must keep its stream open even if the
  // reader has wandered off, or coming back shows a frozen log.
  const current = get(deploys);
  const inFlight = Object.keys(current).some((k) => k !== "__connected" && current[k]?.running);
  if (inFlight || watchers > 0) return;
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
 * adoptInFlight seeds the store from what the SERVER says is deploying.
 *
 * A page reload wipes this module along with everything else, so without
 * it a refresh mid-deploy showed an enabled Deploy button and no log --
 * and the second click was only refused once it reached the server. The
 * refusal is the guard; this is what stops the reader being invited to
 * trip it.
 *
 * Existing entries are left alone: a deploy this tab started already has
 * its progress, and replacing it would throw the log away.
 */
export function adoptInFlight(scenarios) {
  if (!Array.isArray(scenarios)) return;
  const names = new Set(scenarios);

  deploys.update((all) => {
    const next = { ...all };

    for (const scenario of names) {
      // An entry this tab OWNS is left alone. Replacing it would throw
      // away the log of a running billable apply, and the owner is the
      // only thing that can mark it finished.
      if (next[scenario]) continue;
      next[scenario] = { running: true, progress: [], outcome: null, adopted: true };
    }

    // An ADOPTED entry the server no longer reports has finished, and
    // nothing else can tell us: the only websocket event kind is a
    // progress line, so there is no terminal event, and the tab that
    // issued the POST is the only one that calls finishDeploy.
    //
    // Without this an adopted entry stayed `running` for the rest of
    // the session -- a permanently disabled Deploy button, a progress
    // panel that never resolved, and a socket that could never close.
    for (const scenario of Object.keys(next)) {
      if (scenario === "__connected") continue;
      const entry = next[scenario];
      if (entry?.adopted && entry.running && !names.has(scenario)) {
        next[scenario] = { ...entry, running: false, outcome: null, finishedElsewhere: true };
      }
    }
    return next;
  });

  if (names.size > 0) ensureSocket();
  releaseSocket();
}

/**
 * pollInFlight re-asks the server what is running, on an interval.
 *
 * Adopted entries need it: this tab did not issue their POST, so it
 * never sees a response, and there is no terminal websocket event to
 * wait for. Without polling an adopted deploy is `running` forever.
 *
 * Only runs while something adopted is in flight -- a scenario page
 * sitting idle should not poll the estate.
 */
export function pollInFlight(fetchDeploying, intervalMs = 5000) {
  const tick = async () => {
    const current = get(deploys);
    const adoptedRunning = Object.keys(current).some(
      (k) => k !== "__connected" && current[k]?.adopted && current[k]?.running
    );
    if (!adoptedRunning) return;
    try {
      adoptInFlight(await fetchDeploying());
    } catch {
      // Leave the state as it is. An unreachable estate is not evidence
      // that the deploy finished.
    }
  };

  const timer = setInterval(tick, intervalMs);
  return () => clearInterval(timer);
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

/** forget clears a finished deploy once the reader has seen it. */
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
