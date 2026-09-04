# Review pass 156 — S163e, twenty-second round: the deletion converged

**12 findings, all accepted — and the shape has changed.**

Round twenty-one's review of round twenty found **seven behaviour regressions**,
two of them severe, all in the deploy lifecycle. This round, reviewing the
deletion that replaced it, found **one**:

- `ending` was cleared only on a route change, so a retry on the same page kept
  rendering the previous attempt's line for the whole minutes-long apply — a green
  "Deployed." under a live streaming log, or a red "what it may have left behind is
  reported at the top of the page" that a reader would take as the state of the
  deploy currently running. Cleared when a new attempt starts now, and
  mutation-checked.

Everything else is what a deletion leaves behind — stale comments and test
leftovers — and that class does not regenerate:

- The store's module docstring still said it keeps "how it ended" and that this
  "outlives navigation between scenarios". Neither is true: `endDeploy` deletes the
  entry unconditionally.
- `ending`'s own docstring named `resetDeployState` as what clears it. It does not;
  the clears are in `confirmDeploy` and `afterNavigate`, and a maintainer deleting
  the real one on the strength of the docstring would make finished banners follow
  the reader between scenarios again.
- The test-file `cleanup` helper claimed `endDeploy` "refuses to drop a running
  deploy". It drops it whatever its state; the only refusal is `beginDeploy`
  declining to *start* a second one.
- Two tests' "action under test" was a no-op `endDeploy` on an entry the line above
  had already deleted — leftovers of the removed `retireDeploy`, asserting a
  condition that already held.
- One test asserted on the *global* report store rather than filtering by scenario,
  so it passed only because of where it sat in the file.
- Seven tests destructured `deploys` and never used it.
- A comment in `onDestroy` had lost its subject in a copy and began mid-sentence.
- `previewFor` carried two merged drafts of one explanation.

## Two consistency fixes worth the name

- **A 2xx teardown whose body could not be read was reported as a failure**, while
  the identical deploy case is treated as provably clean — and both go through the
  same `writeActionResult`, which answers 2xx only for a clean result. A truncated
  200 put a red "resources may still be running" over an account the server had
  proven clean. Same reasoning, both verbs.
- **`mayHaveCreated` is set by the caller that uses it, not by the shared rule.** It
  was computed for teardown and destructured away again — a value produced only to
  be discarded, which the next reader has to prove dead before touching either
  caller. `words.recorded` is what says a verb can tell a recorded failure from an
  unreported one; teardown does not, so it gets no flag rather than one it drops.
- `deployingLabel`'s `stale = false` default handed out the unqualified
  present-tense claim to any caller who omitted the argument — the outcome the
  no-defaults rule two functions above exists to make unreachable by omission.

## What this round says about the last twenty-one

The deletion worked. One behaviour defect against seven, in a lifecycle that had
produced a regression per round for six rounds. What remains is bookkeeping after a
removal, which is finite by construction: there is no state left to guard, so there
are no guards left to get wrong.
