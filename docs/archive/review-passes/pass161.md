# Review pass 161 — S163e, twenty-sixth round

**10 findings, 8 accepted, 2 declined.** One of them is a defect in round
twenty-five's own fix, and the round found it the way round twenty-five
could not: by reading what the *real* `connectWS` does rather than what
the test's fake does.

## The one that matters

**`releaseSocket` did not bump `generation`.** Round twenty-five reset the
connection flag on disposal so the reconnect window would stop rendering as
a lost connection. But `connectWS`'s dispose only calls `socket.close()`;
the browser fires `onclose` **on a later task**, and `onclose` calls
`onStatus(false)` unconditionally. With no generation bump that late
callback was still current, so it wrote `false` back over the reset one
task afterwards — reinstating the exact false "Not receiving progress"
alarm the reset was written to remove.

The unit test could not see it, and said so in a comment: *"A dispose that
never calls back, exactly like `connectWS`."* That is wrong twice — the
real dispose calls back, and it calls back **late**. A synchronous fake
would still have passed, because `releaseSocket`'s own reset runs after the
dispose returns. The fake now defers via `queueMicrotask` and the test
awaits, and removing the bump fails it.

## Accepted

| # | Finding | Fix |
|---|---------|-----|
| 1 | the missing generation bump, above | bump on disposal; the fake models a late `onclose` |
| 9 | a scenario named `__connected` overwrote the connection flag, and `releaseSocket`'s "anything running?" scan skipped that key — so the socket closed under a running deploy and its log froze | the flag moved to a `connected` store of its own. Scenario names come from YAML, so this was operator-reachable; a separate store removes the collision rather than reserving a name against it |
| 2, 3 | `deployPreviewHandler`'s two 500s and the estate listing's 500 wrote `err.Error()` — the same `*fs.PathError` leak the deploy handler was hardened for last round, in the handlers either side of it | stable messages + `logDetail` |
| 4 | the catch-all `writeActionResult` discarded a non-nil result, including the new `deployment` id, so a registered, reapable deploy was reported to the client as an unknown that "may have created resources nothing is tracking" | the result wins when it carries anything; the bare-error path withholds its internals |
| 6 | a server with no lister answered `already_live_unknown: true`, which the client renders as "the estate could not be fully read" — blaming a read failure on a server that never had a live store | absent `already_live` instead, which the client already tells apart (`ESTATE_NOT_REPORTED`) |
| 5 | `load()`'s `finally { loaded = true }` was ungated while its try and catch were guarded, so a superseded read announced the page had loaded on behalf of a result it threw away | same `token === reads` guard |
| 8 | `deployWarnings` hoisted the already-deploying note above the unmodelled-cost warning that invalidates every figure above it | a third `kind`. Two ranks could not express three, and the ordering the tests asserted held only when `already_live` was non-empty |
| 10 | `isConnected` was dead in application code | resolved by the move to a `connected` store; both predicates are gone |

## Declined

**7 — `reports` is not persisted, so a reload destroys an un-dismissed leak
report.** Real, and out of this PR's scope rather than wrong. The finding
is a *durability* requirement for the leak report, and meeting it properly
means deciding where the durable copy lives — `localStorage` survives a
reload but not a different browser or machine, and the case that matters
(a deploy that failed before registration, so no live record exists) is
exactly the one a server-side record would cover better. That is a slice
with an ADR, not a line in a review round. Recorded as a follow-up rather
than half-solved here.

**Not raised but worth stating:** this round found nothing wrong with the
round-25 deletions themselves — the `finish()` message, the removed
pointer, the `already_deploying` disable. The findings are in code those
changes *touched*, or in code that predates them.
