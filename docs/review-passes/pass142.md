# Review pass 142 — S163e, eighth `/code-review` round

**11 findings, all accepted.** The first one is the round-seven fix eating the
message it most needed to keep.

## The cleanup that swallowed the leak report

Round seven made arriving at a scenario drop a finished deploy, so a success
banner could not greet a reader weeks later about infrastructure whose TTL had
gone. It dropped **every** ending, including this one:

> The deploy did not finish cleanly — it may have created resources that are still
> running. project 7c98d82e is live and could not be deleted

A deploy that fails before `registerDeployment` has **no live record either**, so
that banner is the only place it is ever said — and it was deleted before it
rendered, because the reader happened to be on another page when the apply
finished. The failure this whole arc exists to prevent, arriving through a
cleanup.

Only a success is dropped now. A stale success is a false claim; a failure is an
unread report carrying the project id somebody has to remove by hand. The e2e for
it fails when the distinction is removed.

## A guarantee across two connections is not a guarantee

Round seven's other fix flushes the progress tail before the response so the
browser has not yet stopped listening. Correct as an ordering — and it was written
up, in a comment and in ADR-0027, as though it settled the matter. It does not:
the broadcast goes out on the websocket and the response on the HTTP connection,
and two connections have no ordering between them. An in-process test can only pin
the order the server *writes* in.

Kept as the better of two orderings, and the claim reduced to what it is. The
residual is narrow and now stated: `deploy` newline-terminates every line it
writes, so there is no tail to lose for today's producer. A producer that emitted
a partial final line would need it carried in the response body rather than raced
against it.

## Three copies of one rule, twice over

- `estateSummary` re-derived `total === 0 && unread === 0` in **both** its branches
  — inside the function that calls `knownEmpty`, whose own docstring says two
  copies of the condition are how one comes to contradict the other. Correct
  today, and it would have stayed silent the next time `knownEmpty` gained a term,
  exactly as it did when `deploying` was added. Extracted as `nothingRecorded`,
  which deliberately answers *only* "the read returned no records" so it cannot
  become a fourth copy of the whole question.
- `deployOutcome` was a structural copy of `teardownOutcome`. The words must
  differ — "Teardown returned nothing." beside a Deploy button is its own defect,
  which is why the sibling exists — but ADR-0024's rule (`clean`, not
  `failures.length`) must not. Applied to one and not the other, the deploy screen
  would report a success the teardown screen refuses. One `actionOutcome`, two
  vocabularies, and a test asserting both verbs judge every case identically.

## An inert guard

`acceptProgressEvent(msg, msg.data.subject)` compares the subject with itself. The
scoping that actually happens is the keyed lookup and the running check two lines
below; the call read as scoping while scoping nothing, and a later edit would have
trusted it. Split into `isProgressEvent` for the shape, leaving
`acceptProgressEvent` for callers that genuinely hold a separate "scenario on
screen".

## The rest

- `refuseDeploy` discards the log on the argument that "somebody else holds the
  lock" — true for 423, and it is called for 400/403/404/405 as well. The rule is
  really "nothing of ours started, so no line collected was ours"; for the other
  four the log is empty and the discard is a no-op. Stated once rather than
  conditioned on which refusal arrived.
- `forgetFinishedDeploy` read the reactive `deployEntry`, whose freshness inside
  `afterNavigate` depends on `detail` not yet having been nulled. Reads the store.
- The stale-estate banner read "the estate has not been readable since:
  web-app-paris", which parses as though the scenario were the point in time. The
  list has its own clause now.
- `?.startsWith("deploy_")` on `JSON.parse` output: the TypeScript cast is a
  compile-time claim about untrusted wire data, so a frame whose `type` is a number
  threw inside the socket handler and froze the run log for the session. Guarded
  with `typeof`, restoring the old harmless failure.
- `previewResponse` was dead, and its docstring named a category of test that does
  not exist. Folded back — and the fifth fixture copy round seven claimed to have
  removed was still there, in `TestPreviewReportsWhetherTheServerWouldAcceptTheDeploy`.
  One copy now.
- `slices` and `sort` were both imported for work `slices` does alone.
