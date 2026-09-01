// Deliberately carries no layer3_enabled. Real-cloud apply is settled when
// the server starts (`infrafactory ui --allow-layer3`), so a request cannot
// ask for it -- ADR-0026. Normalising a field the server ignores would put
// it back on the wire and make the next reader believe it still decides
// something.
export function normalizeRunOptions(options = {}) {
  const clean = options.clean === true;
  const noDestroy = options.no_destroy === true;

  if (clean && noDestroy) {
    return { clean: true, no_destroy: false };
  }

  return {
    clean,
    no_destroy: noDestroy
  };
}

export function modeTone(mode) {
  return mode === "incremental" ? "incremental" : "clean";
}

export function modeSummary(runMode) {
  if (!runMode) {
    return {
      title: "Run mode unavailable",
      detail: "Mode detection has not completed yet.",
      tone: "neutral"
    };
  }

  if (runMode.mode === "incremental") {
    return {
      title: "Incremental run",
      detail: runMode.reason,
      tone: "incremental"
    };
  }

  return {
    title: "Clean run",
    detail: runMode.reason,
    tone: "clean"
  };
}
