# Review pass 138 — S163e, fourth `/code-review` round

Fifteen findings, and their shape is the lesson: **the cut removed consumers and
left producers behind.**

## Dead on arrival, because I deleted the other half

- `broadcastDeployComplete` and `errorText` broadcast a `deploy_complete` event to
  every connected browser with **zero readers** — the commit that removed
  adoption removed all of them. Plus ~80 lines of tests pinning it.
- `acceptCompleteEvent` was exported, documented and unit-tested with no
  production caller, and its docstring described the adoption machinery the same
  PR deleted.
- The `deploying` type comment still said "so a reloaded page can restore what it
  was showing" — nothing restores anything now.

All removed. Deleting a feature means deleting what fed it, and I checked the
callers of what I deleted without checking the callees.

## Two real defects

**The estate page contradicted itself.** With a deploy applying and no records,
`knownEmpty` and `estateSummary` both ignored `deploying` — so *"Nothing is
deployed."* rendered directly under *"1 deploy in progress."* The page's own
comment says "two copies is how one of them ends up contradicting the other", and
`deploying` was a third copy neither knew about. That is the same class the earlier
`knownEmpty` work closed, reopened by adding a source of truth without telling the
predicate.

**A refused deploy adopted another tab's stream.** `beginDeploy` ran before the
POST, so a rejected attempt still created an entry — and the store matches progress
by scenario, so the other tab's still-streaming lines appended into it. A red
"already deploying" banner on top of a live, growing log of an apply this tab never
started: the adoption the cut removed, arriving through the back door.

My first fix moved `beginDeploy` after the POST and **broke live progress
entirely** — three DOM tests caught it. The entry has to exist during the apply;
what it must not do is survive a refusal.

## The gap the cut created

`already_live` reads the estate, and a deploy that is applying **has no record
yet** — so the confirmation showed no warning at all in the case a reader is most
likely duplicating. The preview consults the in-flight list too now.

## And the rest

- `liveDeploymentsOf`'s docstring said unreadable records "are ignored rather than
  reported here" while the function returns them and the confirmation renders them.
- A blank scenario name returned `(empty, false)` — the "checked, and nothing
  exists" claim the flag exists to forbid.
- `progress.Close()` was off `defer`, losing the panic guarantee, justified by an
  ordering argument for a broadcast that no longer exists. Deferred again.
- Unchecked type assertions on `map[string]any` panicked the test binary instead
  of failing the assertion that names the field.
- Three stacked comment blocks above `onMount`, one a verbatim duplicate.

## Left open, deliberately

`api.ts` still decides how to parse a body from its status code, and
`tearDownDeployment` carries the same shape — a discriminator in the body closes
the class. And the deploy preview walks the whole live store per click; STATUS
called that "a second instance" of a cost, which is now wrong, because the page
mount that was the first instance was deleted on this branch.
