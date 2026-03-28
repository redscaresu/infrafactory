import test from "node:test";
import assert from "node:assert/strict";

import { modeSummary, normalizeRunOptions } from "../src/lib/scenario-run.js";

test("normalizeRunOptions keeps no_destroy when set alone", () => {
  assert.deepEqual(normalizeRunOptions({ no_destroy: true }), {
    clean: false,
    no_destroy: true,
    layer3_enabled: false
  });
});

test("normalizeRunOptions drops no_destroy when clean is also set", () => {
  assert.deepEqual(normalizeRunOptions({ clean: true, no_destroy: true }), {
    clean: true,
    no_destroy: false,
    layer3_enabled: false
  });
});

test("normalizeRunOptions preserves layer3 flag", () => {
  assert.deepEqual(normalizeRunOptions({ layer3_enabled: true }), {
    clean: false,
    no_destroy: false,
    layer3_enabled: true
  });
});

test("modeSummary reports incremental mode", () => {
  assert.deepEqual(
    modeSummary({
      mode: "incremental",
      reason: "auto-detected from mockway state, terraform.tfstate, and previous successful run"
    }),
    {
      title: "Incremental run",
      detail: "auto-detected from mockway state, terraform.tfstate, and previous successful run",
      tone: "incremental"
    }
  );
});

test("modeSummary reports clean fallback when mode is missing", () => {
  assert.deepEqual(modeSummary(null), {
    title: "Run mode unavailable",
    detail: "Mode detection has not completed yet.",
    tone: "neutral"
  });
});
