// Plain-JS fetch helpers for the /pitfalls page. Kept separate from api.ts
// so the request shape can be unit tested with node:test without resolving
// SvelteKit's $lib/* path aliases.

export async function fetchPitfalls(fetchImpl = globalThis.fetch) {
  const res = await fetchImpl("/api/pitfalls");
  if (!res.ok) {
    const text = await safeText(res);
    throw new Error(text || `request failed: ${res.status}`);
  }
  return res.json();
}

export async function fetchSavePitfalls(provider, pitfalls, fetchImpl = globalThis.fetch) {
  if (!provider) {
    throw new Error("provider is required");
  }
  const res = await fetchImpl(`/api/pitfalls/${encodeURIComponent(provider)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ pitfalls })
  });
  if (!res.ok) {
    const text = await safeText(res);
    throw new Error(text || `request failed: ${res.status}`);
  }
  return res.json();
}

async function safeText(res) {
  try {
    const ctype = res.headers?.get?.("content-type") || "";
    if (ctype.includes("application/json")) {
      const payload = await res.json();
      if (payload && typeof payload === "object" && payload.error) {
        return String(payload.error);
      }
      return "";
    }
    return await res.text();
  } catch {
    return "";
  }
}
