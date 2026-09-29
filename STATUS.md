# STATUS

Current state, and the one place to start. It changes only when **Now** does, and stays under
150 lines (CI-enforced).

## Now

**AWS web stack** ([HLD](docs/hld/2026-09-27-aws-web-stack.md)). Seven of its ten epics are done
or built; every agent-buildable story is merged. What is left waits on the user:

1. **User:** run the AWS account setup, `docs/operations.md` § Layer 3 (AWS) › Scope setup, with
   `REGION=us-east-1` (story `aws-scope-hand-setup`, kind operator). Paste each check's output
   into the PR with ids cut to their last four characters; never the secret key.
2. **Lead:** `aws-scope-planted-leak-proof` on that account, then `aws-whole-scope-adr` (ADR-0040),
   which closes `aws-layer3-claim-sweep-reap`.
3. **Lead, with the user's explicit approval:** `aws-layer3-gate-lift`, the first time AWS is
   admitted at Layer 3.
4. Then scope `aws-layer3-wiring-proof` and `aws-web-live-on-real-aws`; the load balancer is last.

**Decisions waiting on the user:**
- Scaleway `no_public_endpoints` with `target: database` reaches a server-IP policy, and the ip_id
  pitfall gives servers public IPs that `web-app-paris` forbids: story
  `no-public-endpoints-criterion-routing`.
- Make fakeaws's `provider-smoke` (~30 min) a required check on fakeaws `main`?
- Add an `OPENROUTER_API_KEY` secret so `scenario-gate` runs the model (today it skips green).

**Running the swarm:** `scripts/swarm.sh story <slug>` per ready story, `scripts/swarm.sh watch` to
wait (it wakes on finished checks, conflicts, stalls and blocked agents), `scripts/swarm.sh unblock`
after each merge. Codex is often at its usage limit: the lead reviews those PRs.

## Open work

One file per item in `docs/stories/`, with `status: ready | blocked | later`. The PR that
finishes an item deletes its file.

    grep -H '^status:' docs/stories/*.md

## Recent

Not stored here: the squash-commit titles are the record, so this cannot go stale or conflict.

    git log --first-parent -10 --format='%ad %s' --date=short main

History: `docs/status/ARCHIVE.md` (per-arc close-outs) and `docs/status/STATUS_HISTORY.md`.
