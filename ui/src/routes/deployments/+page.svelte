<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { api } from "$lib/api";
  import {
    addressHref,
    addressLabel,
    estateSummary,
    healthBadge,
    knownEmpty,
    needsAttention,
    observedLabel,
    teardownOutcome,
    teardownPrompt,
    ttlLabel,
    versionBadge
  } from "$lib/deployments-view.js";
  import type { Deployment, DeploymentsResponse } from "$lib/types";

  let deployments: Deployment[] = [];
  let unreadable: string[] = [];
  let teardownAllowed = false;
  // Scenarios applying right now.
  //
  // They have NO record yet -- registerDeployment runs after the apply
  // returns -- so they cannot appear in the table below. Without this
  // the page that is meant to answer "what is running" is silent about
  // the thing most actively running, and a reader who has just clicked
  // Deploy elsewhere sees nothing at all.
  let deploying: string[] = [];

  // Confirming is a SECOND deliberate action on a named row, not a
  // dialog that appears everywhere at once. A click cannot destroy
  // anything on its own.
  let confirming = "";
  let destroying = "";
  let outcomes: Record<string, { ok: boolean; message: string }> = {};
  let loadError = "";
  let loaded = false;
  let timer: ReturnType<typeof setInterval> | undefined;

  async function load() {
    try {
      const payload: DeploymentsResponse = await api.getDeployments();
      deployments = payload?.deployments || [];
      unreadable = payload?.unreadable || [];
      teardownAllowed = payload?.teardown_allowed === true;
      deploying = payload?.deploying || [];
      loadError = "";
    } catch (err) {
      // The previous rows are KEPT on error rather than cleared. An
      // empty table reads as "nothing is running", and a failed refresh
      // is not evidence that the estate is empty -- it is evidence that
      // we do not know. The banner says which.
      loadError = err instanceof Error ? err.message : "Could not read the live estate";
    } finally {
      loaded = true;
    }
  }

  async function destroy(d: Deployment) {
    confirming = "";
    destroying = d.id;
    try {
      const result = await api.tearDownDeployment(d.id);
      outcomes = { ...outcomes, [d.id]: teardownOutcome(result) };
    } catch (err) {
      outcomes = {
        ...outcomes,
        [d.id]: {
          ok: false,
          message:
            err instanceof Error
              ? `Teardown could not be completed: ${err.message}`
              : "Teardown could not be completed."
        }
      };
    } finally {
      destroying = "";
      // Reload whatever the outcome. A failed teardown changes the
      // record too -- ADR-0024 keeps an unreclaimable deployment
      // reapable rather than released -- so the table must not keep
      // showing what was true before the attempt.
      await load();
    }
  }

  onMount(() => {
    load();
    // TTLs count down and observations arrive out of band, so a page
    // left open goes stale silently. Thirty seconds is far below the
    // shortest TTL anyone would set.
    timer = setInterval(load, 30_000);
  });

  onDestroy(() => {
    if (timer) clearInterval(timer);
  });

  // The three states are distinct on purpose: an empty list means
  // "nothing is deployed" ONLY when the read succeeded.
  $: estateState = !loaded ? "loading" : loadError ? "failed" : "loaded";
  $: summary = estateSummary(deployments, unreadable, estateState, deploying);
  // The one condition under which this page may say nothing is running.
  // Shared with the summary rather than re-derived, because two copies
  // is how one of them ends up contradicting the other.
  $: estateKnownEmpty = knownEmpty(deployments, unreadable, estateState, deploying);
</script>

<svelte:head><title>Deployments · InfraFactory</title></svelte:head>

