// Deliberately carries no layer3_enabled. Real-cloud apply is settled when
// the server starts (`infrafactory ui --allow-layer3`), so a request cannot
// ask for it -- ADR-0026. Normalising a field the server ignores would put
// it back on the wire and make the next reader believe it still decides
// something.
export function normalizeRunOptions(options = {}) {
  const clean = options.clean === true;
  const noDestroy = options.no_destroy === true;
  const keep = options.keep === true;
  // Orthogonal to everything else: it decides what happens if the
  // second plan is not empty, which can occur under any combination of
  // the others.
  const continueOnDrift = options.continue_on_drift === true;
  // Orthogonal to the rest: a holdout can run under any combination,
  // and its failure ends the run on its own terms.
  const holdout = options.holdout === true;

  // keep wins over no_destroy. Both skip a destroy, and they mean
  // opposite things about what happens next: keep registers what it
  // leaves running, under a TTL, with a teardown command; no_destroy
  // leaves it with nothing tracking it at all. Sending both would let
  // the server pick, on the pair where being wrong costs money.
  if (keep) {
    return { clean, no_destroy: false, keep: true, continue_on_drift: continueOnDrift, holdout };
  }

  if (clean && noDestroy) {
    return { clean: true, no_destroy: false, keep: false, continue_on_drift: continueOnDrift, holdout };
  }

  return {
    clean,
    no_destroy: noDestroy,
    keep: false,
    continue_on_drift: continueOnDrift,
    holdout
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
