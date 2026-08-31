# S165 review pass 28 — one finding acted on, then clean

### [P2] Deletion keyed off every failure rather than the teardown's verdict

`len(failures) == 0` counts failures from every stage of the command, not just
the teardown. So a mock criteria failure followed by a perfectly clean
destroy-and-sweep left the empty run project behind permanently — the account was
provably clean, and the decision looked at the wrong signal.

There is now a `sandboxTeardownClean` flag set from the destroy and sweep
outcome specifically. **The teardown's verdict, not the command's.**

### Then clean

> The changes add the run-owned Scaleway project lifecycle with focused tests
> covering creation, deletion, cleanup placement, cancelled contexts, deploy
> refusal, and environment selection. I did not find a discrete correctness issue
> introduced by the diff that would require blocking the patch.

## What nine passes found, in one line

Almost nothing was a wrong computation. Every real finding was an **operation
ordered so that a failure left something behind**: a project created before its
config was validated, a cleanup attached to a branch some exit path skipped
(three times, in three different places), a delete inheriting a cancelled
context, and a decision reading the wrong success signal. The condition on
*whether* to delete was right from the start; where and when it ran was wrong
five times.
