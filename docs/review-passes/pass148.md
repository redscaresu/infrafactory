# Review pass 148 — S163e, fourteenth `/code-review` round

**12 findings, 8 accepted, 4 declined.** The accepts are all one shape: the
promise "nothing was created" was being made in one place and not its neighbours.

## One pre-apply path got the promise; its siblings did not

Round thirteen added `ErrNoSuchScenario` so a mistyped name could say nothing had
been created. `LiveDeployer.Deploy` has **four other pre-apply exits** and none of
them said it:

- `newRuntime()` failing → 500 → the client reads "we do not know" → a permanent
  red "it may have created resources that are still running" for a server that
  could not rebuild its own config
- `cmd.Flags().Set("ttl", …)` failing → the same
- `WalkDir` over a misconfigured `scenarioRoot` → an `*fs.PathError` that satisfies
  `errors.Is(err, os.ErrNotExist)`, so it took the deliberately-NOT-a-refusal
  branch → the same
- a blank scenario name → the same fact as the not-found branch twelve lines below
  it, and a bare `os.ErrNotExist`

`ErrNothingStarted` is the general sentinel; `ErrNoSuchScenario` is a refinement of
it that answers 404 where its parent answers 500. Both promise the same thing about
the cloud.

## And the 405 a client can actually receive

`deployHandler`'s method check was converted to a refusal last round. It is
unreachable — its only caller invokes it under `if r.Method == http.MethodPost` —
so the conversion made a promise about a response the server does not send, while
the 405 that DOES fire, in `deploymentsHandler`, was left as a plain error. The dead
branch is gone and the live one is a refusal.

## A report that pointed at the log pointed at nothing

`retireDeploy` keeps reports and clears `progress`; `beginDeploy` does the same on
a retry. For the case where the request never returns an ActionResult — a dropped
connection mid-apply — the message is the generic "may still be running", and the
head of the log is the ONLY place the run's project and workdir appear.
`KEPT_OPENING_LINES` exists to preserve exactly those lines, and two navigation
hooks deleted them.

A report carries its own copy now, and renders it.

## A log that ends in nothing

For a report outcome the page suppressed its outcome slot — correctly, since the
layout renders the report — so a reader watching the log saw it simply stop, with
the explanation above the H1, the run-mode card and the buttons. And dismissing the
report left an entry whose outcome the template refuses to render: a finished
deploy with its ending stated nowhere at all.

There is a terminal line pointing at the report, and dismissing the last report
removes the entry.

## The rest

- `previewFor` left `AlreadyLive` nil, so a preview built without
  `liveDeploymentsOf` emitted `already_live: null` — which the client correctly
  reads as "we could not look". The file argues at length that an empty list must be
  a checked claim, with `out := []string{}` on every path; the struct's zero value
  was the one place it was not. An existing test was already marshalling one.
- `forgetDeploy` had no production caller left and its docstring described the job
  `retireDeploy` took over — including the unconditional delete that threw reports
  away, which is what `retireDeploy` exists to stop. Two near-identical exports with
  contradictory docs is how the next reader reintroduces the bug.
- The 404 body said the same fact three times (`no scenario named "typo": no such
  scenario: nothing was started: file does not exist`) because the sentinels were
  wrapped with `%w` into the operator-facing message. A sentinel is for
  `errors.Is`; a message is for a person.

## Declined

- **`teardownOutcome` computes `mayHaveCreated` and the estate page ignores it**, so
  a teardown that could not prove the account clean still loses its message on
  navigation. A real gap of exactly this class — and a different verb, a different
  store shape, and a different page. It wants its own slice rather than being bolted
  on at round fourteen; recorded in STATUS as a named follow-up.
- **`pendingReports` recomputes per progress line.** Declined in rounds ten, twelve
  and thirteen for the same reason: a walk over a store holding a handful of
  entries.
- **`isProgressEvent` lives in the estate presentation module.** File placement.
- **`estateSummary` holds a numeric and a string form of "is anything applying".**
  Declined in rounds eleven and twelve.
