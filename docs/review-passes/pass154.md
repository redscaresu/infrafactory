# Review pass 154 — S163e, twentieth `/code-review` round

**10 findings, 8 accepted, 2 declined.** One decline is a case where the proposed
fix does not do what it claims, and the answer was to withdraw the promise instead.

## Three meanings, and the client picked the alarming one

`deployOutcome`'s unproven message asserted the deploy "left no record". An empty
`ActionResult.Deployment` means one of **three** things: nothing was created;
something was and could not be registered; or the result itself was unreadable, so
the id never arrived. Only the middle is a certain leak, and the third is produced
by `deployOutcome`'s own envelope-parse failure — which then rendered "left no
record" one sentence before a failure detail saying the outcome was unknown.

It says "no record of it reached this page" now, which is true of all three.

## The one response shape that guarantees nothing leaked

`writeActionResult` answers 2xx **only** when the result is provably clean. A 200
whose body a proxy truncates was thrown as an unknown failure — so the page filed a
permanent, hand-dismissible "may have created resources that are still running" for
the single response that proves the opposite, and called it "deploy failed: 200".

`DeployError` now carries a three-state `conclusion` — `refused` / `clean` /
`unknown` — rather than one boolean the caller had to combine with another. Two
booleans is what this kept turning into: "did the server refuse?" and "may this
have created something?" are different questions.

## One bit, five types

`ErrNothingStarted`, `ErrNoSuchScenario`, `nothingStartedSentinel`,
`noSuchScenarioError` and `nothingStartedError` all existed to say "the apply had
not begun" without concatenating sentinel text into an operator's message — with
three different unwrap behaviours, so `errors.As`, `errors.Unwrap` and `errors.Is`
answered differently depending on which one a caller held.

One shape: `api.NothingStarted(message, cause)` and `api.NoSuchScenario(message)`.
The message is the caller's, the sentinel is matched by `Is`, the cause is exposed
by `Unwrap`.

## An invariant left to a docstring

Round sixteen deleted `deployHandler`'s method check because converting it to a
refusal "made a promise about a response the server does not send". The objection
was to the wrapper, not to the guard — and `deployHandler` is a package-level
constructor, so registering it directly would let a GET run a real apply. Restored
as a plain error, with a test that calls the handler directly.

## The rest

- `recordOutcome` wrote the `reports` store from inside `deploys.update`'s updater —
  a side effect in a function contracted to produce a value. Filing happens beside
  the update now.
- `retireOnLeave` read `detail?.name`, which `afterNavigate` nulls and reloads
  asynchronously, so leaving a scenario mid-load retired nothing and the stale
  banner survived. It tracks the last scenario whose detail actually loaded — the
  route's path is not a substitute, because discovery matches on the declared name.
- `if (!beginDeploy(target)) return;` abandoned the click silently, after the dialog
  had already closed. It says so now.
- `teardownOutcome` returned a `mayHaveCreated` no caller reads. Stripped: giving
  teardown the report path is the right answer and is a slice of its own.

## Declined

- **`noSuchScenarioError` needs `Unwrap() error { return os.ErrNotExist }` so
  `os.IsNotExist` agrees with `errors.Is`.** It would not. `os.IsNotExist` unwraps
  `*fs.PathError`, `*os.LinkError` and `*os.SyscallError` and compares by `==`; it
  never consults `Is` **or** `Unwrap`. No custom type can satisfy it short of BEING
  an `*fs.PathError`, which puts filesystem framing back into the message this 404
  exists to keep out. So the promise is **withdrawn** rather than patched: the type
  no longer claims `os.ErrNotExist` at all, and the API layer matches the sentinel.
  A promise that holds for one of two idioms is worse than none.
- **The in-flight banner's two arms repeat six singular/plural ternaries.** Prose in
  two tenses, no behaviour difference. Declined in rounds eleven, twelve, fourteen
  and seventeen for the same reason.
