import test from "node:test";
import assert from "node:assert/strict";

import {
  classifySource,
  emptyPitfall,
  selectInitialProvider,
  sourceBadgeClass,
  sourceBadgeLabel
} from "../src/lib/pitfalls-view.js";

test("classifySource recognises learned, regardless of casing", () => {
  assert.equal(classifySource("learned"), "learned");
  assert.equal(classifySource("Learned"), "learned");
  assert.equal(classifySource("  LEARNED  "), "learned");
});

test("classifySource defaults non-learned values to static", () => {
  assert.equal(classifySource("static"), "static");
  assert.equal(classifySource("seed"), "static");
  assert.equal(classifySource(""), "static");
  assert.equal(classifySource(undefined), "static");
});

test("sourceBadgeClass uses the accent palette for learned and neutral for static", () => {
  assert.match(sourceBadgeClass("learned"), /sky/);
  assert.match(sourceBadgeClass("static"), /slate/);
  assert.match(sourceBadgeClass("seed"), /slate/);
});

test("sourceBadgeLabel returns the normalised label", () => {
  assert.equal(sourceBadgeLabel("learned"), "learned");
  assert.equal(sourceBadgeLabel("static"), "static");
  assert.equal(sourceBadgeLabel("seed"), "static");
});

test("selectInitialProvider returns first provider alphabetically", () => {
  assert.equal(
    selectInitialProvider([{ provider: "scaleway" }, { provider: "gcp" }]),
    "gcp"
  );
});

test("selectInitialProvider returns empty string for empty input", () => {
  assert.equal(selectInitialProvider([]), "");
  assert.equal(selectInitialProvider(null), "");
  assert.equal(selectInitialProvider(undefined), "");
});

test("selectInitialProvider skips groups missing a provider name", () => {
  assert.equal(
    selectInitialProvider([{ provider: "" }, { provider: "scaleway" }]),
    "scaleway"
  );
});

test("emptyPitfall returns a fresh entry with the static default source", () => {
  const a = emptyPitfall();
  const b = emptyPitfall();
  assert.deepEqual(a, { resource: "", rule: "", source: "static", discovered_from: "" });
  // Distinct references so editing one entry doesn't mutate another.
  assert.notEqual(a, b);
});
