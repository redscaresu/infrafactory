import test from "node:test";
import assert from "node:assert/strict";

import { get } from "svelte/store";

import {
  dismissReport as dismissForCleanup,
  finishDeploy as finishForCleanup,
  reports as sharedReports,
  retireDeploy as retireForCleanup
} from "../src/lib/deploy-store.js";

// The store is module-level and shared by every test in this file, so a
// test that leaves an entry RUNNING keeps the socket open for the next
// one. `retireDeploy` deliberately refuses to drop a running deploy --
// that is what makes a log survive navigation -- so teardown has to end
// it first.
function cleanup(...scenarios) {
  for (const scenario of scenarios) {
    finishForCleanup(scenario, { ok: true, mayHaveCreated: false, message: "cleanup" });
    // Reports deliberately survive `retireDeploy`, so teardown has to
    // dismiss them the way an operator would.
    while (get(sharedReports)[scenario]?.length) {
      dismissForCleanup(scenario, get(sharedReports)[scenario][0].id);
    }
    retireForCleanup(scenario);
  }
}

import { isConnected, isRunning } from "../src/lib/deploy-store.js";

// The two questions the page asks, answered from state that outlives it.
test("isRunning is true only while a deploy of that scenario is in flight", () => {
  const all = { "web-app-paris": { running: true, progress: [], outcome: null } };
  assert.equal(isRunning(all, "web-app-paris"), true);
  assert.equal(isRunning(all, "lb-serving-paris"), false, "a different scenario is not running");
  assert.equal(isRunning({}, "web-app-paris"), false);
  assert.equal(isRunning(undefined, "web-app-paris"), false);
});

test("isRunning is false once a deploy has finished", () => {
  const all = { "web-app-paris": { running: false, progress: ["x"], outcome: { ok: true } } };
  assert.equal(isRunning(all, "web-app-paris"), false);
});

// "No output yet" and "we cannot see it" must not render the same way.
test("isConnected reports the socket rather than the absence of lines", () => {
  assert.equal(isConnected({ __connected: true }), true);
  assert.equal(isConnected({ __connected: false }), false);
  assert.equal(isConnected({}), false, "unknown is not connected");
});

// The socket wiring, which is the part most worth testing and was the
// reason this module could not import ws.ts directly.
test("progress lines are routed to the scenario they name", async () => {
  const { deploys, useConnector, watch, beginDeploy, finishDeploy } = await import(
    "../src/lib/deploy-store.js"
  );

  let onMessage;
  useConnector((handler) => {
    onMessage = handler;
    return () => {};
  });

  const stop = watch();
  beginDeploy("web-app-paris");
  beginDeploy("lb-serving-paris");

  onMessage({ type: "deploy_progress", data: { subject: "web-app-paris", line: "init: running" } });
  onMessage({ type: "deploy_progress", data: { subject: "lb-serving-paris", line: "plan: running" } });
  onMessage({ type: "deploy_progress", data: { subject: "not-deployed", line: "ignored" } });

  const all = get(deploys);
  assert.deepEqual(all["web-app-paris"].progress, ["init: running"]);
  assert.deepEqual(all["lb-serving-paris"].progress, ["plan: running"]);
  assert.equal(all["not-deployed"], undefined, "a scenario with no deploy gains no state");

  finishDeploy("web-app-paris", { ok: true, message: "done" });
  assert.equal(get(deploys)["web-app-paris"].running, false);
  assert.deepEqual(get(deploys)["web-app-paris"].progress, ["init: running"], "the log survives");

  cleanup("web-app-paris");
  cleanup("lb-serving-paris");
  stop();
});

// A deploy that is still running must keep its stream open even if the
// reader wandered off, or coming back shows a frozen log.
test("the socket stays open while a deploy is in flight and nobody is watching", async () => {
  const { useConnector, watch, beginDeploy, finishDeploy } = await import(
    "../src/lib/deploy-store.js"
  );

  let closed = 0;
  useConnector(() => () => {
    closed += 1;
  });

  const stop = watch();
  beginDeploy("web-app-paris");

  stop();
  assert.equal(closed, 0, "the deploy is still running; the stream must not be closed");

  finishDeploy("web-app-paris", { ok: true, message: "done" });
  assert.equal(closed, 1, "nothing running and nobody watching, so it closes");

  cleanup("web-app-paris");
});

