<script lang="ts">
  import { onMount } from "svelte";
  import { api } from "$lib/api";
  import {
    emptyPitfall,
    selectInitialProvider,
    sourceBadgeClass,
    sourceBadgeLabel
  } from "$lib/pitfalls-view.js";
  import type { Pitfall, PitfallProviderGroup, PitfallsResponse } from "$lib/types";

  // Editable state is keyed by provider name so each provider keeps its
  // own working copy and save status independent of the others.
  let providers: string[] = [];
  let entriesByProvider: Record<string, Pitfall[]> = {};
  let selectedProvider = "";
  let loadError = "";
  let saveStatus: Record<string, string> = {};
  let saving: Record<string, boolean> = {};

  function cloneEntry(entry: Pitfall): Pitfall {
    return {
      resource: entry.resource || "",
      rule: entry.rule || "",
      source: entry.source || "static",
      discovered_from: entry.discovered_from || ""
    };
  }

  function ingest(payload: PitfallsResponse) {
    const groups: PitfallProviderGroup[] = payload?.providers || [];
    providers = groups.map((g) => g.provider).filter((p): p is string => !!p);
    const next: Record<string, Pitfall[]> = {};
    for (const group of groups) {
      next[group.provider] = (group.pitfalls || []).map(cloneEntry);
    }
    entriesByProvider = next;
    selectedProvider = selectInitialProvider(groups);
  }

  async function load() {
    loadError = "";
    try {
      const payload = await api.getPitfalls();
      ingest(payload);
    } catch (err) {
      loadError = err instanceof Error ? err.message : "Failed to load pitfalls";
      providers = [];
      entriesByProvider = {};
      selectedProvider = "";
    }
  }

  function selectProvider(provider: string) {
    selectedProvider = provider;
  }

  function addEntry(provider: string) {
    const list = entriesByProvider[provider] || [];
    entriesByProvider = {
      ...entriesByProvider,
      [provider]: [...list, emptyPitfall()]
    };
  }

  function deleteEntry(provider: string, index: number) {
    const list = entriesByProvider[provider] || [];
    const next = list.slice();
    next.splice(index, 1);
    entriesByProvider = { ...entriesByProvider, [provider]: next };
  }

  function updateEntry(provider: string, index: number, field: keyof Pitfall, value: string) {
    const list = entriesByProvider[provider] || [];
    const next = list.slice();
    if (!next[index]) return;
    next[index] = { ...next[index], [field]: value };
    entriesByProvider = { ...entriesByProvider, [provider]: next };
  }

  async function saveProvider(provider: string) {
    saveStatus = { ...saveStatus, [provider]: "" };
    saving = { ...saving, [provider]: true };
    const list = (entriesByProvider[provider] || []).map((entry) => ({
      resource: entry.resource.trim(),
      rule: entry.rule.trim(),
      source: entry.source.trim() || "static",
      discovered_from: entry.discovered_from?.trim() || ""
    }));
    try {
      const resp = await api.savePitfalls(provider, list);
      saveStatus = { ...saveStatus, [provider]: `Saved ${resp.count} pitfall${resp.count === 1 ? "" : "s"}.` };
      // Re-load to pick up any backend-side normalisation (e.g. default source).
      await load();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Save failed";
      saveStatus = { ...saveStatus, [provider]: `Save failed: ${message}` };
    } finally {
      saving = { ...saving, [provider]: false };
    }
  }

  onMount(load);

  $: currentEntries = entriesByProvider[selectedProvider] || [];
  $: currentStatus = saveStatus[selectedProvider] || "";
  $: currentSaving = !!saving[selectedProvider];
  $: sortedProviders = [...providers].sort();
</script>

<h1 class="text-2xl font-bold text-slate-900">Pitfalls</h1>
<p class="mt-2 text-slate-600">
  Static and learned guidance the generator consults when planning infrastructure for each
  provider. Edits are saved to <code>pitfalls/&lt;provider&gt;.yaml</code>.
</p>

