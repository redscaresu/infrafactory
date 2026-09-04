import test from "node:test";
import assert from "node:assert/strict";

import { get } from "svelte/store";

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
  const { deploys, useConnector, watch, beginDeploy, finishDeploy, forgetDeploy } = await import(
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

  forgetDeploy("web-app-paris");
  forgetDeploy("lb-serving-paris");
  stop();
});

// A deploy that is still running must keep its stream open even if the
// reader wandered off, or coming back shows a frozen log.
test("the socket stays open while a deploy is in flight and nobody is watching", async () => {
  const { useConnector, watch, beginDeploy, finishDeploy, forgetDeploy } = await import(
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

  forgetDeploy("web-app-paris");
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
  const { deploys, useConnector, watch, beginDeploy, finishDeploy, forgetDeploy } = await import(
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

  forgetDeploy("web-app-paris");
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
  const { deploys, useConnector, watch, beginDeploy, refuseDeploy, forgetDeploy } = await import(
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

  forgetDeploy("web-app-paris");
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
  const { deploys, useConnector, watch, beginDeploy, finishDeploy, forgetDeploy, pendingReports } =
    await import("../src/lib/deploy-store.js");

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

  const reports = pendingReports(get(deploys));
  assert.equal(reports.length, 1, "the leak did not stop existing because the retry worked");
  assert.match(reports[0].message, /7c98d82e/);
  assert.equal(reports[0].scenario, "web-app-paris");

  forgetDeploy("web-app-paris");
  stop();
});

test("two failed attempts leak two projects and report both", async () => {
  const { deploys, useConnector, watch, beginDeploy, finishDeploy, forgetDeploy, pendingReports } =
    await import("../src/lib/deploy-store.js");

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

  const messages = pendingReports(get(deploys)).map((r) => r.message);
  assert.equal(messages.length, 2);
  assert.match(messages[0], /aaa/);
  assert.match(messages[1], /bbb/);

  forgetDeploy("web-app-paris");
  stop();
});

// A plain slice(-N) keeps the last thing that happened and loses the
// first — and `deploy` writes the scenario, its ref, its TTL and its
// workdir first, which is the only place those appear when the request
// never returns an ActionResult.
test("a long log keeps its opening and says how much it dropped", async () => {
  const { deploys, useConnector, watch, beginDeploy, forgetDeploy, finishDeploy } = await import(
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
  assert.ok(entry.progress.length <= 999, "while memory stays bounded");
  assert.match(entry.progress.at(-1), /line 2999/, "the tail is still the last thing that happened");

  finishDeploy("web-app-paris", { ok: true, mayHaveCreated: false, message: "Deployed." });
  forgetDeploy("web-app-paris");
  stop();
});
