---
kind: lead
status: blocked
blocked_by: [aws-hold-for-read-adr-0040, aws-hold-for-read-release, aws-hold-for-read-guard, aws-hold-for-read-hold, aws-hold-for-read-runbook]
epic: aws-hold-for-read
depends_on: [aws-hold-for-read-adr-0040, aws-hold-for-read-release, aws-hold-for-read-guard, aws-hold-for-read-hold, aws-hold-for-read-runbook]
touches: [docs/operations.md, docs/epics/aws-hold-for-read.md, docs/hld/2026-09-30-import-production-state-into-the-mocks.md]
risk: high
---

# Prove the hold once on real AWS (held stack live, second claim refused, release with the copy, sweep empty, cost), then close the epic

**You:** say go before the paid apply; the lead waits for it in the conversation.

prod-read-seam-real-recording waits on this slug, so the recording never runs on a hold not yet
proved on real AWS.

**Lead,** serial with every other real-AWS run, under the €5 cap, with the Layer 3 key file and the
out-of-git config (operations.md ### Running aws-web-live). No agent runs any step.
1. `reap scenarios/training/aws-web-live.yaml --config <cfg> --dry-run` reports every swept
   collection empty (the baseline). Generate aws-web-live's HCL and test it green against fakeaws,
   both **without** `--config <cfg>`, so neither can reach real AWS.
2. After the go: `test scenarios/training/aws-web-live.yaml --config <cfg> --hold`.
3. `stat` the copy and its dir; `jq '.resources | length'` on the copy, printing only the count.
4. While held: `test scenarios/training/aws-web-live.yaml --config <cfg>` is refused by the guard
   before any AWS request; plain `reap ... --config <cfg>` (no --take-over) refuses naming the holder;
   the instance's http endpoint (from step 2's probe stage) answers, and `reap --dry-run` reports the
   scope not empty.
5. `reap ... --config <cfg> --take-over <holder> --release-hold`. If it refuses, run plain
   `reap ... --take-over <holder>` as the exit, record that, and file a story: the proof failed.
6. `reap --dry-run` reports every swept collection empty.
7. Cost from instance-hours, confirmed in billing.
8. Delete the copy: no read was taken and it holds the account id.

No real Ctrl-C leg: the hold process exits after printing, and test's interrupt guard was proved on
real AWS on 2026-10-10 (STATUS.md:8-13); the hold's interrupt paths are proved against fakes.

**PR:** a "Last run <date>, under €x" line in docs/operations.md's hold section; deletes
docs/epics/aws-hold-for-read.md; the HLD's ## Epics line reads `aws-hold-for-read — done (#PR)`, and
Rollout 0's "with `--keep`" (HLD:442) reads "with `test --hold`" and its "`reap` clears it" reads
"`reap --release-hold` clears it".

**Unwritten rules it touches (S156d):** serial real-AWS runs under the €5 cap; verify against the
thing itself, not the printed stage; no account id in a committed file or PR body; a refusal of the
release is a failed proof, not a workaround.

**Done when:**
- The user said go in the conversation before step 2; step 1's dry-run reported the scope empty.
- Step 2's output has aws_scope_claim_kept (skip) naming the holder and the release command, and no
  destroy, aws_scope_sweep or aws_scope_release stage. `stat` shows the holds dir 0700 and the copy
  0600; the copy's resource count is > 0.
- Step 4: the guard refused `test` with no AWS stage; plain reap refused naming the hold's holder; the
  endpoint answered and the dry-run listed the stack's instance, VPC and security group.
- Step 5 exited 0 via `--release-hold`, with a destroy that removed more than zero resources before
  aws_scope_sweep and aws_scope_release passed; step 6 reports every swept collection empty; `ls` shows
  the copy gone after step 8.
- ADR-0040's hold amendment and the runbook section name the merged flags (`--hold`, `--release-hold`)
  and path (`.infrafactory/holds/`); a mismatch blocks closing the epic and the lead files a docs story.
- The PR records the cost, under €5; a grep for the account id over the diff and the PR body finds
  nothing; the epic file is gone; `make doc-hygiene` passes.
