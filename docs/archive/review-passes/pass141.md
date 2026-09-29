# Review pass 141 — S163e, seventh `/code-review` round

**11 findings, 10 accepted, 1 declined.** One of them is a regression pass 140
introduced, which is the argument for the round.

## The fix that broke the flush

Pass 140 made the store ignore progress for entries that are not running — correct,
and it silently truncated the log it was protecting.

`progress.Close()` was deferred, so the buffered trailing line is broadcast *after*
`writeActionResult`. The browser's fetch resolves first, `finishDeploy` sets
`running: false`, and the new guard then drops the line. The last line of a
billable apply's log, discarded by the flush that exists to guarantee it is never
discarded.

Closed explicitly before the response now, with the `defer` kept for panics —
`Close` is idempotent. Pinned by a `ResponseWriter` that records what had been
broadcast by the time the headers were written, and the pin fails when the explicit
call is removed.

## The window `running` cannot close

The entry is created **before** the POST, so it is running — and collecting — for
the whole round trip. When the answer is a refusal, the *reason* is that somebody
else's apply of this scenario holds the lock, which makes every line collected in
that window theirs. The page rendered another tab's live log underneath an "already
deploying" banner.

`refuseDeploy` discards the log and keeps the entry. Keeping it is deliberate: the
refusal still has to be reported, and an outcome is keyed by scenario so it lands on
the right page and only that page.

## A banner that outlived every hook meant to drop it

`onDestroy` and `afterNavigate` drop a deploy that had *already finished* when the
reader left. Neither can drop one that was still running then and finished
afterwards — so that entry lived forever, greeting every later visit with
"deployed. It is listed on the Deployments page until its TTL expires." for
infrastructure whose TTL may long since have gone. The exact defect those two hooks
exist to prevent, reached by waiting a few seconds longer.

Arriving at a scenario now drops a finished deploy too, keyed on the navigation
counter so a save — which also reloads `detail` — cannot make a banner vanish
mid-visit.

## Demoted one layer down

Pass 140 fixed `alreadyLiveWarning` dropping the second-bill warning, by
concatenating all three warnings into one paragraph — which demoted it to the second
sentence of the first string, and `deployWarnings`' own docstring promises them
"separated so a page can render them differently". A test reading `.first()` cannot
tell the two arrangements apart.

`alreadyLiveWarnings` returns a list, strongest first, and the e2e now asserts the
second-bill warning leads and that the applying warning is a *separate* element.

## The rest

- `DeploymentDeployer.InFlight`'s docstring still named the deleted reload consumer
  and still said the server had no lock — false since S163c, and the authoritative
  copy an implementer reads. Every other instance of that claim was corrected two
  rounds ago.
- "The window closes rather than moving" was half true. The *finishing* window
  closes; a deploy that STARTS between the two reads is still in neither answer. No
  ordering fixes that, and its harm is bounded — the reader's own deploy is then
  refused — but the sentence claimed more than it had.
- The socket handler scanned every store key asking "is this yours?", needing a
  `__connected` skip. The event names its subject; a keyed lookup collapses three
  guards to one and makes the sentinel unreachable by construction.
- `res.json()` was unguarded, so a truncated body threw a `SyntaxError` past the
  `DeployError` classification — discarding a `startedNothing` the status had already
  settled, and putting a JavaScript parser message on the screen this slice exists
  to make trustworthy.
- The JSDoc block explaining `estateSummary`'s three-state contract sat above
  `knownEmpty`, because a second block had been inserted between it and its
  function. Both signatures were edited across two rounds without anyone noticing.
- The scenario fixture was copy-pasted into five preview tests. `previewOf(t,
  ServerConfig{...})` now fills in the config and the request; the callers supply
  only what they vary. Two copies remain, one of them the helper's.

## Declined

**`already_deploying: false` is a claim with no `unknown` counterpart.** True, and
the asymmetry with `already_live_unknown` is real — but a flag here would carry no
information. The only state in which the server has not looked is
`deployer == nil`, and that same condition sets `allowed: false`, so the deploy
cannot proceed at all. The residual case the finding is really about — an apply
started by the CLI or by another server process — is not *unknown to the check*, it
is invisible to the server, and no field the server sets can say otherwise.

What guards it is elsewhere and already built: the run-owned project per deploy
(ADR-0025), so a duplicate is contained rather than merged, and the estate page,
where both appear once recorded. The limit is stated in `InFlight`'s docstring, in
the scope note under the Deploy button, and in ADR-0027.
