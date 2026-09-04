# Review pass 152 — S163e, eighteenth `/code-review` round

**10 findings, all accepted.** One reverses a call I made twice, and the reason is
worth stating: I had been treating a vacuous truth as if it were a falsehood.

## Half a guard

Round seventeen put the "one deploy at a time" rule in the store, where the state
lives. The store cannot stop the POST — and `confirmDeploy` ignored its answer and
sent one anyway. The 423 that came back was handed to `refuseDeploy`, which cleared
**the first deploy's** log and marked it finished while it kept creating
infrastructure: verbatim the failure the guard's own comment says it prevents.

`beginDeploy` returns whether the deploy may proceed, and the caller honours it.

## A vacuous truth is not a falsehood

Round fifteen made the cross-origin guard a plain error, and round sixteen did the
same to the collection's 405, on the argument that `started_nothing` is a claim
about an apply — meaningless on a read, wrong-verbed on a teardown.

Meaningless is not false. Nothing *was* started on either path, because no handler
ran. Withholding a true claim is not neutral: a refused deploy POST then read as
"we do not know what happened", and the page pinned a permanent "it may have
created resources that are still running" for a request the middleware rejected
outright — the alarm fatigue `dismissReport` exists to avoid, manufactured where
the truth was known.

**The rule, settled: any path that answers before a deploy could begin may say so.**
A vacuous truth on a PUT costs nothing; a missing one manufactures a false alarm.

## `undefined` is what an absent field IS

Round seventeen collapsed `deploying`/`deployingKnown` into one tri-state using
`null` for "never told" — and left `= []` defaults on both functions. So
`knownEmpty(d, u, state, payload.deploying)`, written the obvious way, passes
`undefined` when the server omitted the key, the default fires, and the answer is
"Nothing is deployed." on an estate that may be mid-apply. The comment arguing that
a value "cannot be forgotten the way an argument can" was undone by a default,
which is a third way to forget.

The sentinel is `undefined`, there is no default, and only an array counts as an
answer. And the summary now SAYS the gap — it had degraded to a bare "0
deployments", a read-derived emptiness claim with the caveat removed.

## The rest

- `ErrNoSuchScenario` was `fmt.Errorf("no such scenario: %w", ErrNothingStarted)`,
  so its own `Error()` read "no such scenario: nothing was started" — the
  self-contradicting, sentinel-leaking message the CLI built two custom types to
  avoid. `LiveDeployer` escaped it only by returning its own type; the interface is
  exported, and an implementer signalling the documented way would put that sentence
  in front of an operator.
- `loadDetail`'s catch overwrote `status` with the read error, so a transient
  refresh failure after a SUCCESSFUL save replaced "Saved" with "request failed:
  500" — telling a reader their write had failed when it had not.
- `deployOutcome`'s recorded branch re-implemented `actionOutcome`'s failure-reason
  assembly, bypassing the function whose docstring is "ADR-0024's rule, once". It is
  a fourth entry in the `words` object now.
- `ActionResult.deployment` was added to the wire and never to `types.ts`, so a TS
  caller reading it is a compile error and an auditor reading the contract concludes
  the whole recorded branch is unreachable.
- Two keying comments described the index/content scheme the code stopped using
  when reports got ids.
- **Reports have their own store.** They change twice a session; `deploys` is written
  once per progress line. Reading them out of it subscribed the root layout —
  mounted on every route — to thousands of notifications for data that had not
  changed. Declined five times as a performance point; accepted as a structural one,
  because two lifetimes in one store is what made the coupling.
