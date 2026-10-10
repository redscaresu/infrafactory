# STATUS

Current state, and the one place to start. It changes only when **Now** does, and stays under
150 lines (CI-enforced).

## Now

**AWS Layer 3 is real (2026-10-10).** Epic aws-web-live-on-real-aws is done: on the user's own
AWS member account, the first real `test` measured the key's policy (nothing refused), the claim's
contention and interrupt legs held, one LLM `run aws-web-live --holdout` reached target on
iteration 1, the holdout failed a stack that opened 22, and a real apply failure
(InvalidSubnet.Range) taught `pitfalls/aws.yaml` an entry that the next iteration used. Every run
ended with the account swept empty; all real spend stayed under EUR 0.10 of the EUR 5 cap.
[HLD](docs/hld/2026-09-27-aws-web-stack.md); its `## Epics` shows what is done. The HLD's next epic
is aws-web-stack-load-balancer; the user plans to pivot to other work first, so it waits.

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