// A finished entry stays on screen until the reader navigates away, and
// the stream is keyed by SCENARIO -- so without this, a second deploy of
// the same scenario started anywhere else (another tab, the CLI)
// appended into it, and the page rendered a live, growing log of an
// apply it did not start underneath a completed-outcome banner.
//
// That is the adoption this arc removed, arriving through the door the
// store's own comments name.
test("a finished deploy stops absorbing progress for its scenario", async () => {
  const { deploys, useConnector, watch, beginDeploy, finishDeploy } = await import(
    "../src/lib/deploy-store.js"
  );

  let onMessage;
  useConnector((handler) => {
    onMessage = handler;
    return () => {};
  });

  const stop = watch();
  beginDeploy("web-app-paris");
  onMessage({ type: "deploy_progress", data: { subject: "web-app-paris", line: "apply: running" } });
  finishDeploy("web-app-paris", { ok: true, message: "Deployed." });

  onMessage({
    type: "deploy_progress",
    data: { subject: "web-app-paris", line: "apply: somebody else's run" }
  });

  const entry = get(deploys)["web-app-paris"];
  assert.deepEqual(entry.progress, ["apply: running"], "only what this tab was watching");
  assert.equal(entry.running, false);

  cleanup("web-app-paris");
  stop();
});

// The window the `running` check cannot close on its own.
//
// The entry is created BEFORE the POST, so it is running -- and
// collecting -- for the whole round trip. When the answer is a refusal,
// the reason is that somebody else's apply of this scenario holds the
// lock, which makes every line collected in that window theirs. Keeping
// them rendered another tab's live log underneath an "already deploying"
// banner.
test("a refused deploy discards the lines it collected while waiting", async () => {
  const { deploys, useConnector, watch, beginDeploy, refuseDeploy } = await import(
    "../src/lib/deploy-store.js"
  );

  let onMessage;
  useConnector((handler) => {
    onMessage = handler;
    return () => {};
  });

  const stop = watch();
  beginDeploy("web-app-paris");
  onMessage({
    type: "deploy_progress",
    data: { subject: "web-app-paris", line: "apply: somebody else's run" }
  });

  refuseDeploy("web-app-paris", { ok: false, message: "already deploying" });

  const entry = get(deploys)["web-app-paris"];
  assert.deepEqual(entry.progress, [], "those lines were never ours");
  assert.equal(entry.running, false);
  assert.equal(entry.outcome.message, "already deploying", "the refusal still has to be reported");

  cleanup("web-app-paris");
  stop();
});

// The sentinel is a boolean, not an entry. A keyed lookup makes it
// unreachable by construction; the scan needed an explicit skip.
test("the connection sentinel is never mistaken for a deploy", async () => {
  const { deploys, useConnector, watch } = await import("../src/lib/deploy-store.js");

  let onMessage;
  let onConnected;
  useConnector((handler, connected) => {
    onMessage = handler;
    onConnected = connected;
    return () => {};
  });

  const stop = watch();
  onConnected(true);
  onMessage({ type: "deploy_progress", data: { subject: "__connected", line: "x" } });

  assert.equal(get(deploys).__connected, true, "still a boolean, not an entry with a log");
  stop();
});

// The reader's obvious next action after a failed deploy is to try
// again — and that used to delete the project id the failure was about.
// If the retry then succeeded, the banner became "Deployed." and the
// next navigation dropped it, so a leaked project with no live record
// was named nowhere at all.
test("a retry does not delete the report of what the last attempt leaked", async () => {
  const { deploys, useConnector, watch, beginDeploy, finishDeploy, pendingReports, reports } = await import("../src/lib/deploy-store.js");

  useConnector(() => () => {});
  const stop = watch();

  beginDeploy("web-app-paris");
  finishDeploy("web-app-paris", {
    ok: false,
    mayHaveCreated: true,
    message: "web-app-paris: project 7c98d82e is live and could not be deleted"
  });

  beginDeploy("web-app-paris");
  finishDeploy("web-app-paris", { ok: true, mayHaveCreated: false, message: "Deployed." });

  const filed = pendingReports(get(reports)).filter((r) => r.scenario === "web-app-paris");
  assert.equal(filed.length, 1, "the leak did not stop existing because the retry worked");
  assert.match(filed[0].message, /7c98d82e/);
  assert.equal(filed[0].scenario, "web-app-paris");

  cleanup("web-app-paris");
  stop();
});

