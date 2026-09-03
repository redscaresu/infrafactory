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
 * The server has no in-flight lock, so this is the only thing standing
 * between a reader and that second deploy. It has to outlive the page.
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
