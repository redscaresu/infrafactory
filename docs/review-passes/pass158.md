# Review pass 158 — S163e, twenty-fourth round

**10 findings, 8 accepted, 2 declined.** One behaviour defect, and it settles a
question I had flip-flopped on twice.

## Never assert clean about a body you did not read

An unparseable 2xx was classified `"clean"` — no report filed, emerald banner. The
argument was that `writeActionResult` answers 2xx only for a provably clean result,
so a truncated body means this server was cut off mid-write.

**A proxy, captive portal or TLS interceptor can answer 2xx with an unparseable
body too, and nothing here tells them apart.** So "the parse failed, therefore it
was our server" does not hold. Worse, `tearDownDeployment` had already been fixed to
refuse exactly this synthesis, so the two verbs disagreed — and the classification
got *safer as the evidence got worse*: a body that parsed into something
unrecognised was `"unknown"`, while one that parsed into nothing at all was
`"clean"`.

Both are `"unknown"` now. Erring this way costs a wasted look at the Deployments
page; erring the other way is a green tick over an apply that may be running and
billing, with no report anywhere.

The `"clean"` state is **deleted** rather than left unreachable — nothing produces
it, so nothing should document it.

## The rest

- `releaseSocket` disposed the socket without clearing `__connected`. `connectWS`'s
  dispose only calls `close()`; `onStatus(false)` arrives later via `onclose`, and
  the next `ensureSocket` bumps the generation and silences it *deliberately*. So
  the flag kept a stale `true` across the gap and a deploy started before the
  replacement opened rendered "Starting…" instead of "Not receiving progress" — the
  conflation `streamConnected` exists to remove.
- `estateSummary`'s `failed` branch dropped the "this server did not report what is
  applying" caveat the `loaded` branch treats as mandatory — a third divergent copy
  of the emptiness question.
- `applyingLabel` was dead: nothing rendered it, and the banner 38 lines below said
  "Not `applyingLabel`". Deleted with its import.
- `deployWarnings` sorted the estate-wide caveat by matching its **opening words**.
  Rewording it, or adding a second estate-wide warning, would silently promote it
  above the unmodelled-cost line that invalidates the figures. Each warning carries
  a `kind` now; a tag cannot drift from prose the way a prefix match can.
- `fileReport` ran one line *before* `deploys.update`, which does not achieve what
  its docstring claims for calling it "beside" the updater: `reports` subscribers
  still fired while `deploys` held a running entry. Harmless only because the layout
  reads `reports` and not `deploys` — i.e. the stated invariant was not established.
  It runs after.
- The confirm button ignored `preview.already_deploying`. The warning promises the
  click "will be refused", and it would be — but `beginDeploy` succeeds because THIS
  tab has no entry, so for the whole round trip the panel labelled as this deploy
  streams the other apply's output, and the 423 then discards it. Minutes of output
  attributed to a deploy that never started.
- `deployConfirmation`/`deployWarnings` were invoked inline in `{#each}` heads, so
  they rebuilt on any dirty dependency of the block. Hoisted to track `preview`.
- `writeRefusal(w, 500, err.Error())` put an `*fs.PathError` — absolute filesystem
  paths and all — into a body the page renders verbatim. "Return meaningful errors
  without exposing internals": the cause goes to the log, the operator gets a stable
  message and the promise that nothing was created.

## Declined

- **The `os.ErrNotExist` branch is unreachable and the guard chain is
  order-dependent.** True on both counts, and the ordering is load-bearing:
  `NothingStarted("", walkErr)` wraps an `*fs.PathError` that satisfies
  `errors.Is(err, os.ErrNotExist)`, so the refusal check must precede it. That is
  what the comment says and what the tests exercise; converting an ordering
  invariant into a structural one here means a typed switch over an interface whose
  implementations are outside this package, which is a bigger change than the risk.
- **`writeRefusal` is a general helper making a positional promise.** The rule is
  now settled and written down — any path that answers before a deploy could begin
  may use it — and every call site is a pre-dispatch guard. A distinct name would
  say it more loudly; it would not make it structural either, since nothing stops a
  future handler calling a differently-named function too late.
