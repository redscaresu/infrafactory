# Review pass 157 — S163e, twenty-third round

**10 findings, 8 accepted, 2 declined.** I called the deletion converged after the
last round. **That was premature**: this round found a real behaviour defect in the
new design, and a second one that my own round-twenty-two "fix" introduced.

## A single slot, raced by one component

`ending` was one variable. The scenario page is reused across `[...path]` routes,
so one component instance can have several deploys in flight — deploy A, navigate,
deploy B — and whichever finished **last** overwrote the other's terminal line and
its whole log. The progress panel and the outcome vanished from under a reader for
a real, billable apply that had just completed, and `endDeploy` had already deleted
the store entry, so nothing could restore it.

Keyed by scenario now. That is not the state machine the deletion removed: it is
still component-local, still cleared wholesale on a route change, still has no
staleness rules. It just stopped assuming one page means one deploy.

## Manufacturing the proof ADR-0024 demands

Round twenty-two made `tearDownDeployment` return a synthetic
`{clean: true, steps: [], failures: []}` for a 2xx whose body could not be read —
reasoning, correctly, that `writeActionResult` answers 2xx only for a clean result.
`teardownOutcome` then rendered **"Destroyed. The account is provably clean."**
about a response this code never parsed.

The symmetry with `deployScenario` was the mistake. "Clean" means different things
for the two verbs: for a deploy it means nothing was left behind, and the client
uses it only to decide **not** to raise an alarm; for a teardown it is ADR-0024's
central claim, that the account is provably empty. Synthesising it from a status
manufactures exactly the proof the rule exists to demand. It reports honestly now —
not a failure, not a success.

## The rest

- `deployHandler`'s docstring said the method guard answers "a plain error, not a
  refusal" directly above a guard that calls `writeRefusal`, with an inline comment
  saying the opposite. Two contradictory statements of intent about one line.
- `already_live_unknown` is estate-**global**: one undecodable record anywhere sets
  it for every scenario, and it was being pushed as warning #0 — an unactionable red
  line at the top of every Deploy confirmation until somebody found the bad file,
  ahead of `modelled === false`, whose own docstring says it invalidates the figures
  above it. It is said last now, and pinned.
- The estate banner rendered `deployingLabel` verbatim, so the summary line two
  elements above said the same sentence. Sharing the builder removed divergence and
  guaranteed duplication instead; the count belongs to the summary, the banner names
  what is applying.
- `load()` had no request token while being driven by two sources — a 30s interval
  and a teardown's `await load()` — so a slow earlier response could resurrect a
  destroyed row and an apply that had already finished. Tolerable when the payload
  was a table; `deploying` now drives an alarm and gates the page's only emptiness
  claim.
- The refusal branch discards the log for every refusal kind, and the comment
  justified it only for the lock case. **The code is right and the comment was
  narrow**: a refusal means nothing of ours was applying, so any line that arrived
  belongs to somebody else's apply whatever the refusal was. Comment fixed, code
  unchanged.

## Declined

- **Four documented-unreachable branches violate YAGNI.** Each is a type-level
  guard on a function whose wrong answer is a false claim about billable
  infrastructure — the `name == ""` guard, `previewFor`'s cautious defaults, the
  absent-result branch, `attributed`'s colon arm. And `previewFor`'s defaults are not
  unobservable: a test marshals that struct directly, which is how the `null` they
  prevent was found. The rule they serve — a zero value must not make a claim — has
  cost two defects this arc when it was not followed.
- **`slices.Contains(deployingScenarios(state), …)` sorts a map to answer a
  membership question.** True, and it is a handful of strings; the alternative widens
  the deployer interface with an `IsDeploying` method. Declined in rounds twelve and
  seventeen for the same reason, now recorded for the preview path too.