test("two failed attempts leak two projects and report both", async () => {
  const { deploys, useConnector, watch, beginDeploy, finishDeploy, pendingReports, reports } = await import("../src/lib/deploy-store.js");

  useConnector(() => () => {});
  const stop = watch();

  for (const project of ["aaa", "bbb"]) {
    beginDeploy("web-app-paris");
    finishDeploy("web-app-paris", {
      ok: false,
      mayHaveCreated: true,
      message: `project ${project} is live and could not be deleted`
    });
  }

  const messages = pendingReports(get(reports))
    .filter((r) => r.scenario === "web-app-paris")
    .map((r) => r.message);
  assert.equal(messages.length, 2);
  assert.match(messages[0], /aaa/);
  assert.match(messages[1], /bbb/);

  cleanup("web-app-paris");
  stop();
});

// A plain slice(-N) keeps the last thing that happened and loses the
// first — and `deploy` writes the scenario, its ref, its TTL and its
// workdir first, which is the only place those appear when the request
// never returns an ActionResult.
test("a long log keeps its opening and says how much it dropped", async () => {
  const { deploys, useConnector, watch, beginDeploy, finishDeploy } = await import(
    "../src/lib/deploy-store.js"
  );

  let onMessage;
  useConnector((handler) => {
    onMessage = handler;
    return () => {};
  });

  const stop = watch();
  beginDeploy("web-app-paris");
  for (let i = 0; i < 3000; i += 1) {
    onMessage({
      type: "deploy_progress",
      data: { subject: "web-app-paris", line: i === 0 ? "  workdir: /tmp/if-run-abc" : `line ${i}` }
    });
  }

  const entry = get(deploys)["web-app-paris"];
  assert.match(entry.progress[0], /workdir/, "the identifying half of the log survives");
  assert.ok(entry.dropped > 0, "and the reader is told something is missing");
  // Bounded, not exact: trimming happens in batches, so the log may sit
  // a little over the cap between trims. Doing it on every line past
  // the cap only bounds the stall the cap exists to remove.
  assert.ok(entry.progress.length <= 1099, "while memory stays bounded");
  assert.ok(entry.progress.length >= 999, "and the cap is not over-trimmed");
  assert.match(entry.progress.at(-1), /line 2999/, "the tail is still the last thing that happened");

  finishDeploy("web-app-paris", { ok: true, mayHaveCreated: false, message: "Deployed." });
  cleanup("web-app-paris");
  stop();
});

// The round-eleven fix accumulated reports on the entry; forgetting the
// entry threw them away again. Fail, retry, succeed, navigate — and the
// project the first attempt leaked is named nowhere, because the second
// attempt's outcome says there is nothing to report.
test("a report outlives the entry's last outcome", async () => {
  const { deploys, useConnector, watch, beginDeploy, finishDeploy, pendingReports, reports } = await import("../src/lib/deploy-store.js");

  useConnector(() => () => {});
  const stop = watch();

  beginDeploy("web-app-paris");
  finishDeploy("web-app-paris", {
    ok: false,
    mayHaveCreated: true,
    message: "project 7c98d82e is live and could not be deleted"
  });
  beginDeploy("web-app-paris");
  finishDeploy("web-app-paris", { ok: true, mayHaveCreated: false, message: "Deployed." });

  const entry = get(deploys)["web-app-paris"];
  assert.equal(entry.outcome.mayHaveCreated, false, "the LAST outcome has nothing to report");
  assert.equal(get(reports)["web-app-paris"].length, 1, "the report is filed separately");
  const mine = pendingReports(get(reports)).filter((r) => r.scenario === "web-app-paris");
  assert.match(mine[0].message, /7c98d82e/);

  cleanup("web-app-paris");
  stop();
});

