# Codex review pass 44 — S154 `live observe`

`codex exec review --base main` on `s154-live-observe` at `b36bee1`.

First pass under the **one-clean-pass** convergence rule (AGENTS.md, reduced
from two on 2026-08-31 for cost).

One finding, accepted — and accepted as more than its P2.

## [P2] A live deployment that cannot be probed was skipped, not failed

`observeDeployment` skipped when the record carried no address or no port, and
the command exited zero. My comment excused it as "deployments created before
S154 carry neither" — which was **wrong about where the case comes from.**

`registerDeployment` captures the address best-effort:

```go
address, _ := harness.LiveEndpoint(workDir, "load_balancer")
```

So an apply that succeeded without producing a load balancer address writes a
record with `Port` set and `Address` empty — from **today's** deploy path, not
from a legacy record. `live observe` would then report success while a live
deployment sat unmonitored. That is the false green this project refuses
everywhere else: the record says something is running and the command has just
admitted it cannot tell.

Fixed: a **live** deployment that cannot be probed fails, naming which half is
missing (the two have different causes and different fixes), the project id, and
the way out. A **released** deployment still skips — there is genuinely nothing
to observe.

Covered by `TestObserveFailsOnALiveDeploymentItCannotProbe` (all three shapes)
and `TestObserveSkipsAReleasedDeployment`.

## Re-reading my own fix, per the one-pass rule

The second pass is what caught the pass-41 regression, so under one pass the
discipline is to treat the fix as the likeliest defect and re-read it against
its own class. The class here is *"we could not check" must not look like
success*. The other `skip` in that function is:

```go
if d.Undecodable { return skip("record could not be decoded") }
```

Checked rather than assumed: `FilesystemStore.List` returns an undecodable
record in **both** returned slices, so the caller's `unreadable` loop already
fails for it. Not a false green — but the pairing was undocumented, so the skip
now says it is reported as a failure below. The skip records that the deployment
was not probed; the failure makes the command exit non-zero. Neither alone does
both.

## Nothing declined this pass.
