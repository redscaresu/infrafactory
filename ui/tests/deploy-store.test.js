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
