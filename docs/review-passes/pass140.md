# Review pass 140 — S163e, sixth `/code-review` round

**12 findings, 11 accepted, 1 declined.** Four of them are on the fixes from pass
139, which is the useful part: the round found the defects my corrections
introduced, and one of them was in the justification I wrote for a correction.

## A race the ordering created, not the code

The preview read the estate and *then* the in-flight list. A deploy that finished
between the two reads is in **neither** answer — absent from the estate because it
had not registered yet, absent from the in-flight list because it had already
released — so the confirmation rendered with no warning at all and an explicit
"checked, and nothing exists" claim, at the exact moment the scenario went live.

Reversed. Anything that leaves the in-flight list after the first read has
necessarily registered before the second, so the window closes rather than moving.
Pinned by a test that records which read happened first, and the pin fails when the
order is swapped back.

## I qualified the rows and not the claim beside them

Pass 139 made `estateSummary`'s failed branch carry `deploying`, on the argument
that it is "the one thing here still known". **That argument is wrong.** The
in-flight list is read in the same request as the rows, so it is exactly as stale
as they are — and the rows say "read before the error, and possibly out of date"
about themselves while the deploy count asserted the present tense, and kept
asserting it for as long as polling failed.

`deployingLabel(deploying, stale)` now carries the qualifier, and is shared with
the estate banner, which was rebuilding the same count and pluralisation two lines
below the summary — the two-copies-of-one-claim shape `knownEmpty` exists to
remove.

## Adoption through the last remaining door

`acceptProgressEvent` matches on scenario and nothing checked whether the entry was
still **running**. A finished deploy's entry stays until the reader navigates away,
so a second deploy of the same scenario from anywhere else — another tab, the CLI —
appended into it, and the page rendered a live, growing log of an apply it did not
start underneath a completed-outcome banner.

The store now ignores every entry that is not running.

## One request, two answers, two scopings

An outcome lived in the scenario-keyed store and survived navigation; a refusal was
a component variable. Pass 139 bolted a navigation token onto the refusal, and that
produced the finding: `forgetDeploy` ran unconditionally while the message was
gated, so a refusal arriving after any navigation deleted the entry and reported
**nothing at all** — the button silently reverting to "Deploy…" as though the click
had never landed.

`deployRefusal` is deleted. Every ending is an outcome, keyed by scenario, with no
token and no clearing rule. Forgetting on refusal existed to stop a refused entry
adopting another tab's stream, and the running-only filter above closes that for
every finished entry rather than for this one — so removing the state was available
precisely because the other fix landed first.

## The class, not the instance

- `deployScenario` still special-cased a bare **409** as an `ActionResult`. Any
  other producer of 409 — a proxy, an intermediary, the next refusal somebody adds
  — was parsed as a result, found no `clean` field, and rendered "resources may
  still be running" for a request that never reached the deployer. That is the same
  defect moving the refusal to 423 fixed for ONE producer. The **body** now decides:
  `isActionResult` checks for `clean`. Applied to teardown too, which had it.
- `startedNothingStatuses` was derived from `deployHandler` alone and missed
  **403**, which `guardCrossOriginRequests` answers before any handler runs. Added.
  The list stays an allowlist on purpose, and the file now says why: erring this way
  says "may still be running" about a request that never touched the cloud, and
  erring the other way says "nothing happened" while a project is being billed.
- `teardownOutcome` was reused verbatim for deploys, so a 409 rendered "Not
  provably clean — resources may still be running" and a malformed body rendered
  **"Teardown returned nothing."** next to a Deploy button. Only reachable on the
  failure branch, where a reader is least equipped to discount it. `deployOutcome`
  is its sibling.
- `estateSummary`'s loaded branch returned "1 deploy in progress" alone when the
  estate held zero records — never saying the estate had been read. The empty-state
  panel is suppressed in that case and the table does not render, so that line was
  the only thing that spoke.
- The `name == ""` guard's comment claimed a blank `scenario:` key reached it. It
  cannot: an empty query is 400 and a name mismatch is 404. The guard stays as a
  type-level refusal to make an unchecked claim; the false explanation is gone.

## Declined

**`slices.Contains(deployingScenarios(state), …)` allocates and sorts under the
deployer's mutex.** True, and left alone. The list is one process's in-flight
deploys — a handful at most — and the alternative is widening the deployer
interface with an `IsDeploying` method to save a sort of three strings, on a
handler that already accepts a filesystem walk per click. The seam costs more than
the sort.
