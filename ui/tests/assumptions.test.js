import { test } from "node:test";
import assert from "node:assert/strict";

import { deriveCurrentStage } from "../src/lib/run-view.js";

// Assumptions the UI's COMMENTS depend on.
//
// See internal/api/assumptions_test.go for why this file exists. The
// short version: every serious defect found in the week of 2026-09-01
// was a confident claim about another component, written into a comment
// and never checked. Facts go in tests; decisions stay in prose.

// deriveCurrentStage reads ONLY `stage_start` events.
//
// Relied on by: ADR-0027 and stage_log_writer.go. Their first version
// justified recovering a `stage` field onto every progress entry with
// "the console groups by stage". It does not -- nothing reads that field
// -- and the whole design rested on it.
test("assumption: deriveCurrentStage ignores everything but stage_start", () => {
  const progress = JSON.stringify({
    event: "sandbox_deploy_progress",
    stage: "apply",
    status: "start",
    detail: "apply: running"
  });
  assert.equal(
    deriveCurrentStage([progress]),
    "",
    "a stage on a non-stage_start event feeds nothing"
  );

  const started = JSON.stringify({ event: "stage_start", status: "start", stage: "apply" });
  assert.equal(deriveCurrentStage([started]), "apply", "only this shape is read");
});

// deriveCurrentStage is given RAW JSON STRINGS, not parsed objects.
//
// Relied on by: the correction to ADR-0027. The Live Run console appends
// `JSON.stringify(msg)` for every frame and renders each string
// directly, so there was never a well-formed rendering for raw bytes to
// be inconsistent with -- the premise the adapter was built on.
test("assumption: the run console's lines are JSON strings, not objects", () => {
  const line = JSON.stringify({ event: "stage_start", status: "start", stage: "init" });
  assert.equal(typeof line, "string");
  assert.equal(deriveCurrentStage([line]), "init");

  // Handed the object it would have to parse, it finds nothing: the
  // function's contract is strings, which is what the page produces.
  assert.equal(
    deriveCurrentStage([{ event: "stage_start", status: "start", stage: "init" }]),
    "",
    "objects are not what this is fed"
  );
});

// A WebSocket's close handler fires on a LATER TASK, and connectWS's
// dispose calls back through it.
//
// Relied on by: deploy-store.js's generation bump on release. A comment
// there once said "a dispose that never calls back, exactly like
// connectWS" -- wrong twice, and the test written on it could not see a
// real defect.
test("assumption: a socket close callback is asynchronous", async () => {
  let closed = false;
  const fired = [];

  // The shape `connectWS` relies on: assigning `onclose`, then calling
  // `close()`, and the handler running after the current task.
  const socket = {
    onclose: null,
    close() {
      closed = true;
      queueMicrotask(() => this.onclose?.());
    }
  };
  socket.onclose = () => fired.push("closed");

  socket.close();
  assert.equal(closed, true, "close() itself is synchronous");
  assert.deepEqual(fired, [], "but the handler has NOT run yet");

  await Promise.resolve();
  assert.deepEqual(fired, ["closed"], "it runs on a later task, after our cleanup");
});
