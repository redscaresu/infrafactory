import test from "node:test";
import assert from "node:assert/strict";

import {
  buildRunBaselineURL,
  buildRunPlanURL,
  compareRunIDs,
  deriveFailureHint,
  deriveLiveConsoleNotice,
  filterRuns,
  formatBaselineState,
  formatRunDate,
  mergeConsoleLines,
  selectLatestRun,
  synthesizeLiveConsoleLines
} from "../src/lib/run-view.js";

test("buildRunPlanURL encodes scenario and run id", () => {
  assert.equal(buildRunPlanURL("web app", "run/1"), "/api/runs/web%20app/run%2F1/plan");
});

test("buildRunBaselineURL encodes scenario and run id", () => {
  assert.equal(buildRunBaselineURL("web app", "run/1"), "/api/runs/web%20app/run%2F1/baseline");
});

test("selectLatestRun prefers running run within a scenario", () => {
  const runs = [
    { scenario: "web-app-paris", run_id: "20260228T100000Z", status: "failed" },
    { scenario: "web-app-paris", run_id: "20260228T110000Z", status: "running" },
    { scenario: "other", run_id: "20260228T120000Z", status: "success" }
  ];

  const selected = selectLatestRun(runs, "web-app-paris");
  assert.equal(selected?.run_id, "20260228T110000Z");
});

test("selectLatestRun falls back to newest run when none are running", () => {
  const runs = [
    { scenario: "web-app-paris", run_id: "20260228T100000Z", status: "failed" },
    { scenario: "web-app-paris", run_id: "20260228T110000Z", status: "success" }
  ];

  const selected = selectLatestRun(runs, "web-app-paris");
  assert.equal(selected?.run_id, "20260228T110000Z");
});

test("filterRuns applies search and status filters", () => {
  const runs = [
    { scenario: "web-app-paris", run_id: "20260228T100000Z", status: "success", terminal_reason: "target_reached" },
    { scenario: "iam-policies-paris", run_id: "20260228T090000Z", status: "failed", terminal_reason: "repair_budget_exhausted" }
  ];

  assert.equal(filterRuns(runs, "web-app", "all").length, 1);
  assert.equal(filterRuns(runs, "", "failed").length, 1);
  assert.equal(filterRuns(runs, "repair_budget", "failed").length, 1);
});

test("deriveFailureHint explains known transport failures", () => {
  assert.match(deriveFailureHint('generate code: generator transport failed: phase "plan_architecture": run "claude": context canceled'), /context was canceled/i);
  assert.match(deriveFailureHint("openrouter unavailable: OPENROUTER_API_KEY is not set"), /OpenRouter API key/i);
  assert.equal(compareRunIDs("20260228T120000Z", "20260228T110000Z"), -1);
});

test("formatRunDate renders ISO timestamps and preserves invalid values", () => {
  assert.equal(formatRunDate("2026-02-28T11:25:00Z"), "2026-02-28 11:25:00Z");
  assert.equal(formatRunDate("not-a-date"), "not-a-date");
  assert.equal(formatRunDate(""), "-");
});

test("deriveLiveConsoleNotice reflects terminal run state when no websocket events exist", () => {
  assert.equal(deriveLiveConsoleNotice(null, [], []), "No active run.");
  assert.equal(deriveLiveConsoleNotice({ status: "running" }, [], []), "Waiting for run events...");
  assert.match(
    deriveLiveConsoleNotice({ status: "success" }, [{ iteration: 1 }], []),
    /iteration artifacts were loaded/i
  );
  assert.equal(deriveLiveConsoleNotice({ status: "success" }, [], ["x"]), "");
});

test("synthesizeLiveConsoleLines falls back to run metadata and iteration artifacts", () => {
  const lines = synthesizeLiveConsoleLines(
    { run_id: "20260228T125323Z", status: "running", terminal_reason: "" },
    [
      {
        iteration: 1,
        statuses: [{ stage: "iteration_1_generate", status: "success" }],
        failures: []
      }
    ],
    []
  );

  assert.equal(lines[0], "run_id=20260228T125323Z status=running terminal_reason=-");
  assert.match(lines[1], /iteration=1 statuses=iteration_1_generate=success/);
});

test("synthesizeLiveConsoleLines preserves websocket output when present", () => {
  const lines = synthesizeLiveConsoleLines(
    { run_id: "20260228T125323Z", status: "running" },
    [{ iteration: 1 }],
    ['{"event":"run_start"}']
  );

  assert.deepEqual(lines, ['{"event":"run_start"}']);
});

test("mergeConsoleLines appends live lines after replay lines without duplication", () => {
  const merged = mergeConsoleLines(
    ['{"event":"run_start"}', '{"event":"iteration_start"}'],
    ['{"event":"iteration_start"}', '{"event":"stage_start"}']
  );

  assert.deepEqual(merged, ['{"event":"run_start"}', '{"event":"iteration_start"}', '{"event":"stage_start"}']);
});

test("formatBaselineState pretty prints valid json and preserves invalid text", () => {
  assert.equal(formatBaselineState('{"instance":{"servers":[{"id":"srv-1"}]}}'), '{\n  "instance": {\n    "servers": [\n      {\n        "id": "srv-1"\n      }\n    ]\n  }\n}');
  assert.equal(formatBaselineState("not-json"), "not-json");
});
