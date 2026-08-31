# Codex review pass 43 — S166+S167 cutover

`codex exec review --base main` on `s166-cutover`. Same code as pass 42; only
that pass's archive file differs.

**Clean.** No findings.

> I did not find a discrete correctness, safety, or maintainability regression
> in the changed code. The main lifecycle changes are covered by focused tests
> and the updated teardown paths consistently use the new run-project
> marker/provenance model.

**Second consecutive clean pass. The loop converges here.**

## The arc in one table

| Pass | Outcome |
|---|---|
| 30 | sweep target moved to the marker |
| 31 | 3 findings — `reap` never deleted; unreadable state read as "no strays"; no-state deployments unreachable |
| 32 | 2 findings — **D6 moved to the Account delete**; `run` auto-destroy gap closed |
| 33 | 2 findings — teardown/reap destroyed against the shared project; interrupt guard never deleted |
| 34 | 1 finding — same asymmetry in `run`; the existing audit read **one file** |
| 35 | 1 finding — marker-only `reclaimable()` made pre-cutover records forgettable |
| 36 | 2 findings — **both declined**, a state-file fallback with no instance to serve |
| 37 | 3 findings — three error paths still fell back; class closed at the seam |
| 38 | clean |
| 39 | 1 finding — the prompt contradicted itself; the pitfall still said the NIC was impossible |
| 40 | 2 findings — `deploy` could lose a project to Ctrl-C |
| 41 | 1 finding — **a regression pass 40 introduced**; create made uncancellable |
| 42 | clean |
| 43 | clean |

Sixteen findings across fourteen passes: fourteen accepted, two declined.

## What the shape of them says

**Almost none was a wrong computation.** The conditions were right nearly every
time; where and when they ran was wrong repeatedly. Removing a deletion that
Terraform used to perform invalidated every path that had been depending on it
without naming the dependency, and those paths surfaced one review at a time
rather than from any single reading of the diff.

Three lessons that outlive this slice:

1. **A narrowly scoped audit is worse than none.** The apply/destroy project
   asymmetry had a test from the start; it read `test_command.go` alone, so the
   defect landed in three other files with the audit green throughout.
2. **Close classes at the seam, not by inspection.** Three passes fixed
   individual empty-project-id instances, none of them a literal `""`. Making
   the seam refuse an empty id ended it.
3. **A fix can be a trade.** Pass 40 closed a leak window and opened another
   inside it. Both properties were needed at once.

Real-cloud verification is separate and already done:
[docs/status/s168-cutover-canary.md](../status/s168-cutover-canary.md).
