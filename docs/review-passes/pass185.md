# Review pass 185 — removing the pull-request gate

`codex exec review --base main` on `chore/remove-the-pr-gate`. Clean on the
first pass, nothing raised.

> The changes consistently remove the Layer 3 PR workflow and update the
> README, runbook, ADR, status, and tests to reflect manual-only Layer 3
> validation. The adjusted tests use the checked-in CLI config as the
> allowlist source.

## What was removed, and what was not

Deleted: `.github/workflows/layer3-gate.yml` (565 lines).

Kept: `examples/layer3-gate/` — those are HCL fixtures for the shape gate and
have nothing to do with pull requests. The name is now slightly misleading and
renaming them is a separate, noisier change.

## Deleted rather than disabled

Converting it to `workflow_dispatch` looked like a six-line change. It is not:
the body is built around `github.event.pull_request` — base and head checkouts,
the head-SHA re-verification guard, and both comment-posting steps. Gutting a
hundred lines to keep a workflow nobody intends to run is worse than deleting
it, and `git log` is the undo.

Worth recording that the six-line estimate was made before reading the body,
and was wrong by two orders of magnitude.

## Two audits caught the fallout

Neither was anticipated; both were right.

**`TestReadmeLayer3GateClaimMatchesWorkflows`** refused a README still
advertising a gate that no longer exists. It is two-sided: with the workflow
present it demands the README describe it, and with it absent it demands the
README disclaim it. Exactly the drift-becomes-a-failed-test pattern this
project uses everywhere.

Its vocabulary had to widen, and that is the interesting part. It accepted only
the literal phrase **"not yet merged"** — written while the gate was unbuilt.
Requiring that now would force the README to imply the gate is *forthcoming*,
which is a different false statement from the one the guard exists to prevent.
It now accepts either that phrase or "no pull-request gate". Mutation-checked:
a README claiming the gate again fails it.

**The fixture tests scraped their allowlist out of the workflow's heredoc** —
deliberately, because a hardcoded copy in a test had already drifted from the
real list once and passed while the gate failed. With the workflow gone there
is only one copy left, so they read `infrafactory.yaml` through the same loader
the CLI uses. That is a strictly better source: it is the list the demo
actually runs against.

## The consequence, stated plainly in the ADR

This repository no longer proves a change against a real cloud automatically.
It proves it when somebody asks. That is a genuine reduction in coverage, taken
deliberately — mitigated by the fact that the gate was opt-in per pull request
and never ran on a change nobody labelled.