// Two things were conflated: the BANNER for how the last attempt ended,
// which goes stale, and the REPORT that infrastructure may exist with no
// record, which does not. Returning early whenever an entry held a
// report kept both, so a failed-then-retried deploy showed "Deployed. It
// is listed on the Deployments page until its TTL expires." on every
// later visit for the rest of the session.
test("retiring a deploy drops the stale banner and keeps the report", async () => {
  const {
    deploys,
    useConnector,
    watch,
    beginDeploy,
    finishDeploy,
    retireDeploy,
    dismissReport,
    pendingReports,
    reports
  } = await import("../src/lib/deploy-store.js");

  useConnector(() => () => {});
  const stop = watch();

  beginDeploy("retire-keeps-report");
  finishDeploy("retire-keeps-report", {
    ok: false,
    mayHaveCreated: true,
    message: "project 7c98d82e is live and could not be deleted"
  });
  beginDeploy("retire-keeps-report");
  finishDeploy("retire-keeps-report", { ok: true, mayHaveCreated: false, message: "Deployed." });

  // The action under test, not teardown.
  retireDeploy("retire-keeps-report");

  assert.equal(get(deploys)["retire-keeps-report"], undefined, "the stale banner goes entirely");
  const mine = pendingReports(get(reports)).filter((r) => r.scenario === "retire-keeps-report");
  assert.equal(mine.length, 1, "the leak report stays");

  dismissReport("retire-keeps-report", mine[0].id);
  stop();
});

test("retiring a deploy with nothing to report removes it entirely", async () => {
  const { deploys, useConnector, watch, beginDeploy, finishDeploy, retireDeploy } = await import(
    "../src/lib/deploy-store.js"
  );

  useConnector(() => () => {});
  const stop = watch();

  beginDeploy("retire-clean");
  finishDeploy("retire-clean", { ok: true, mayHaveCreated: false, message: "Deployed." });
  retireDeploy("retire-clean");

  assert.equal(get(deploys)["retire-clean"], undefined);
  stop();
});

// An alarm nobody can silence is an alarm everybody learns to ignore.
// Nothing else can retire this: the deploy failed before registration,
// so there is no live record for a reaper or a listing to clear.
test("a report can be dismissed once the operator has dealt with it", async () => {
  const { deploys, useConnector, watch, beginDeploy, finishDeploy, dismissReport, pendingReports, reports } = await import("../src/lib/deploy-store.js");

  useConnector(() => () => {});
  const stop = watch();

  for (const project of ["aaa", "bbb"]) {
    beginDeploy("dismiss-two");
    finishDeploy("dismiss-two", {
      ok: false,
      mayHaveCreated: true,
      message: `project ${project} is live`
    });
  }

  const mine = (all) => pendingReports(all).filter((r) => r.scenario === "dismiss-two");
  const before = mine(get(reports));
  assert.equal(before.length, 2);

  // Dismissing one names WHICH: two attempts can fail identically.
  dismissReport("dismiss-two", before[0].id);

  const after = mine(get(reports));
  assert.equal(after.length, 1);
  assert.match(after[0].message, /bbb/, "the one that was dealt with is the one that went");

  dismissReport("dismiss-two", after[0].id);
  assert.equal(mine(get(reports)).length, 0);
  stop();
});

// A report that pointed at the log pointed at nothing: `retireDeploy`
// and a retry both clear `progress`. When the request never returned an
// ActionResult the message is generic — "may still be running" — and
// the head of the log is the only place the run's workdir is named, so
// the report takes a copy.
test("a report carries the opening lines that identify its run", async () => {
  const { deploys, useConnector, watch, beginDeploy, finishDeploy, retireDeploy, pendingReports, reports } = await import("../src/lib/deploy-store.js");

  let onMessage;
  useConnector((handler) => {
    onMessage = handler;
    return () => {};
  });

  const stop = watch();
  beginDeploy("carries-opening");
  for (const line of ["Deploying carries-opening (abc123) for 4h0m0s", "  workdir: /tmp/if-run-xyz"]) {
    onMessage({ type: "deploy_progress", data: { subject: "carries-opening", line } });
  }
  finishDeploy("carries-opening", {
    ok: false,
    mayHaveCreated: true,
    message: "The deploy may still be running on the server."
  });

  // Leaving drops the entry and its log entirely, exactly as navigating
  // away does. The report is in its own store and is untouched.
  retireDeploy("carries-opening");
  assert.equal(get(deploys)["carries-opening"], undefined);

  const [report] = pendingReports(get(reports)).filter((r) => r.scenario === "carries-opening");
  assert.match(report.opening.join("\n"), /workdir: \/tmp\/if-run-xyz/,
    "the generic message names nothing; these lines are all there is");

  cleanup("carries-opening");
  stop();
});

