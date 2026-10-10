// Pure helpers for the /pitfalls page so the rendering logic can be unit tested.

const NEUTRAL_BADGE = "bg-slate-200 text-slate-800";
const ACCENT_BADGE = "bg-sky-100 text-sky-900";

// classifySource normalises the free-form source string from the backend into
// either "learned" or "static". Anything other than "learned" (case-insensitive)
// is treated as static, which matches the backend default.
export function classifySource(source) {
  if (typeof source === "string" && source.trim().toLowerCase() === "learned") {
    return "learned";
  }
  return "static";
}

// sourceBadgeClass returns the Tailwind class string for a pitfall source
// badge. "learned" entries are highlighted with an accent colour; everything
// else (static / seed / unknown) renders with the neutral pill style used
// elsewhere in the UI.
export function sourceBadgeClass(source) {
  return classifySource(source) === "learned" ? ACCENT_BADGE : NEUTRAL_BADGE;
}

// sourceBadgeLabel returns the display label for a source badge. The backend
// already lowercases its values, but we normalise here so empty / unknown
// values render as "static" (the backend default).
export function sourceBadgeLabel(source) {
  return classifySource(source);
}

// selectInitialProvider returns the provider that should be selected by
// default when the page first loads: the first provider alphabetically, or
// an empty string if no providers are present.
export function selectInitialProvider(providers) {
  if (!Array.isArray(providers) || providers.length === 0) {
    return "";
  }
  const names = providers
    .map((p) => (p && typeof p.provider === "string" ? p.provider : ""))
    .filter((name) => name !== "");
  if (names.length === 0) {
    return "";
  }
  names.sort((a, b) => (a < b ? -1 : a > b ? 1 : 0));
  return names[0];
}

// emptyPitfall returns a fresh, blank pitfall entry suitable for appending
// to the editing list when the user clicks "+ Add". New entries default to
// the static source.
export function emptyPitfall() {
  return { resource: "", rule: "", source: "static", discovered_from: "" };
}

// saveStatusMessage reports a save, and says when the server scrubbed
// account ids from what was submitted: the reloaded editor then shows
// different text, and it should not change silently.
export function saveStatusMessage(resp) {
  const count = resp?.count ?? 0;
  let message = `Saved ${count} pitfall${count === 1 ? "" : "s"}.`;
  const scrubbed = resp?.scrubbed ?? 0;
  if (scrubbed > 0) {
    message += ` Account ids were replaced with ACCOUNT_ID in ${scrubbed} field${scrubbed === 1 ? "" : "s"}.`;
  }
  return message;
}
