# Codex review pass 36 — S166+S167 cutover

`codex exec review --base main` on `s166-cutover` at `e0007f3`.

Two findings, **both declined**. Both ask for the same thing — fall back to
reading `scaleway_account_project` out of `terraform-live.tfstate` when a
workdir has no marker — and that is the dual model this arc deliberately
dropped.

- [P2] Handle pre-cutover state without a marker — `internal/harness/orphan_sweep.go`
- [P2] Allow reap to reclaim legacy state-only runs — `internal/cli/reap_command.go`

## Why declined

**The scenario has no instance.** Checked rather than assumed: the live store
does not exist, `./output` contains no `terraform-live.tfstate`, and the only
two on disk are committed test fixtures under `internal/cli/output/`. Every
Layer 3 run destroys and sweeps before it finishes, and the arc closed with zero
resources left, verified against the API. There is no workdir this would rescue.

**It is the decision the cutover was.** ADR-0025's amendment records it: no
fleet, no external consumers, nobody mid-migration, and *"two models means two
code paths, and two code paths is where this arc's bugs came from."* Passes
30–35 are that lesson in evidence — six wrong placements of a right condition,
every one of them a path that had been depending on something implicit.

**A fallback would be untested by construction.** Nothing can exercise it
without hand-fabricating a workdir, so it would be a second project-resolution
path, guarding a case with no instance, that no test run ever walks. That is
strictly worse than refusing.

**The safety property codex is protecting is already held**, as of pass 35: a
pre-cutover record is *not* silently forgettable. `reclaimable()` accepts marker
or state, teardown still destroys through tofu, the Account delete is skipped
rather than performed on no evidence, and the sweep refuses loudly because the
blast radius is unknown. Kept, visible, red.

**reap refusing outright is right for reap specifically.** Its contract is
"destroy this run's project and prove the account is clean". Without a marker it
can do neither half — it could run `tofu destroy` and then fail the sweep
anyway, which is destroying real resources on a path that reports failure
regardless. `live teardown` differs because it holds a record that names what to
destroy.

## Recorded in code, not just here

Both sites now carry the reasoning inline, so the next reviewer meets the
decision instead of re-deriving the finding.
