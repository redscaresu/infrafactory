import test from "node:test";
import assert from "node:assert/strict";

import {
  buildRunBaselineURL,
  buildRunPlanURL,
  compareRunIDs,
  deriveCurrentIteration,
  deriveCurrentStage,
  deriveFailureHint,
  deriveLiveConsoleNotice,
  filterRuns,
  formatBaselineState,
  formatRunDate,
  mergeConsoleLines,
  needsFinalReload,
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

test("deriveCurrentIteration returns 0 when run is not running", () => {
  assert.equal(deriveCurrentIteration(null, []), 0);
  assert.equal(deriveCurrentIteration({ status: "success" }, []), 0);
  assert.equal(deriveCurrentIteration({ status: "failed" }, [{ iteration: 1, failures: [{ detail: "x" }] }]), 0);
});

test("deriveCurrentIteration returns 1 when running with no completed iterations", () => {
  assert.equal(deriveCurrentIteration({ status: "running" }, []), 1);
});

test("deriveCurrentIteration returns N+1 when N iterations completed with failures", () => {
  const iterations = [
    { iteration: 1, failures: [{ detail: "something broke" }], stages: [] },
    { iteration: 2, failures: [{ detail: "still broken" }], stages: [] }
  ];
  assert.equal(deriveCurrentIteration({ status: "running" }, iterations), 3);
});

test("deriveCurrentIteration returns N+1 when last iteration has stages but no failures", () => {
  const iterations = [
    { iteration: 1, stages: [{ stage: "generate", status: "success" }], failures: [] }
  ];
  assert.equal(deriveCurrentIteration({ status: "running" }, iterations), 2);
});

test("deriveCurrentIteration returns iterations.length when last iteration is empty", () => {
  const iterations = [
    { iteration: 1, failures: [{ detail: "x" }], stages: [] },
    { iteration: 2 }
  ];
  assert.equal(deriveCurrentIteration({ status: "running" }, iterations), 2);
});

test("deriveCurrentStage returns empty string when no console lines", () => {
  assert.equal(deriveCurrentStage([]), "");
});

test("deriveCurrentStage returns stage name from last stage_start event", () => {
  const lines = [
    '{"event":"stage_start","stage":"iteration_1_plan_architecture","status":"start"}',
    '{"event":"stage_start","stage":"iteration_1_generate","status":"start"}'
  ];
  assert.equal(deriveCurrentStage(lines), "generate");
});

test("deriveCurrentStage strips iteration prefix", () => {
  const lines = [
    '{"event":"stage_start","stage":"iteration_2_generate","status":"start"}'
  ];
  assert.equal(deriveCurrentStage(lines), "generate");
});

test("deriveCurrentStage ignores non-JSON lines", () => {
  const lines = [
    '{"event":"stage_start","stage":"iteration_1_plan_architecture","status":"start"}',
    "some plain text log line",
    "another non-json line"
  ];
  assert.equal(deriveCurrentStage(lines), "plan_architecture");
});

test("deriveCurrentStage ignores stage events that are not status=start", () => {
  const lines = [
    '{"event":"stage_start","stage":"iteration_1_generate","status":"start"}',
    '{"event":"stage_end","stage":"iteration_1_generate","status":"success"}'
  ];
  assert.equal(deriveCurrentStage(lines), "generate");
});

test("deriveCurrentStage returns stage without prefix when stage has no iteration prefix", () => {
  const lines = [
    '{"event":"stage_start","stage":"validate","status":"start"}'
  ];
  assert.equal(deriveCurrentStage(lines), "validate");
});

// needsFinalReload tests — regression for iteration timeline not updating after run completes

test("needsFinalReload returns false when runMeta is null", () => {
  assert.equal(needsFinalReload(null, false), false);
});

test("needsFinalReload returns false when run is still running", () => {
  assert.equal(needsFinalReload({ status: "running" }, false), false);
});

test("needsFinalReload returns true when run completed and no reload done yet", () => {
  assert.equal(needsFinalReload({ status: "failed" }, false), true);
  assert.equal(needsFinalReload({ status: "success" }, false), true);
});

test("needsFinalReload returns false after final reload is done", () => {
  assert.equal(needsFinalReload({ status: "failed" }, true), false);
  assert.equal(needsFinalReload({ status: "success" }, true), false);
});
