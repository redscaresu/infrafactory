import test from "node:test";
import assert from "node:assert/strict";

import {
  buildLineDiff,
  buildRunArtifactsURL,
  buildRunBundleURL,
  buildSnapshotLabel,
  buildSnapshotOptions,
  getDefaultCompareSnapshot,
  highlightHCL
} from "../src/lib/iac-view.js";

test("buildRunBundleURL encodes scenario and run id", () => {
  assert.equal(buildRunBundleURL("web app/paris", "2026:run"), "/api/runs/web%20app%2Fparis/2026%3Arun/bundle.zip");
});

test("buildRunArtifactsURL encodes scenario and run id", () => {
  assert.equal(buildRunArtifactsURL("web app/paris", "2026:run"), "/api/runs/web%20app%2Fparis/2026%3Arun/artifacts.zip");
});

test("buildSnapshotLabel renders final and iteration labels", () => {
  assert.equal(buildSnapshotLabel("final"), "Final output");
  assert.equal(buildSnapshotLabel(3), "Iteration 3");
});

test("buildSnapshotOptions and default compare snapshot are deterministic", () => {
  assert.deepEqual(buildSnapshotOptions([1, 2, 3]), ["final", 3, 2, 1]);
  assert.equal(getDefaultCompareSnapshot(3, [1, 2, 3]), 2);
  assert.equal(getDefaultCompareSnapshot(1, [1, 2, 3]), "final");
  assert.equal(getDefaultCompareSnapshot("final", [1, 2, 3]), 3);
});

test("highlightHCL tokenizes keywords strings numbers comments interpolation and functions", () => {
  const highlighted = highlightHCL('resource "scaleway_vpc" "main" {\n  name = "vpc-${var.name}"\n  cidr = cidrsubnet(var.base, 4, 2)\n}\n# comment');

  assert.equal(highlighted.length, 5);
  assert.equal(highlighted[0][0].className, "token-keyword");
  assert.equal(highlighted[1].some((token) => token.className === "token-attribute"), true);
  assert.equal(highlighted[1].some((token) => token.className === "token-interpolation"), true);
  assert.equal(highlighted[2].some((token) => token.className === "token-function"), true);
  assert.equal(highlighted[2].some((token) => token.className === "token-number"), true);
  assert.equal(highlighted[4][0].className, "token-comment");
});

test("buildLineDiff marks additions removals and context", () => {
  const diff = buildLineDiff("a\nb\nc", "a\nb2\nc\nd");

  assert.equal(diff.some((row) => row.type === "remove" && row.before === "b"), true);
  assert.equal(diff.some((row) => row.type === "add" && row.after === "b2"), true);
  assert.equal(diff.some((row) => row.type === "add" && row.after === "d"), true);
  assert.equal(diff.some((row) => row.type === "context" && row.after === "a"), true);
});
