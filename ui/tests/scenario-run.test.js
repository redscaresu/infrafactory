import test from "node:test";
import assert from "node:assert/strict";

import { modeSummary, normalizeRunOptions } from "../src/lib/scenario-run.js";

test("normalizeRunOptions keeps no_destroy when set alone", () => {
  assert.deepEqual(normalizeRunOptions({ no_destroy: true }), {
    clean: false,
    no_destroy: true
  });
});

test("normalizeRunOptions drops no_destroy when clean is also set", () => {
  assert.deepEqual(normalizeRunOptions({ clean: true, no_destroy: true }), {
    clean: true,
    no_destroy: false
  });
});

// A caller asking for real-cloud apply must not be able to put it back on
// the wire. The server settles that at start time and ignores the field,
// so normalising it would only mislead the next reader (ADR-0026).
test("normalizeRunOptions refuses to carry a layer3 request", () => {
  assert.deepEqual(normalizeRunOptions({ layer3_enabled: true }), {
    clean: false,
    no_destroy: false
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
