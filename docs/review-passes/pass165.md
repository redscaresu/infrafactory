# Review pass 165 — live run selects the newest run

`codex exec review --base main` on `fix/live-run-selects-newest`.

## Verdict

Clean on the first pass.

> No discrete correctness issues were found in the diff. The changes consistently
> make latest-run selection timestamp-based and avoid auto-pinning the live page
> URL while preserving explicit run_id pins.

Nothing declined — nothing was raised.

## What the pass was reviewing

`selectLatestRun` sorted newest-first and then discarded that order, scanning the
whole history for any run whose status was `running` and preferring it. That can
only disagree with `filtered[0]` when an **older** run claims to be running, which
is always a lie: a run id is a UTC timestamp, so a live run is already first.

There was a steady supply of liars. Nothing marks a run terminal when its process
dies, so **21 records going back to 29 May** still said `status: "running"` on
disk. The live page showed a run interrupted on 30 August — frozen elapsed timer,
stage list ten days stale — instead of the run started minutes earlier.

Two further sites in the same chain:

- `scenarios/[...path]/+page.svelte` carried a verbatim copy of the rule. It now
  calls `selectLatestRun`, so there is one implementation to be wrong.
- `live/+page.svelte` wrote the **auto-selected** run id back into the URL via
  `replaceState`. `onMount` reads `run_id` first and then never looks for a newer
  run, so an incidental landing became a permanent pin that survived every reload.
  It now writes the scenario only; an explicit `?run_id=` from the runs list is
  still honoured, because that pin is one somebody asked for.

## The part worth keeping

The old behaviour had a test, and it was green:

```js
test("selectLatestRun prefers running run within a scenario", () => {
  { run_id: "20260228T100000Z", status: "failed"  },
  { run_id: "20260228T110000Z", status: "running" },   // ALSO the newest
```

The running fixture was also the newest, so the assertion held whether the
implementation preferred running runs or simply took the newest. The test named a
behaviour its own sample could not distinguish from the alternative — so it
certified the bug for as long as it existed.

The new test inverts the ordering, which is the entire fix to the test:

```js
{ run_id: "20260830T201451Z", status: "running" },   // older, and lying
{ run_id: "20260909T093610Z", status: "failed"  },   // newer, and true
```

Mutation-checked rather than assumed: restoring the old one-liner fails exactly
one test (`pass 148 / fail 1`); with the fix, `pass 149 / fail 0`.

Same shape as [[feedback-narrow-sample-wrong-conclusion]] — a check whose sample
cannot separate the two answers reads as confirmation of whichever one is there.
