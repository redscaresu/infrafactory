# Review pass 150 — S163e, sixteenth `/code-review` round

**10 findings, all accepted.** Three are round fifteen's own fixes applied to one
site and not its neighbour; one is a rule I wrote and then broke twenty lines away.

## The fix applied to one of its two callers

Round fifteen gave `knownEmpty` a `deployingKnown` term so a payload that never
said what was applying could not license "Nothing is deployed." `estateSummary`
calls `knownEmpty` too, and was not given the parameter — so with the field absent
the empty-state panel was correctly suppressed while the summary line **above it**
said "Nothing is deployed." Two derived emptiness claims contradicting each other
on one screen, which is the exact defect the extraction exists to prevent.

## A closing socket marking its replacement dead

Disposing a socket does not silence it. The close handshake is asynchronous, so an
old connection's `onclose` can fire after a replacement has opened — writing
`__connected: false` over a healthy socket. Nothing re-fires `onopen`, so every
deploy for the rest of the session rendered "Not receiving progress — the apply is
still running, but this page cannot see it." over a stream that was working.

Both callbacks carry a generation and check it.

## The rule, broken twenty lines from where it is written

`noSuchScenarioError` exists because wrapping a sentinel with `%w` puts it in the
operator's message. Its three siblings, added in the same commit, did exactly that:
`fmt.Errorf("%w: %w", api.ErrNothingStarted, err)` reads back as **"nothing was
started: config is unreadable"** — a sentence that contradicts itself and hands an
internal discriminator to a reader, since the handler puts `err.Error()` straight
into the body.

`nothingStarted(err)` wraps without prefixing. A test asserts both halves: the
promise stays machine-readable and neither sentinel's text reaches the message.

## A claim about the wrong verb, again

Round fifteen reverted `writeRefusal` on the origin guard because `started_nothing`
is a claim about whether an APPLY created infrastructure, and the guard refuses
every endpoint. The collection's 405 has the same problem — `/api/deployments`
delegates POST and every other verb lands there, including a DELETE meant for a
teardown — and it was converted in round fourteen for the opposite reason. Plain
error. There is consequently no deploy-specific 405 at all, which the test now says.

## The rest

- `readJSON` returned `null` for both "the parse failed" and "the value is null".
  Run artifacts are served verbatim with a JSON content type, and one of them can be
  exactly `null`, so a valid response became "could not be read". It returns
  `{ok, value}`.
- `DeploymentsResponse.deploying` was declared required while the page guards it
  with `Array.isArray`. The type makes the guard provably dead and invites its
  deletion — the same argument `already_live?` carries two files away.
- The `onMount` snapshot was justified by "afterNavigate does not fire on a direct
  load". It does: `loadDetail` is called only from that hook, and a direct load
  renders. The line was dead and the comment taught the next reader something untrue
  about the framework.
- Progress for a scenario this tab is not watching still called `deploys.update`,
  which notifies every subscriber whether or not the value changed — thousands of
  layout re-renders for a foreign apply whose lines the store discards. Checked
  before the update now.
- The mid-navigation race test synchronised on `waitForTimeout(200)`. On a loaded
  machine that lets the detail fetch land first, and the test then reports as a
  flake rather than as the regression it exists to catch. It waits on the Deploy
  button leaving its "Deploying …" label.
- Three names for one operation — `forgetFinishedDeploy` → `forgetReportlessDeploy`
  → `retireDeploy` — one of them asserting the opposite of what the delegate does,
  since `retireDeploy` KEEPS reports. One `retireOnLeave`.
