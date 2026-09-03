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
      const current = get(deploys);
      for (const scenario of Object.keys(current)) {
        if (scenario === "__connected") continue;
        if (!acceptProgressEvent(msg, scenario)) continue;
        deploys.update((all) => {
          const entry = all[scenario];
          if (!entry) return all;
          return { ...all, [scenario]: { ...entry, progress: [...entry.progress, msg.data.line] } };
        });
      }
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

/** forget clears a finished deploy once the reader has left its page. */
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