<section class="space-y-4">
  <div>
    <h1 class="text-2xl font-bold text-slate-900">Deployments</h1>
    <p class="mt-1 text-sm text-slate-600" data-testid="estate-summary">
      {summary}
    </p>
  </div>

  {#if loadError}
    <!-- Never silent. A page that cannot read the estate must not look
         like a page reading an empty estate. -->
    <div
      class="rounded border border-rose-300 bg-rose-50 px-4 py-3 text-sm text-rose-900"
      data-testid="estate-load-error"
    >
      <p class="font-semibold">The live estate could not be read.</p>
      <p class="mt-1">{loadError}</p>
      <p class="mt-1">
        Anything below was read earlier and may be out of date. This is not evidence that
        nothing is running.
      </p>
    </div>
  {/if}

  {#if deploying.length > 0}
    <div
      class="rounded border border-sky-300 bg-sky-50 px-4 py-3 text-sm text-sky-900"
      data-testid="estate-deploying"
    >
      <p class="font-semibold">
        {deploying.length} deploy{deploying.length === 1 ? "" : "s"} in progress.
      </p>
      <p class="mt-1">
        Applying now, so {deploying.length === 1 ? "it has" : "they have"} no record yet and
        {deploying.length === 1 ? "does" : "do"} not appear below: {deploying.join(", ")}.
      </p>
    </div>
  {/if}

  {#if unreadable.length > 0}
    <!-- A record that will not decode may describe running, billing
         infrastructure. `live ls` exits non-zero for this; a page has to
         show it. -->
    <div
      class="rounded border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900"
      data-testid="estate-unreadable"
    >
      <p class="font-semibold">
        {unreadable.length} live record{unreadable.length === 1 ? "" : "s"} could not be read.
      </p>
      <p class="mt-1">
        Each may describe infrastructure that is still running and still costing money. Nothing
        below accounts for {unreadable.length === 1 ? "it" : "them"}.
      </p>
      <ul class="mt-2 list-disc space-y-1 pl-5 font-mono text-xs">
        {#each unreadable as record}
          <li>{record}</li>
        {/each}
      </ul>
    </div>
  {/if}

  {#if estateKnownEmpty}
    <p
      class="rounded border border-slate-200 bg-slate-50 px-4 py-6 text-sm text-slate-600"
      data-testid="estate-empty"
    >
      No live deployments. <code>infrafactory deploy &lt;scenario&gt;</code> creates one.
    </p>
  {:else if deployments.length > 0}
    <div class="overflow-x-auto rounded border border-slate-200">
      <table class="min-w-full text-left text-sm">
        <thead class="bg-slate-50 text-xs uppercase tracking-wider text-slate-600">
          <tr>
            <th class="px-3 py-2">Deployment</th>
            <th class="px-3 py-2">Health</th>
            <th class="px-3 py-2">Version</th>
            <th class="px-3 py-2">Last observed</th>
            <th class="px-3 py-2">TTL</th>
            <th class="px-3 py-2">Address</th>
            {#if teardownAllowed}<th class="px-3 py-2">Actions</th>{/if}
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-200">
          {#each deployments as d (d.id)}
            <tr
              class={needsAttention(d) ? "bg-rose-50/60" : ""}
              data-testid={`deployment-row-${d.id}`}
              data-attention={needsAttention(d) ? "true" : "false"}
            >
              <td class="px-3 py-2">
                <div class="font-semibold text-slate-900">{d.scenario || d.id}</div>
                <div class="font-mono text-xs text-slate-500">{d.id}</div>
                {#if d.upgraded}
                  <span
                    class="mt-1 inline-block rounded bg-sky-100 px-2 py-0.5 text-xs text-sky-900"
                    data-testid={`deployment-upgraded-${d.id}`}>upgraded</span
                  >
                {/if}
              </td>
              <td class="px-3 py-2">
                <span
                  class={`inline-block rounded-full px-2 py-1 text-xs font-semibold ${healthBadge(d.health).tone}`}
                  data-testid={`deployment-health-${d.id}`}>{healthBadge(d.health).label}</span
                >
                {#if d.health?.detail}
                  <p class="mt-1 text-xs text-slate-600">{d.health.detail}</p>
                {/if}
              </td>
              <td class="px-3 py-2">
                <span
                  class={`inline-block rounded-full px-2 py-1 text-xs font-semibold ${versionBadge(d.health).tone}`}
                  data-testid={`deployment-version-${d.id}`}>{versionBadge(d.health).label}</span
                >
              </td>
              <td class="px-3 py-2 text-slate-700" data-testid={`deployment-observed-${d.id}`}>
                {observedLabel(d.health)}
              </td>
              <td class="px-3 py-2 text-slate-700" data-testid={`deployment-ttl-${d.id}`}>
                {ttlLabel(d.time_to_live_seconds, d.expired)}
              </td>
              <td class="px-3 py-2">
                {#if d.address}
                  <a
                    class="font-mono text-xs text-sky-800 underline"
                    href={addressHref(d)}
                    target="_blank"
                    rel="noreferrer noopener"
                    data-testid={`deployment-address-${d.id}`}>{addressLabel(d)}</a
                  >
                {:else}
                  <span class="text-xs text-slate-500">no address recorded</span>
                {/if}
              </td>
              {#if teardownAllowed}
                <td class="px-3 py-2">
                  {#if destroying === d.id}
                    <span class="text-xs text-slate-600" data-testid={`deployment-destroying-${d.id}`}>
                      Destroying…
                    </span>
                  {:else if confirming === d.id}
                    <!-- The confirmation NAMES what is about to go. "Are
                         you sure?" is a speed bump people learn to click
                         through; stating which project and address makes
                         a misclick on the wrong row visible while it is
                         still reversible. -->
                    <div class="space-y-2" data-testid={`deployment-confirm-${d.id}`}>
                      <p class="text-xs text-rose-900">{teardownPrompt(d)}</p>
                      <div class="flex gap-2">
                        <button
                          class="rounded bg-rose-700 px-2 py-1 text-xs font-semibold text-white"
                          data-testid={`deployment-destroy-${d.id}`}
                          on:click={() => destroy(d)}>Destroy</button
                        >
                        <button
                          class="rounded border border-slate-300 px-2 py-1 text-xs text-slate-700"
                          data-testid={`deployment-cancel-${d.id}`}
                          on:click={() => (confirming = "")}>Cancel</button
                        >
                      </div>
                    </div>
                  {:else}
                    <button
                      class="rounded border border-rose-300 px-2 py-1 text-xs font-semibold text-rose-800"
                      data-testid={`deployment-teardown-${d.id}`}
                      on:click={() => (confirming = d.id)}>Tear down</button
                    >
                  {/if}

                  {#if outcomes[d.id]}
                    <p
                      class={`mt-2 text-xs ${outcomes[d.id].ok ? "text-emerald-800" : "text-rose-800 font-semibold"}`}
                      data-testid={`deployment-outcome-${d.id}`}
                    >
                      {outcomes[d.id].message}
                    </p>
                  {/if}
                </td>
              {/if}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  {#if !teardownAllowed}
    <p class="text-xs text-slate-500" data-testid="estate-readonly-note">
      Read-only. Start the server with <code>infrafactory ui --allow-teardown</code> to destroy
      deployments from here, or use <code>infrafactory live teardown &lt;id&gt;</code>.
    </p>
  {:else}
    <p class="text-xs text-slate-500">
      Teardown deletes real infrastructure and cannot be undone. Expired deployments can be
      cleared in one go with <code>infrafactory live reap</code>.
    </p>
  {/if}
</section>