// Disposing a socket does not silence it: the close handshake is
// asynchronous, so an old connection's `onclose` can fire after a
// replacement has opened. It then wrote `__connected: false` over a
// healthy socket, and nothing ever re-fires `onopen` — so every deploy
// for the rest of the session rendered "Not receiving progress" over a
// stream that was working.
test("a closing socket cannot mark its replacement disconnected", async () => {
  const { deploys, useConnector, watch, isConnected } = await import(
    "../src/lib/deploy-store.js"
  );

  const statuses = [];
  useConnector((_onMessage, onStatus) => {
    statuses.push(onStatus);
    return () => {};
  });

  const first = watch();
  statuses[0](true);
  first(); // watchers → 0, nothing running, so the socket is disposed

  const second = watch();
  statuses[1](true);
  assert.equal(isConnected(get(deploys)), true);

  // The old connection's close handshake completes late.
  statuses[0](false);

  assert.equal(isConnected(get(deploys)), true, "the live socket is still live");
  second();
});

// Dismissing used to name a POSITION, and positions move. Two clicks
// landing before a re-render deleted two different reports — the second
// one a leak the operator had never read.
test("dismissing names a report, not a position", async () => {
  const { deploys, useConnector, watch, beginDeploy, finishDeploy, dismissReport, pendingReports, reports } = await import("../src/lib/deploy-store.js");

  useConnector(() => () => {});
  const stop = watch();

  for (const project of ["aaa", "bbb", "ccc"]) {
    beginDeploy("stable-ids");
    finishDeploy("stable-ids", {
      ok: false,
      mayHaveCreated: true,
      message: `project ${project} is live`
    });
  }

  const mine = () => pendingReports(get(reports)).filter((r) => r.scenario === "stable-ids");
  const middle = mine()[1];

  // The same handle twice, as a double-click would.
  dismissReport("stable-ids", middle.id);
  dismissReport("stable-ids", middle.id);

  const left = mine().map((r) => r.message);
  assert.equal(left.length, 2, "the second click removed nothing that was not already gone");
  assert.match(left[0], /aaa/);
  assert.match(left[1], /ccc/);

  cleanup("stable-ids");
  stop();
});

// The button's `disabled` lives on a page; the store outlives it. A
// second start over a live apply reset its log and its outcome, and the
// 423 that followed marked the original finished — so the socket
// handler discarded its lines while it kept creating infrastructure.
test("a second start cannot orphan a running deploy", async () => {
  const { deploys, useConnector, watch, beginDeploy, finishDeploy } = await import(
    "../src/lib/deploy-store.js"
  );

  let onMessage;
  useConnector((handler) => {
    onMessage = handler;
    return () => {};
  });

  const stop = watch();
  beginDeploy("no-double-start");
  onMessage({
    type: "deploy_progress",
    data: { subject: "no-double-start", line: "  workdir: /tmp/if-run-abc" }
  });

  beginDeploy("no-double-start");

  const entry = get(deploys)["no-double-start"];
  assert.deepEqual(entry.progress, ["  workdir: /tmp/if-run-abc"], "the live log is untouched");
  assert.equal(entry.running, true);

  finishDeploy("no-double-start", { ok: true, mayHaveCreated: false, message: "Deployed." });
  cleanup("no-double-start");
  stop();
});

// The store can decline to record a second start; it cannot stop the
// POST. So it has to SAY so, and the caller has to stop — otherwise the
// 423 that comes back is applied to the first deploy, clearing its log
// and marking it finished while it keeps creating infrastructure.
test("beginDeploy reports whether the deploy may proceed", async () => {
  const { useConnector, watch, beginDeploy, finishDeploy } = await import(
    "../src/lib/deploy-store.js"
  );

  useConnector(() => () => {});
  const stop = watch();

  assert.equal(beginDeploy("binding-answer"), true);
  assert.equal(beginDeploy("binding-answer"), false, "one is already running");

  finishDeploy("binding-answer", { ok: true, mayHaveCreated: false, message: "Deployed." });
  assert.equal(beginDeploy("binding-answer"), true, "and a retry is allowed once it ends");

  finishDeploy("binding-answer", { ok: true, mayHaveCreated: false, message: "Deployed." });
  cleanup("binding-answer");
  stop();
});
