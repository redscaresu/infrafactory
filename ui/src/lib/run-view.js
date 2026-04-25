export function compareRunIDs(a, b) {
  if (a === b) return 0;
  return a < b ? 1 : -1;
}

export function buildRunPlanURL(scenario, runID) {
  return `/api/runs/${encodeURIComponent(scenario)}/${encodeURIComponent(runID)}/plan`;
}

export function buildRunBaselineURL(scenario, runID) {
  return `/api/runs/${encodeURIComponent(scenario)}/${encodeURIComponent(runID)}/baseline`;
}

export function selectLatestRun(runs, scenario = "") {
  const filtered = scenario ? runs.filter((run) => run.scenario === scenario) : [...runs];
  if (filtered.length === 0) {
    return null;
  }
  filtered.sort((a, b) => compareRunIDs(a.run_id, b.run_id));
  return filtered.find((run) => run.status === "running") || filtered[0];
}

export function filterRuns(runs, search, statusFilter) {
  const query = search.trim().toLowerCase();
  return runs.filter((run) => {
    const matchesQuery =
      query === "" ||
      run.scenario.toLowerCase().includes(query) ||
      run.run_id.toLowerCase().includes(query) ||
      (run.terminal_reason || "").toLowerCase().includes(query);
    const matchesStatus = statusFilter === "all" || run.status === statusFilter;
    return matchesQuery && matchesStatus;
  });
}

export function formatRunDate(value) {
  if (!value) {
    return "-";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toISOString().replace("T", " ").replace(".000Z", "Z");
}

export function deriveFailureHint(detail) {
  if (detail.includes("context canceled")) {
    return "The generator process started but its context was canceled before completion. If this repeats after the latest backend restart, inspect backend logs for a secondary cancellation path.";
  }
  if (detail.includes("not found in PATH")) {
    return "The configured generator command is missing from the backend runtime PATH.";
  }
  if (detail.includes("OPENROUTER_API_KEY")) {
    return "The backend runtime is missing the OpenRouter API key.";
  }
  if (detail.includes("transport failed")) {
    return "The generator runtime is reachable, but the transport failed during execution. Check CLI auth, retries, and backend logs.";
  }
  return "";
}

export function deriveLiveConsoleNotice(runMeta, iterations, lines) {
  if (lines.length > 0) {
    return "";
  }
  if (!runMeta) {
    return "No active run.";
  }
  if (runMeta.status === "running") {
    return "Waiting for run events...";
  }
  if (iterations.length > 0) {
    return `Run ${runMeta.status}. No live events were captured, but iteration artifacts were loaded.`;
  }
  return `Run ${runMeta.status}. No live events or iteration artifacts were captured.`;
}

export function synthesizeLiveConsoleLines(runMeta, iterations, lines) {
  if (lines.length > 0) {
    return lines;
  }
  if (!runMeta) {
    return [];
  }

  const synthetic = [];
  synthetic.push(`run_id=${runMeta.run_id || "-"} status=${runMeta.status || "unknown"} terminal_reason=${runMeta.terminal_reason || "-"}`);
  for (const iteration of iterations) {
    const statuses = (iteration.statuses || [])
      .map((status) => `${status.stage || "unknown"}=${status.status || "unknown"}`)
      .join(", ");
    if (statuses) {
      synthetic.push(`iteration=${iteration.iteration || "?"} statuses=${statuses}`);
    }
    for (const failure of iteration.failures || []) {
      synthetic.push(
        `iteration=${iteration.iteration || "?"} failure stage=${failure.stage || "unknown"} check=${failure.check || "unknown"} detail=${failure.detail || "No detail provided"}`
      );
    }
  }
  return synthetic;
}

export function mergeConsoleLines(historyLines, liveLines) {
  if (liveLines.length === 0) {
    return historyLines;
  }
  const seen = new Set(historyLines);
  const merged = [...historyLines];
  for (const line of liveLines) {
    if (!seen.has(line)) {
      merged.push(line);
      seen.add(line);
    }
  }
  return merged;
}

export function deriveCurrentIteration(runMeta, iterations) {
  if (!runMeta || runMeta.status !== "running") return 0;
  const completedIterations = iterations.length;
  const lastIteration = iterations[iterations.length - 1];
  if (lastIteration && (lastIteration.failures?.length || lastIteration.stages?.length)) {
    return completedIterations + 1;
  }
  return Math.max(completedIterations, 1);
}

export function deriveCurrentStage(consoleLines) {
  for (let i = consoleLines.length - 1; i >= 0; i--) {
    const line = consoleLines[i];
    try {
      const parsed = JSON.parse(line.startsWith('{') ? line : '{}');
      if (parsed.event === "stage_start" && parsed.status === "start") {
        return parsed.stage?.replace(/^iteration_\d+_/, '') || "";
      }
    } catch { /* ignore non-JSON lines */ }
  }
  return "";
}

export function formatBaselineState(payload) {
  if (!payload) {
    return "";
  }
  try {
    return JSON.stringify(JSON.parse(payload), null, 2);
  } catch {
    return payload;
  }
}

/**
 * Returns true when a completed run needs one final data reload to pick up
 * iterations that were written between the last poll and terminal detection.
 */
export function needsFinalReload(runMeta, finalReloadDone) {
  if (!runMeta || !runMeta.status) return false;
  if (runMeta.status === "running") return false;
  return !finalReloadDone;
}