{#if loadError}
  <p class="mt-4 text-sm text-red-700" data-testid="pitfalls-load-error">{loadError}</p>
{/if}

{#if sortedProviders.length === 0 && !loadError}
  <p class="mt-4 text-sm text-slate-600">No pitfalls files were found.</p>
{/if}

{#if sortedProviders.length > 0}
  <div class="mt-4 flex flex-wrap gap-2" role="tablist" aria-label="Pitfall providers">
    {#each sortedProviders as provider}
      <button
        type="button"
        role="tab"
        aria-selected={provider === selectedProvider}
        class={`rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.16em] ${
          provider === selectedProvider
            ? "bg-slate-900 text-white"
            : "bg-slate-200 text-slate-800 hover:bg-slate-300"
        }`}
        data-testid={`pitfalls-tab-${provider}`}
        on:click={() => selectProvider(provider)}
      >
        {provider}
      </button>
    {/each}
  </div>

  <section class="mt-4" data-testid="pitfalls-section" data-provider={selectedProvider}>
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h2 class="text-lg font-semibold text-slate-900">{selectedProvider}</h2>
      <div class="flex gap-2">
        <button
          type="button"
          class="rounded border border-slate-400 px-3 py-1.5 text-xs text-slate-900 hover:bg-slate-100"
          data-testid="pitfalls-add"
          on:click={() => addEntry(selectedProvider)}
        >
          + Add
        </button>
        <button
          type="button"
          class="rounded bg-slate-900 px-3 py-1.5 text-xs text-white disabled:opacity-60"
          data-testid="pitfalls-save"
          on:click={() => saveProvider(selectedProvider)}
          disabled={currentSaving}
        >
          {currentSaving ? "Saving..." : "Save"}
        </button>
      </div>
    </div>

    {#if currentStatus}
      <p
        class={`mt-3 text-sm ${currentStatus.startsWith("Save failed") ? "text-red-700" : "text-emerald-700"}`}
        data-testid="pitfalls-save-status"
      >
        {currentStatus}
      </p>
    {/if}

    <table class="mt-4 w-full border-collapse text-sm">
      <thead>
        <tr class="text-left text-slate-500">
          <th class="pb-2 pr-3 w-48">Resource</th>
          <th class="pb-2 pr-3">Rule</th>
          <th class="pb-2 pr-3 w-28">Source</th>
          <th class="pb-2 pr-3 w-48">Discovered From</th>
          <th class="pb-2 w-20"></th>
        </tr>
      </thead>
      <tbody>
        {#each currentEntries as entry, index (index)}
          <tr class="border-t border-slate-200 align-top" data-testid="pitfalls-row">
            <td class="py-2 pr-3">
              <input
                class="w-full rounded border border-slate-300 bg-white px-2 py-1 font-mono text-xs"
                aria-label="Resource"
                bind:value={entry.resource}
                on:input={(e) => updateEntry(selectedProvider, index, "resource", (e.target as HTMLInputElement).value)}
              />
            </td>
            <td class="py-2 pr-3">
              <textarea
                class="min-h-[3.5rem] w-full rounded border border-slate-300 bg-white px-2 py-1 text-xs"
                aria-label="Rule"
                bind:value={entry.rule}
                on:input={(e) => updateEntry(selectedProvider, index, "rule", (e.target as HTMLTextAreaElement).value)}
              ></textarea>
            </td>
            <td class="py-2 pr-3">
              <div class="flex flex-col gap-1">
                <span
                  class={`inline-flex w-fit rounded-full px-2 py-0.5 text-[0.65rem] font-semibold uppercase tracking-[0.14em] ${sourceBadgeClass(entry.source)}`}
                  data-testid="pitfalls-source-badge"
                >
                  {sourceBadgeLabel(entry.source)}
                </span>
                <select
                  class="rounded border border-slate-300 bg-white px-2 py-1 text-xs"
                  aria-label="Source"
                  bind:value={entry.source}
                  on:change={(e) => updateEntry(selectedProvider, index, "source", (e.target as HTMLSelectElement).value)}
                >
                  <option value="static">static</option>
                  <option value="learned">learned</option>
                </select>
              </div>
            </td>
            <td class="py-2 pr-3">
              <input
                class="w-full rounded border border-slate-300 bg-white px-2 py-1 text-xs"
                aria-label="Discovered from"
                bind:value={entry.discovered_from}
                on:input={(e) => updateEntry(selectedProvider, index, "discovered_from", (e.target as HTMLInputElement).value)}
              />
            </td>
            <td class="py-2">
              <button
                type="button"
                class="rounded border border-red-400 px-2 py-1 text-xs text-red-700 hover:bg-red-50"
                data-testid="pitfalls-delete"
                on:click={() => deleteEntry(selectedProvider, index)}
              >
                Delete
              </button>
            </td>
          </tr>
        {/each}
        {#if currentEntries.length === 0}
          <tr>
            <td class="py-3 text-sm text-slate-600" colspan="5">
              No pitfalls for this provider yet. Click "+ Add" to create one.
            </td>
          </tr>
        {/if}
      </tbody>
    </table>
  </section>
{/if}
