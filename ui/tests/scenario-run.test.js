import test from "node:test";
import assert from "node:assert/strict";

import { modeSummary, normalizeRunOptions } from "../src/lib/scenario-run.js";

test("normalizeRunOptions keeps no_destroy when set alone", () => {
  assert.deepEqual(normalizeRunOptions({ no_destroy: true }), {
    clean: false,
    no_destroy: true,
    keep: false
  });
});

test("normalizeRunOptions drops no_destroy when clean is also set", () => {
  assert.deepEqual(normalizeRunOptions({ clean: true, no_destroy: true }), {
    clean: true,
    no_destroy: false,
    keep: false
  });
});

// A caller asking for real-cloud apply must not be able to put it back on
// the wire. The server settles that at start time and ignores the field,
// so normalising it would only mislead the next reader (ADR-0026).
test("normalizeRunOptions refuses to carry a layer3 request", () => {
  assert.deepEqual(normalizeRunOptions({ layer3_enabled: true }), {
    clean: false,
    no_destroy: false,
    keep: false
  });
});

test("normalizeRunOptions carries keep", () => {
  assert.deepEqual(normalizeRunOptions({ keep: true }), {
    clean: false,
    no_destroy: false,
    keep: true
  });
});

// Both skip a destroy and mean opposite things afterwards: keep
// registers what it leaves running, under a TTL; no_destroy leaves it
// with nothing tracking it. Sending both would let the server pick, on
// the pair where being wrong costs money.
test("normalizeRunOptions drops no_destroy when keep is also set", () => {
  assert.deepEqual(normalizeRunOptions({ keep: true, no_destroy: true }), {
    clean: false,
    no_destroy: false,
    keep: true
  });
});

// clean is orthogonal: it decides how the run STARTS, keep decides what
// survives it.
test("normalizeRunOptions allows clean alongside keep", () => {
  assert.deepEqual(normalizeRunOptions({ clean: true, keep: true }), {
    clean: true,
    no_destroy: false,
    keep: true
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
