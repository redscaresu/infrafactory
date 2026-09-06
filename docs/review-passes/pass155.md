# Review pass 155 — S163e, twenty-first round: the rethink

**12 findings — and 7 of them were regressions from round twenty's 8 fixes.** Two
were severe, and both were the fix performing the exact failure it was written to
prevent:

- the branch honouring `beginDeploy`'s refusal called `finishDeploy` on an entry
  that is *running by definition*, ending a live apply's log and re-enabling the
  button;
- the `"clean"` conclusion fired for **any** 2xx that was not an ActionResult —
  including a well-formed `{"status":"ok"}` from a proxy that never reached the
  deploy handler — rendering a green tick and no report.

Both verified against the code rather than taken on faith.

That is a fix-to-regression ratio of about 1:1, sustained for six rounds, and
concentrated entirely in one place: the scenario page's deploy lifecycle. The Go
handlers, the estate page and the preview had been quiet for several rounds.

So this round is not another set of fixes. **It is the deletion the evidence has
been pointing at since round eight.**

## What was actually wrong

The `deploys` store held a terminal `outcome` — how the last deploy ended — in a
store that outlives the page. Everything that kept breaking existed to keep that
honest:

`retireOnLeave` · `shownScenario` · the route-change guard · the arrival hook
(added, then deleted) · the stale-success rules · `mayHaveCreated` as a
forgetting predicate · the report pointer · `dismissReport`'s cross-store
deletion · `finishDeploy` / `refuseDeploy` / `retireDeploy` as three separate
endings.

Every one of those produced a defect in a later round. Every lasting fix in this
arc came from deleting state, not from guarding it — the adoption machinery, the
arrival hook, the parallel `deployingKnown` boolean, five error types collapsed to
one.

## Three lifetimes, three homes

- **What is running** → the `deploys` store. Entries exist only while a deploy is
  in flight, so they can be dropped the moment it ends.
- **What the reader just watched** → the page, transient. One `ending` value,
  rendered only when its scenario is the one on screen and cleared on a route
  change. Those two facts *are* the scoping rule: no token, no clearing pass, no
  staleness question.
- **What must not be lost** → `reports`, in the layout, durable and dismissible.
  A statement that infrastructure may exist with no record of it.

Three lifecycle functions became one — `endDeploy(scenario, outcome)` files a
report if there is one to file, drops the entry, and hands the log back to the
caller so a log does not vanish at the moment it ends.

**Net −82 lines across the store and the page**, and the store's exported surface
is down to eight.

## The trade, stated

A deploy that finishes while the reader is on another page is **not announced when
they come back**. Three rounds of defects came from trying to announce it. The
durable answers were always elsewhere: the Deployments page says what is deployed,
a report says what may have been left behind. This page says what is happening
while you are watching it.

## Round twenty-one's findings that survive the deletion

- `readJSON`'s `ok` flag was computed and discarded — and it is exactly the
  discriminator the 2xx case needed. A body that **failed to parse** means the
  server was cut off mid-write, so its 2xx is this server's word and the result was
  clean; a body that **parsed into something unrecognised** means something else
  answered, and its status proves nothing.
- `deployHandler`'s restored method check answered with a plain error while its
  sibling used `writeRefusal` — so a client read it as unknown and pinned a leak
  report for a request no handler ran.
- Its test asserted on `(&fakeDeployer{}).calls` — a freshly allocated value, always
  nil — so it passed for an implementation that ran the apply and *then* wrote 405.
- The CLI's `if recorded` guard had no test that ran it: the one that claimed to
  tested `encoding/json`'s `omitempty`. Extracted as `recordedDeploymentID` and
  pinned. **Owed since round nineteen, where I marked it accepted and did not
  implement it.**
- `status` was never cleared on navigation, so a failed post-save refresh wrote a
  read error into it that then rendered under the *next* scenario's title.
- `knownEmpty` and `estateSummary` kept a `state = "loaded"` default three lines
  below a comment arguing a default is "a third way to forget".

## Declined

- **The preview's estate walk.** Declined in rounds ten, seventeen and twenty and
  recorded in ADR-0027. A `ListByScenario` on the store is the fix and it is a store
  change, not a review fix.
- **`attributed`/`sentence` live in the `.svelte` file, unreachable by
  `node --test`.** Fair, and the smaller page makes it cheaper to do — but moving
  them is a change in its own right, and this round is already a deletion. Named
  here rather than bundled.
