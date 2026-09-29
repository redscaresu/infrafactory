# STATUS

Current state, and the one place to start. It changes only when **Now** does, and stays under
150 lines (CI-enforced).

## Now

**AWS web stack**: [HLD](docs/hld/2026-09-27-aws-web-stack.md); its `## Epics` shows what is done.
Every agent-buildable story is merged. The next step is the user's: the AWS account setup
(`aws-scope-hand-setup`, runbook in `docs/operations.md` § Layer 3 (AWS) › Scope setup,
`REGION=us-east-1`). After it, the lead's `aws-scope-planted-leak-proof`, then `aws-whole-scope-adr`,
then `aws-layer3-gate-lift`, which needs the user's explicit approval.

What waits on the user is the board's **Waiting on you** view (`kind: operator` stories); what the
lead runs is **Lead-run**.

**Running the swarm:** `swarm.sh conduct <epic>` (the swarm-dev plugin) starts a fresh conductor per
epic in the HLD's herdr workspace; it builds, merges and unblocks the epic's stories, then reports. When codex is at its usage limit, the lead reviews the PR.

## Open work

One file per item in `docs/stories/`, with `status: ready | blocked | later`. The PR that
finishes an item deletes its file.

    grep -H '^status:' docs/stories/*.md

## Recent

Not stored here: the squash-commit titles are the record, so this cannot go stale or conflict.

    git log --first-parent -10 --format='%ad %s' --date=short main

History: git, the HLDs' `## Epics` lists, and the frozen `docs/archive/status/` (see its README).
