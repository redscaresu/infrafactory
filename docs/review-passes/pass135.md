# Review pass 135 — S163c, via `/code-review`

Nine findings on the merged in-flight-lock slice, from the code-review harness.
Eight fixed; one recorded.

## The worst: two 409s with incompatible bodies

`writeActionResult` answers **409** for a deploy that RAN and could not prove
itself clean — carrying an `ActionResult`. The new "already deploying" refusal
also answered 409, carrying `{"error": ...}`.

The client special-cases 409 as an ActionResult. So a refusal was parsed as a
result, found no `clean` field, and rendered **"Not provably clean — resources may
still be running"** for a request that never touched the cloud. The most alarming
possible way to be wrong, on the screen where alarm is expensive.

The refusal is **423 Locked** now. Two cases, two statuses, and the client's 409
branch is unambiguous.

## The claim I had just corrected elsewhere, still wrong here

`deploy-store.js` still said *"The server has no in-flight lock, so this is the
only thing standing between a reader and that second deploy"* — false as of the
very PR it sits in. Pass 134 was about exactly this defect class, and I corrected
the comment in `live_service.go` and the ADR and left the one that names the
missing lock **by name**.

## The lock's scope was overstated in the ADR

It prevents **concurrent** duplicates. Deploy, wait, deploy again still produces a
second run-owned project — the lock releases when the first finishes and nothing
consults the estate. The ADR stated the harm without that qualification.

A lock is the wrong tool for the sequential case: it cannot tell "I forgot" from
"I meant it", and refusing outright would break redeploying after a teardown. The
preview reports `already_live` instead, and the confirmation says what exists. The
ADR now says what the lock does and does not close.

## An adopted deploy could never finish

`adoptInFlight` set `running: true`, and nothing cleared it: there is no terminal
websocket event (the only kind is a progress line), and only the tab that issued
the POST calls `finishDeploy`. So after a reload the button stayed disabled and
the panel never resolved **for the rest of the session**.

It polls while anything adopted is running, and an adopted entry the server stops
reporting is marked finished. An entry this tab *owns* is never cleared that way —
only the owner knows when its own POST returned.

## And the rest

- **The stale-banner fix did not cover the case its own doc names.**
  `afterNavigate` does not fire for a component being destroyed, so leaving the
  scenarios *section* still left the banner to reappear. Also hooked to
  `onDestroy`; the e2e only covered scenario→scenario, inside one component.
- **`adopted` was written and never read.** A reloaded page showed "Starting…"
  during minute four of an apply — claiming nothing had happened when minutes of
  output had happened and was lost. It says so now.
- **`adoptInFlight` had no test**, and its "leave owned entries alone" branch had
  zero coverage: deleting that line passed everything while destroying the log of
  a running billable apply. Four tests now, both mutations verified failing.

## Recorded, not fixed

Every scenario-page mount fetches the full estate listing — a filesystem walk plus
`Health()`, `Expired()` and a sort for every deployment — to read one string
array. Real waste on a page that mostly edits YAML. The fix is a lighter endpoint
or folding `deploying` into the preview response the page already requests, and it
is worth doing when the estate is large enough to notice.

## And one of mine

`pollInFlight` was used before it was imported, because a string replacement
targeted an import list in the wrong order and silently did not apply. The build
passed; the page threw at mount and **eleven of twelve e2e tests failed**. Caught
immediately by running them — but it is the fourth time this session that blind
string surgery on this file has broken it, and reading before editing has been the
fix every time.
