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

// adoptInFlight had no test at all, and its "leave existing entries
// alone" branch had zero coverage: deleting that line passed everything
// while destroying the log of a running billable apply.
test("adoptInFlight does not overwrite a deploy this tab owns", async () => {
  const { deploys, useConnector, adoptInFlight, beginDeploy, forgetDeploy } = await import(
    "../src/lib/deploy-store.js"
  );
  let onMessage;
  useConnector((handler) => {
    onMessage = handler;
    return () => {};
  });

  beginDeploy("web-app-paris");
  onMessage({ type: "deploy_progress", data: { subject: "web-app-paris", line: "init: running" } });

  adoptInFlight(["web-app-paris"]);

  const entry = get(deploys)["web-app-paris"];
  assert.deepEqual(entry.progress, ["init: running"], "the log of a running apply must survive");
  assert.equal(entry.adopted, undefined, "this tab owns it; it was not adopted");

  forgetDeploy("web-app-paris");
});

// An adopted entry the server no longer reports has finished. Nothing
// else can tell us: there is no terminal websocket event, and only the
// tab that issued the POST calls finishDeploy. Without this it stayed
// `running` for the rest of the session.
test("an adopted deploy stops running once the server stops reporting it", async () => {
  const { deploys, useConnector, adoptInFlight, forgetDeploy, isRunning } = await import(
    "../src/lib/deploy-store.js"
  );
  useConnector(() => () => {});

  adoptInFlight(["web-app-paris"]);
  assert.equal(isRunning(get(deploys), "web-app-paris"), true);

  adoptInFlight([]);
  assert.equal(
    isRunning(get(deploys), "web-app-paris"),
    false,
    "a permanently disabled Deploy button is the failure this prevents"
  );

  forgetDeploy("web-app-paris");
});

// A deploy this tab owns must NOT be cleared by an estate listing that
// has not caught up yet -- only the owner knows when its own POST
// returned.
test("adoptInFlight never marks an owned deploy finished", async () => {
  const { deploys, useConnector, adoptInFlight, beginDeploy, forgetDeploy, isRunning } =
    await import("../src/lib/deploy-store.js");
  useConnector(() => () => {});

  beginDeploy("web-app-paris");
  adoptInFlight([]);

  assert.equal(isRunning(get(deploys), "web-app-paris"), true, "the owner decides, not the listing");
  forgetDeploy("web-app-paris");
});

test("adoptInFlight ignores a payload that is not a list", async () => {
  const { deploys, adoptInFlight } = await import("../src/lib/deploy-store.js");
  adoptInFlight(undefined);
  adoptInFlight(null);
  assert.deepEqual(Object.keys(get(deploys)).filter((k) => k !== "__connected"), []);
});
