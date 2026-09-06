# Review pass 143 — S163e, ninth `/code-review` round

**10 findings, 9 accepted, 1 declined in part.** The first is the twin of round
eight's, on the other half of the same rule — and this is the last round on
substance. From here nits do not block: a clean pass means no finding that changes
behaviour, correctness, or what a reader is told.

## The same defect, at the other end

Round eight stopped ARRIVING at a scenario from deleting a failure report. Leaving
still did, on the reasoning that "the reader was on the page while it was
displayed, so it has been read".

That is wrong, and the message itself proves it:

> The deploy did not finish cleanly — it may have created resources that are still
> running. project 7c98d82e is live and could not be deleted.
> **check the Deployments page before starting another**

The Deployments link sits directly beneath the button. **Following the
instruction was what deleted the project id the instruction was about** — and a
deploy that fails before `registerDeployment` has no live record either, so the
estate page shows nothing in its place.

One predicate now, `forgetIfSucceeded`, applied at both ends. A stale success is a
false claim about infrastructure whose TTL may have expired. A failure is an unread
report and survives until the tab does. Two e2e tests cover the leave-then-return
path in both directions, and both fail when the distinction is removed.

## A claim the reader can see is untrue

The in-flight banner said "Applying now, so it has no record yet and does not
appear below: web-app-paris." Redeploying is deliberately allowed, so the table
below can hold an EARLIER deployment of that scenario — and the banner then
asserted an absence the reader could see was false, on the one page whose whole
thesis is never saying something false about the estate. It now says the applying
deploy has no record *of its own*, and that a row with the same name is an earlier
one.

## Two sources for one sentence

`deployOutcome` set `proven: "Deployed."` and the template ignored it, hardcoding
its own success text — so an edit to the builder changed nothing on screen while
every unit test asserting on it kept passing. The builder owns the sentence; the
template renders `message` for both branches.

## Docs that had drifted from the code

- ADR-0027 and STATUS said `startedNothing` is "exactly … 400, 404, 405, and 423".
  The set also contains **403**, from the origin guard, which is not
  `deployHandler`. Reconciling the two the other way — deleting 403 — would turn an
  origin-guard refusal back into "the deploy may still be running", which is the
  false alarm this round exists to remove.
- `knownEmpty`'s docstring was stranded above `nothingRecorded` by the extraction
  in the previous round: the same insertion defect I had just moved that block to
  fix for `estateSummary`, committed in the same edit. It also still said "Three
  things must all hold" after `deploying` made it four.

## Dead and unguarded

- `acceptProgressEvent` had no production caller left — the store switched to
  `isProgressEvent` — while keeping a docstring describing scoping the store no
  longer does, and five tests reporting coverage for a path the app never runs.
  Deleted, tests retargeted at `isProgressEvent`.
- The plural `already_live` warning dropped the "second bill" language, so the
  cost consequence disappeared exactly where it was largest. The test asserted only
  the count and the ids, so it was invisible.
- `isActionResult` reads `clean`, and `ActionResult.Clean` gaining `omitempty` — the
  natural instinct for a bool — would make precisely the *unprovable* case
  serialise without the key, so the client would classify the richest, most urgent
  response as a bare error and discard the project id. Go tests unmarshal into the
  struct and would not notice. Pinned on the JSON.
- `loadDetail` had no catch, unlike both its siblings, and is called unawaited from
  `afterNavigate` after `detail = null`. A 500 left a blank screen with no message
  and an unhandled rejection.

## Declined in part

**`actionOutcome`'s absent-result branch is unreachable from both callers.** True:
`api.deployScenario` and `api.tearDownDeployment` return only when `isActionResult`
holds. The branch stays — it is the one place a green tick could appear over
nothing at all, and the guarantee preventing that lives in another module — but the
finding's real half is accepted: the tests implied screen coverage they do not
have, and now say what they actually assert.
