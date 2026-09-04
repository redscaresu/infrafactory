# Review pass 145 — S163e, eleventh `/code-review` round

**10 findings, 7 accepted, 3 declined.** One of them is concrete enough to
overturn a decline from the previous round.

## The reader's obvious next action deleted the leak report

A deploy fails with "it may have created resources that are still running —
project 7c98d82e is live and could not be deleted". Round nine and ten made that
banner survive navigation, because it is the only place a project with no live
record is ever named.

Then the reader does the obvious thing and clicks Deploy again. `beginDeploy`
overwrote the entry. If the retry succeeded, the banner became "Deployed." and the
next navigation dropped it — and 7c98d82e was named nowhere in the UI at all.

Reports are now a list that survives `beginDeploy` and accumulates, because two
failed attempts leak two projects.

## The status allowlist was already wrong, so the server says it now

Round ten declined the body-discriminator on the grounds that the concrete harm
needed a future edit. It did not: `deployHandler` answers **404 in two places** —
"no such scenario", before the apply, and an `os.ErrNotExist` returned by `Deploy`,
after it. The client called both "nothing started", so a post-apply
`os.ErrNotExist` from any `DeploymentDeployer` implementation would discard the log
of a running apply and tell the reader nothing happened.

`writeRefusal` writes `started_nothing: true`, on the paths that reject a request
before it can touch the cloud — including the origin guard, which is not
`deployHandler` at all. The client reads the body; the allowlist is deleted. Absence
means unknown, which is the safe default. `TestOnlyPreApplyRefusalsSayNothingWasStarted`
pins which paths may make the claim, including the two that may not.

## Reports were reachable by luck

Round ten put cross-scenario reports in the `{:else if detailError}` branch, so
whether an unread leak report was visible depended on an unrelated GET failing.
They render in the LAYOUT now — including for the reader who follows the message's
own advice and goes to the Deployments page.

## Three more

- The 999-line cap kept the tail and dropped the head, which is where `deploy`
  writes the scenario, its ref, its TTL and its **workdir** — the only place those
  appear when the request never returns an ActionResult. It drops from the middle
  now and says how many lines went, because a truncated log with no marker reads as
  a whole one.
- The refusal path skipped attribution for the whole `startedNothing` class on the
  grounds that "the server names it deliberately" — true only of the lock refusal.
  "invalid json body", "method not allowed", the origin guard's message and the
  no-`--allow-deploy` 404 all rendered unattributed in a slot shared with every
  scenario. Prefixed unless the message already contains the name; mutation-checked
  in both directions, since over-prefixing is what produced "web-app-paris:
  web-app-paris is already deploying".
- `types.ts` declared `already_live` required while `alreadyLiveWarnings`
  deliberately treats an absent list as "we could not look". The type would make
  that branch provably dead and invite its deletion.
- The in-flight banner's heading was staleness-qualified from `estateState` while
  its body branched on `loadError` — under a comment saying they had been unified.

## Declined

- **`estateSummary` derives "is anything applying" three ways.** All three inside
  one function, on one variable, in ten lines. No behaviour change.
- **`refuseDeploy`/`finishDeploy` return the store unchanged when the entry is
  missing.** Unreachable: both are only called after `beginDeploy` in the same
  handler.
- **A shared rationale is copy-pasted across four sites, and comments carry dated
  "corrected 2026-09-04" entries.** The dated entries are deliberate in this
  codebase — a false explanation is treated as a defect, and recording that one was
  found is how the next reader knows not to trust the old shape.
