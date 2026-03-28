export function normalizeRunOptions(options = {}) {
  const clean = options.clean === true;
  const noDestroy = options.no_destroy === true;
  const layer3Enabled = options.layer3_enabled === true;

  if (clean && noDestroy) {
    return { clean: true, no_destroy: false, layer3_enabled: layer3Enabled };
  }

  return {
    clean,
    no_destroy: noDestroy,
    layer3_enabled: layer3Enabled
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
