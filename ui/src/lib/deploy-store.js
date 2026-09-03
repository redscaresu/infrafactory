import { writable, get } from "svelte/store";
import { acceptCompleteEvent, acceptProgressEvent } from "./deployments-view.js";

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
        if (scenario === "__connected") continue;

        if (acceptProgressEvent(msg, scenario)) {
          const line = msg.data.line;
          deploys.update((all) => {
            const entry = all[scenario];
            if (!entry) return all;
            return { ...all, [scenario]: { ...entry, progress: [...entry.progress, line] } };
          });
          continue;
        }

        // The terminal event. Only an ADOPTED deploy needs it -- the
        // tab that issued the POST learns from its own response, and
        // that response carries the full ActionResult, which is richer.
        if (acceptCompleteEvent(msg, scenario)) {
          deploys.update((all) => {
            const entry = all[scenario];
            if (!entry || !entry.adopted) return all;
            return {
              ...all,
              [scenario]: {
                ...entry,
                running: false,
                outcome: msg.data.clean
                  ? // The page renders its own text for ok:true, so
                    // duplicating it here would be a second copy that
                    // silently diverges. Left empty on purpose.
                    { ok: true, message: "" }
                  : {
                      ok: false,
                      message:
                        msg.data.error ||
                        "did not finish cleanly — resources may exist. Check the Deployments page."
                    }
              }
            };
          });
          releaseSocket();
        }
      }
    },
    (connected) => {
      deploys.update((all) => ({ ...all, __connected: connected }));
      // A terminal event can be MISSED: the hub drops messages for a
      // client whose buffer filled, a reconnect loses everything sent
      // in the gap, and an event arriving before this tab has adopted
      // anything matches no entry.
      //
      // Without a recovery path an adopted entry stays running for the
      // rest of the session -- a permanently disabled button and a
      // panel that never resolves, which is the failure the terminal
      // event was introduced to fix.
      //
      // Re-asking on (re)connect is that path, and it costs one request
      // per connection rather than one every few seconds.
      if (connected) void resync();
    }
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

// resync re-asks the server what is deploying. Installed by the page,
// because this module does not own the API client.
let resync = async () => {};

/** useResync installs the recovery fetch. */
export function useResync(fn) {
  resync = fn;
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
      // A RUNNING entry is left alone -- replacing it would throw away
      // the log of a live apply, and its owner is the only thing that
      // can mark it finished.
      //
      // A FINISHED one is not: skipping those meant a tab whose own
      // deploy had been refused (423) held a stale entry forever, so
      // every later listing was ignored, the button stayed enabled, and
      // the reader was invited to click again for the whole apply --
      // precisely what adoption exists to prevent.
      if (next[scenario]?.running) continue;
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
        // An outcome, not a bare `running: false`. Without one the
        // panel's `{#if deploying || progress.length}` goes false and
        // the whole thing VANISHES -- the page ends up identical to one
        // where nothing ever ran, for an apply that just created
        // billable infrastructure.
        next[scenario] = {
          ...entry,
          running: false,
          outcome: {
            ok: false,
            message: "finished while this page was not watching — check the Deployments page for what it left."
          }
        };
      }
    }
    return next;
  });

  if (names.size > 0) ensureSocket();
  releaseSocket();
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
