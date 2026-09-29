# Review pass 136 — S163d, via `/code-review`

Thirteen findings on the **fix** PR. Twelve addressed.

## Three instances of the defect the PR existed to fix

The PR's headline was *"the claim I had just corrected elsewhere was still wrong
here"*. It shipped that same defect three more times:

- **`already_live` was computed and never read.** The ADR said *"the preview
  reports `already_live` and the confirmation says what exists"*; STATUS said it;
  the handler's doc comment said it; there was a server test. The UI never touched
  the field, and the type did not declare it. **The guard I documented did not
  exist**, and a full estate listing ran on every preview to populate a dead field.
- **The ADR still said the refusal answers 409**, nine lines above the amendment I
  edited — so a reader implementing a client from the decision record would write
  `res.status === 409` and recreate the exact bug.
- **The "server has no in-flight lock" claim was also in `+page.svelte`**, a file
  the diff edits. I removed it from `deploy-store.js` and left the copy next door.

Three documents and one comment asserting a guard that was not there. The pattern
is not carelessness about one line; it is that **I write the claim in every place
at once and then correct it in one place at a time.**

## The altitude finding, which was right

I treated *"there is no terminal websocket event"* as a fixed constraint and built
polling around it — a filesystem walk of the estate every five seconds, a race
between the listing and the owning tab, and an answer that could say "it stopped"
but never "it succeeded", because the estate does not carry that.

The hub was already there and already subject-scoped. `deploy_complete` is five
lines in the handler this same PR touches, and it removes the poll, the race, and
the missing-outcome gap together. **Polling is gone.**

## Correctness

- **`adoptInFlight` skipped any existing entry, not just a running one.** A tab
  whose own deploy was refused held a stale finished entry, so every later listing
  was ignored and the reader was invited to click again for the whole apply.
- **An adopted deploy that completed rendered nothing** — `outcome: null` and a
  `finishedElsewhere` flag read nowhere, so the panel vanished and the page looked
  like one where nothing had run. The same "written and never read" defect this PR
  claimed to fix for `adopted`.
- **`liveDeploymentsOf` failed open**: a `List()` error and undecodable records
  both produced `already_live: []`, indistinguishable from "checked and found
  none", on a guard about billable infrastructure. It reports `already_live_unknown`
  now.
- **No client test for 423.** A revert to 409 would restore the alarming message
  with every suite green.
- **A parameter shadowed the `scenario` package**, compiling only because the body
  happened not to use it.

## Recorded honestly rather than fixed

The ADR now says the lock is **an in-memory map in one process**: the CLI `deploy`
command never touches `LiveDeployer`, so `infrafactory deploy` alongside a UI
deploy of the same scenario still produces two projects. That is a bigger hole
than the sequential one the previous amendment recorded, and the PR that exists to
stop the ADR overstating the lock was still overstating it.

Also restored: the deleted warning that a scenario-keyed stream cannot tell you
*which* deployment id you are watching — narrowed by the lock, not retired, and
still the argument to `live teardown`.

## Left open

`ui/src/lib/api.ts` still decides how to parse a body from its status code, and
`tearDownDeployment` has the same shape. A discriminator in the body would close
the class rather than this instance. Worth doing; not in this pass.
