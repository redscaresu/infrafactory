# Review pass 153 — S163e, nineteenth `/code-review` round

**10 findings, 9 accepted, 1 declined.** The pair worth naming is the first and the
last: a defect, and the test that walked straight past it.

## Dismissing one report destroyed a sibling's account

`dismissReport` deleted the scenario's whole `deploys` entry whenever its outcome
was a report — without checking whether the dismissed report was the last one. Two
failed deploys file two reports; the operator cleans up the first project and
clicks "I have dealt with this"; the pointer and the entire apply log vanish while
the second report, naming a project that is still live, sits in the layout.

The justifying comment ("an outcome that IS a report is not rendered anywhere once
its report is gone") is false the moment a sibling survives.

**And the unit test walked exactly that path.** It destructured `deploys`, never
asserted on it, and passed. The unused binding was the tell.

## Zero values that make claims

`previewFor` was given `AlreadyLive: []string{}` two rounds ago so an absent list
could not read as "we could not look". Its mirror was left alone:
`AlreadyLiveUnknown` defaults to `false`, which is the positive claim "checked, and
nothing exists" — from a preview that never consulted the live store. Every
non-looking path of `liveDeploymentsOf` returns `(out, true)`; the struct was the
one place saying otherwise.

The estate page had the same shape: `let deploying: string[] | undefined = []`
initialises the tri-state to an ANSWER, masked today only by `estateState` being
`loading` until the first read returns.

## Retiring on a navigation that never left

`retireOnLeave` ran on every `afterNavigate` — including a click on the scenario
already on screen, which a probe confirmed reaches the hook. That discarded the
banner and the apply log of a deploy the reader had not left.

Two false starts worth recording. Comparing the incoming path against
`scenarioPath` never fired, because a reactive statement on `$page` has already
updated it by the time the hook runs. And `from?.url.pathname` **threw** on a
navigation whose `from` carries a null `url` — a throw inside `afterNavigate`
aborts it, so `loadDetail` never ran and every scenario page rendered blank. A
two-second console probe found that; two full suite runs had not.

The test needed care too: asserting immediately after the click passed on the first
poll, before the retire it exists to catch. It waits for the navigation's refetches
to settle first.

## The rest

- `noSuchScenarioError.Is` promised `os.ErrNotExist` compatibility. `os.IsNotExist`
  does not consult custom `Is` methods — it unwraps three concrete types and
  compares by `==` — so the promise holds for `errors.Is` only. Stated, because the
  difference is invisible at the call site.
- An unparseable 2xx reported "deploy failed: 200", naming a success status as a
  failure on the screen this slice exists to make trustworthy. It has its own
  wording now.
- Three comments in one file disagreed about the tri-state sentinel — two
  duplicated "Tri-state:" blocks above one declaration, one saying `null` and one
  `undefined`, with a third `null` comment inside `load()` directly above `…:
  undefined`.

## Declined

**`teardownOutcome` computes `mayHaveCreated` and the estate page ignores it.**
Real, and the same class the deploy path spent four rounds on: an unprovable
teardown gets a transient row banner where an unprovable deploy gets a persistent,
dismissible report. It is a different verb, a different store shape and a different
page — declined in rounds fourteen and eighteen for that reason, and recorded in
STATUS as a named follow-up rather than bolted on at round nineteen. What is fixed
here is the silence: `teardownOutcome`'s docstring now says the flag is computed by
the shared rule and not yet consumed.
