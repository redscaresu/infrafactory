# Review pass 151 — S163e, seventeenth `/code-review` round

**10 findings, 8 accepted, 2 declined.** The largest one reverses a decision from
round eight, because the reason for it no longer holds.

## A permanent alarm on the common path

`mayHaveCreated` was set for every unclean deploy, on the argument that "a deploy
that fails before registration has no live record either, so this is the only place
it is ever said". **That is false for the usual case.** `deploy` registers from
whatever the state shows, whether or not the apply succeeded — the CLI even prints
`tear it down with infrafactory live teardown dep-x` — so a half-failed apply
usually leaves a record with a TTL, listed on the estate page and reapable.

Every one of those raised a red banner that only a human could dismiss, for
infrastructure something else already tracks, and could not even name what to tear
down: neither `ActionResult` nor `OutputResult` carried the id. Both do now, set
only when registration succeeded — the same condition the CLI's recovery line keys
off. A recorded failure points at its record; an unrecorded one keeps the alarm.

## The arrival hook is gone

Round eight added it because leaving only retired what had ALREADY finished, so a
deploy finishing afterwards was never dropped and greeted every later visit. Leaving
is unconditional now, which fixes that on its own — and the arrival hook had become
the thing doing the damage:

- it raced `loadDetail`, deleting a refusal that landed during the fetch before it
  had ever rendered (round sixteen fixed this with a snapshot);
- the snapshot then could not tell "finished long ago" from "finished five seconds
  ago while I stepped away", so a reader who navigated back precisely to see how a
  deploy went found the banner **and the whole apply log** already deleted.

Both were guards on state. The state is deleted: a banner is shown at most once, on
the visit the reader came back for, and leaving retires it.

## The rest

- `dismissReport` identified a report by array POSITION, and the button captured
  that position at render time — so two clicks landing before a re-render deleted
  two different reports, the second a leak nobody had read. Ids are minted in
  `recordOutcome`.
- `beginDeploy` overwrote a still-running entry. The only guard was the button's
  `disabled`, on the page the store exists to outlive: a second start reset a live
  apply's log, the 423 that followed marked it finished, and the socket handler
  began discarding the original's lines while it kept creating infrastructure. The
  rule is in the store now.
- `DeployError`, `startedNothing`, `isActionResult` and `readJSON` lived in
  `api.ts`, which `node --test` cannot import — so the four functions that decide
  whether a reader is told "nothing was created" or "resources may still be running"
  had no unit tests at all, and inverting any of them passed the whole suite.
  Extracted to `deploy-response.js`, the pattern `pitfalls-api.js` already
  establishes, with tests.
- `deploying` and `deployingKnown` were two variables encoding one tri-state,
  threaded as separate positional arguments — the exact shape whose failure was
  round sixteen's headline. It is `string[] | null` now, which makes the omission
  unrepresentable rather than something a caller must remember to forward, and takes
  both view functions back under the four-parameter rule.

## Declined

- **`pendingReports` recomputes per progress line.** Declined in rounds ten, twelve,
  thirteen and fourteen: a walk over a store holding a handful of entries.
- **`liveDeploymentsOf` walks the whole estate per preview.** Declined in round ten
  and recorded in ADR-0027. It now runs on two endpoints rather than one, which
  raises the cost without changing the argument: a `ListByScenario` on the store is
  the fix, and it is a store change, not a review fix.
