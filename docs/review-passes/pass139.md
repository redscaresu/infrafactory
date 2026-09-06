# Review pass 139 — S163e, fifth `/code-review` round

**13 findings, 12 accepted, 1 declined.** The round after the cut, and the
findings are what a cut leaves behind: survivors describing machinery that has
gone, and one place where the client had not been taught a rule the server had
already adopted.

## The one that mattered

`confirmDeploy`'s catch called `forgetDeploy` on **every** rejection, under a
comment asserting *"Nothing was started, so nothing is shown as running."*

That is false by design. `deployHandler` runs the apply on a `destructiveContext`
— detached from the request — because "a client disconnecting halfway would leave
resources with no completed record". So a sleeping laptop, a wifi hop or a proxy
timeout four minutes into an apply rejects the fetch while the apply keeps
running and keeps creating billable infrastructure. The page deleted the entry
and every progress line collected so far, and rendered *"web-app-paris: Failed to
fetch"*.

Only the server can say nothing started. `DeployError.startedNothing` carries
that word: true for the statuses `deployHandler` can only produce **before**
`Deploy` is called (400, 404, 405, 423), false for everything else — 500
explicitly included in "everything else", because `writeActionResult` returns it
for a deploy that ran and errored.

Mutation-checked in both directions:

- forget on every failure → `a deploy whose connection drops is not called a
  deploy that never ran` fails
- forget on none → `a refused deploy does not claim resources may be running`
  and `a refusal does not outlive the attempt that caused it` fail

## A rule broken twelve lines above where it is written

`alreadyLiveWarning` returned early on `already_deploying`, discarding
`already_live` and `already_live_unknown` — directly above its own comment
saying it "must not DISCARD what was found", which was written to fix the same
defect on a different branch of the same function.

The three warnings answer different questions and can all be true at once, so
they accumulate now. The discarded one was the strongest: *"dep-existing is
already deployed from this scenario. Deploying again creates a SECOND project and
a second bill."*

## The third copy of the emptiness question

Pass 138 closed this class by extracting `knownEmpty` so two derived claims about
emptiness could not contradict each other. `estateSummary`'s `failed` branch was
a **third** claim that neither knew about: with a deploy in flight and a refresh
that errored, it rendered *"Whether anything is running is unknown"* directly
above the banner naming the billable apply.

`load()` keeps `deploying` across a failed read on purpose — it is the one thing
still known — so the failed branch carries it too.

## The rest

- `confirmDeploy` was the only async path on the page without a `navigation`
  token, so a deploy failure for A could render on B's page. The refusal moved
  out of the scenario-keyed store into a component variable, which is what
  removed the scoping that made the old path safe.
- A refusal was never cleared when a new deploy started, so a successful retry
  rendered "already deploying" and "deployed" simultaneously.
- `liveDeploymentsOf`'s docstring said its bool "reports whether the answer is
  complete"; it means the inverse. Editing to match the stated contract would
  invert the flag whose entire purpose is preventing a false "nothing is
  deployed".
- Three comments and a test docstring still described `deploy_complete` and
  reload-restoration — both deleted in the same PR.
- `ui/e2e/deploy.spec.ts`'s "a reload does not disable deploy for an idle
  scenario" was **vacuous**: the page no longer fetches `/api/deployments` on
  mount, so its route mock was never hit and the assertion was the unconditional
  default. Deleted; three real tests added in its place.
- `TestPreviewWarnsWhenTheScenarioIsApplyingRightNow` asserted `AlreadyLive` was
  empty against a lister holding **nothing**, so it passed for the wrong reason
  and would pass with the feature removed. The fake now holds a live deployment
  of another scenario; the assertion is mutation-checked.
- The hand-rolled membership loop is `slices.Contains`.

## Declined

**The preview endpoint reads the whole estate on every Deploy click.** Real: a
filesystem walk plus a decode per record, and the estate page polls the same
`List()` independently.

Left alone. It is one read per deliberate human click, against an estate bounded
by what a person is willing to pay to keep running, and the alternative — a
cached index — is a second source of truth that can disagree with the store,
which is the exact class the last three rounds were spent deleting. The cost is
now recorded in STATUS.md and ADR-0027 instead, which was the substance of the
finding.
