# STATUS

Current state, and the one place to start. It changes only when **Now** does, and stays under
150 lines (CI-enforced).

## Now

**AWS web stack**: [HLD](docs/hld/2026-09-27-aws-web-stack.md); its `## Epics` shows what is done.
The three stories the user's 2026-09-29 decisions made ready have merged (#394, #396, fakeaws #41),
and policy-correctness is done. The user set up the AWS Layer 3 scope on 2026-09-29 (a member
account in us-east-1, every step verified; runbook in `docs/operations.md` § Layer 3 (AWS) › Scope
setup), and the planted-leak proof passed on it the same day: the sweep names every leak, a held
claim refuses reap, reap empties the account. Whole-scope ownership is recorded (ADR-0040; the
claim-sweep-reap epic is done). The gate lift (the aws-layer3-gate epic's last story) runs the AWS
gate on the aws path and checks account and user data after apply; next is aws-layer3-wiring-proof.

What waits on the user is the board's **Waiting on you** view: `kind: lead` stories, which the
lead runs once the user has done the story's **You:** line.

**Running the swarm:** `/swarm` (the swarm-dev plugin) reads the board, does the next step and loops,
resuming wherever it stopped; it stops where the user decides. `swarm.sh next` shows the step. When codex is at its usage limit, the lead reviews the PR.

## Open work

One file per item in `docs/stories/`, with `status: ready | blocked | later`. The PR that
finishes an item deletes its file.

    grep -H '^status:' docs/stories/*.md

## Recent

Not stored here: the squash-commit titles are the record, so this cannot go stale or conflict.

    git log --first-parent -10 --format='%ad %s' --date=short main

History: git, the HLDs' `## Epics` lists, and the frozen `docs/archive/status/` (see its README).
