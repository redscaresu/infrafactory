import test from "node:test";
import assert from "node:assert/strict";

import {
  DeployError,
  isActionResult,
  readJSON,
  startedNothing
} from "../src/lib/deploy-response.js";

// These decide whether a reader is told "nothing was created" or
// "resources may still be running". They lived in api.ts, which
// `node --test` cannot import, so inverting any of them passed the
// whole suite.

test("startedNothing believes only an explicit server claim", () => {
  assert.equal(startedNothing({ error: "x", started_nothing: true }), true);
  assert.equal(startedNothing({ error: "x", started_nothing: false }), false);
  assert.equal(startedNothing({ error: "x" }), false, "absence is unknown, not a promise");
  assert.equal(startedNothing(null), false);
  assert.equal(startedNothing("started_nothing"), false, "a string is not a claim");
  assert.equal(startedNothing({ started_nothing: "true" }), false, "nor is a truthy string");
});

// Erring towards "unknown" sends a reader to check the Deployments page
// for infrastructure that was never created. Erring the other way tells
// them nothing happened while a project is being billed.
test("an unrecognised body is unknown rather than a refusal", () => {
  assert.equal(startedNothing(undefined), false);
  assert.equal(startedNothing([]), false);
});

test("isActionResult discriminates on the field, not the status", () => {
  assert.equal(isActionResult({ clean: false, failures: [] }), true);
  assert.equal(isActionResult({ clean: true }), true);
  assert.equal(isActionResult({ error: "already deploying" }), false);
  assert.equal(isActionResult(null), false);
  assert.equal(isActionResult("clean"), false);
});

test("readJSON separates a failed parse from a null document", async () => {
  const parsed = await readJSON({ json: async () => null });
  assert.deepEqual(parsed, { ok: true, value: null }, "null is a valid JSON document");

  const failed = await readJSON({
    json: async () => {
      throw new SyntaxError("Unexpected end of JSON input");
    }
  });
  assert.deepEqual(failed, { ok: false, value: null });
});

test("DeployError carries the server's word about what started", () => {
  const refused = new DeployError("already deploying", true);
  assert.equal(refused.startedNothing, true);
  assert.equal(refused.message, "already deploying");
  assert.ok(refused instanceof Error, "so a caller that only knows Error still sees it");

  assert.equal(new DeployError("Failed to fetch", false).startedNothing, false);
});
