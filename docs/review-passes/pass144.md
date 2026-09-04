# Review pass 144 — S163e, tenth `/code-review` round

**13 findings, 9 accepted, 4 declined.** The declines matter as much as the
accepts: convergence is one pass with no finding that changes behaviour,
correctness, or what a reader is told, and by this round most of the yield was
style. Several of the accepts are defects the previous round's fixes introduced.

## A banner that could never be cleared

Round nine made a failed deploy survive until the tab does, because it may
describe infrastructure nobody has a record of. A REFUSAL is a failure — and
`startedNothing` is the server's own word that it created nothing. So a transient
423 stuck: "web-app-paris is already deploying" reappeared on every later visit for
the rest of the session, under an enabled Deploy button, long after the apply it
referred to had finished.

The rule was asking the wrong question. It is not "did it succeed?" but **"could
this still describe infrastructure nobody has a record of?"** — now
`outcome.mayHaveCreated`, answered where the outcome is built rather than inferred
from `ok` at the call site. A success is recorded on the estate page; a refusal
started nothing; an unproven deploy or a request that failed after the apply began
is the only kind that must not be forgotten.

## The fix for a blank page hid the leak report

Round nine's `{:else if detailError}` branch replaces the whole `{#if detail}`
subtree — which contains the outcome banner. A deploy fails with "project 7c98d82e
is live and could not be deleted"; the reader follows the message's advice, comes
back, and the scenario GET happens to 500. The store still holds the report and
nothing renders it.

The error branch now renders every kept report, named individually, because without
`detail` the page cannot say which scenario it is looking at.

## Three more that state something false

- "Deploying again creates a THIRD project" is only true for exactly two existing
  deployments. Three renders "creates a THIRD" for what would be the fourth.
- `Array.isArray(preview?.already_live) ? … : []` turned an ABSENT field into the
  positive claim "nothing of this scenario is live" — the exact false negative the
  server's `(out, true)` on every unchecked path exists to forbid, thrown away at
  the client boundary. An older server or a trimmed body rendered no warning at
  all, indistinguishable from a checked answer.
- The outcome prefixed the scenario onto a refusal message that already began with
  it: "web-app-paris: web-app-paris is already deploying". Two layers solving the
  same attribution problem; the builder does it now, and skips the one message that
  names itself.

## The rest

- The progress log grew without bound, copying the whole array per line. A real
  apply emits thousands of lines over minutes, so the tab stalls during the apply
  the log exists to make watchable. Capped at 999, like the Live Run page one file
  away.
- `loadDetail` returned on an empty path before clearing `detailError`, so a
  previous scenario's failure rendered for a page that was never asked about.
- The dropped-connection message ran two sentences together: server errors and
  `deploy failed: 502` never carry terminal punctuation.
- The in-flight staleness flag was derived as `Boolean(loadError)` in the page and
  as a hardcoded `true` inside `estateSummary`. They agree by coincidence; both
  read `estateState` now, which is the whole reason `deployingLabel` exists.

## Declined

- **The `startedNothing` allowlist mirrors one handler's semantics and has no body
  discriminator.** The concrete harms need either a future edit that adds a
  pre-apply status, or an intermediary that both forwards a request and answers
  403 for it. The direction is already recorded in `api.ts` and ADR-0027, and the
  allowlist errs safe by construction. Not built.
- **`deployWarnings`/`deployConfirmation` are called in the template rather than
  hoisted to `$:`.** Allocation on reactive updates. No behaviour change, and the
  nodes are static `<p>`/`<li>`.
- **The e2e preview stub is repeated nine times.** Test duplication. The *drift* it
  caused was real and is fixed — every stub now carries `already_live`, which the
  server always sends — but extracting a helper changes nothing about what the
  suite proves.
- **`__connected` shares the key namespace with scenario names.** One exclusion, in
  one function, with a test pinning it. A separate store would be tidier and would
  change no behaviour.
