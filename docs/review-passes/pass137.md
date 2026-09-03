# Review pass 137 — S163d, third `/code-review` round

Fourteen findings. **One of them was introduced by the previous round's fix**, and
it is the worst thing in this slice so far.

## A refused deploy announced that the running one had finished

`broadcastDeployComplete` was called before the `ErrDeployInProgress` branch. So a
POST that the lock **rejected** broadcast `deploy_complete` for the scenario — and
the broadcast is subject-scoped, so it cannot distinguish "the one I just refused"
from "the one still applying". They share a scenario.

Every watcher's log stopped, every button re-enabled, and a multi-minute apply
creating real infrastructure was reported as finished. The terminal event added to
*fix* a stuck-running bug could instead declare a running deploy over.

It is now sent only by a call that actually ran. `TestARefusedDeployDoesNotAnnounceACompletion`
covers both refusal paths.

## The filter that named an event instead of a family

`/live` filtered `deploy_progress` by exact match. Adding `deploy_complete` walked
straight back into the defect the filter exists for — raw JSON in an unrelated
run's log, plus a run-state fetch per event. It matches `deploy_*` now.

That is the second time in this slice that adding a second member of a set broke a
guard written for the first.

## Ordering: the terminal event outran the last line

`defer progress.Close()` runs when the handler returns — *after* the broadcast. A
client treating `deploy_complete` as terminal has already stopped listening when
the trailing line of a failed apply arrives, which is the entire reason `Close`
exists. Closed explicitly, before the broadcast.

## The warning that suppressed itself

`already_live_unknown` is estate-**global**: one corrupt record anywhere sets it.
`alreadyLiveWarning` returned early on it and **discarded a non-empty list**, so
"dep-existing is already deployed — deploying again creates a SECOND project"
became "whether this scenario is already deployed is unknown", for every scenario,
until somebody found the bad file. The strongest warning replaced by the weakest.
Both are said now.

Related: a nil lister returned `(empty, false)` — the claim "checked, and nothing
exists" that the flag's own docstring forbids, three lines above the `err != nil`
branch that gets it right.

## Written and never read, again

`finishedElsewhere` was set and read nowhere — the same defect the previous round
recorded for `adopted`, in the fix for it. Worse: the entry it produced had
`outcome: null`, so the panel's guard went false and **the whole deploy panel
vanished**, leaving a page identical to one where nothing ever ran.

## And the rest

- Deleting the poll left the broadcast as the *only* recovery, with none when it is
  missed — a dropped message, a reconnect gap, or an event arriving before this tab
  adopted anything. Recovery now runs on reconnect, which costs one request per
  connection rather than one every few seconds.
- STATUS contradicted itself: "it polls now" in one paragraph, "removes the poll"
  twenty-four lines later. `pollInFlight` was added and deleted on the same branch.
  **This is verbatim the pattern the same entry diagnoses.**
- A 13-line tombstone comment argued against a function that never existed on main.
- A JSDoc block was inserted between a docstring and its function, so both attached
  to the new one and `acceptProgressEvent` was left undocumented.
- The success message existed twice, once dead.
- The unreadable branch — the reason the flag exists — had no server-side test.

## The honest read on this slice

Three review rounds on one feature: 9 findings, then 13, then 14. Each round's
fixes generated the next round's findings, and in two rounds the *fix itself*
introduced a defect of the class it was fixing.

That is not a review problem. It is a signal that the deploy-progress UI carries
more state, and more coupling between server events and client state machines,
than its value justifies — and that I have been treating each finding as a local
repair rather than asking whether the design is worth defending.
