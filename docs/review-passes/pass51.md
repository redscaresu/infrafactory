# Codex review pass 51 — S155b `live upgrade`

Two findings, both accepted, both **P1 and both data-safety**.

## [P1] `--from` pointing at the deployment's own workdir emptied it

`replaceDeployedHCL` removes the superseded `.tf` files before copying the new
ones in. So a `--from` naming the deployment's own workdir deleted the files it
was about to read, and the workdir ended up holding **no configuration at all**
— for infrastructure that is still running.

Refused now, for the workdir itself and for any path inside it, with symlinks
resolved before comparing: a symlinked path deletes the files just as
effectively as a literal one.

## [P1] A rejected configuration was left in the workdir

An init or plan failure never reaches the cloud, so the deployment is still
running the **old** configuration — while the workdir now held the new, rejected
one. Every later operation would plan against something that was never applied.

Restored from `.infrafactory-previous/` on exactly the predicate pass 50
introduced for the tag, `applyRan(err)`. The two decisions are the same decision:
*did anything reach the cloud?* A failure **during** apply keeps the new
configuration, because something may be running from it and reverting would hide
that.

## The shape of this slice's findings

Passes 50 and 51 both landed on the same question and I had answered it in only
one of the places it applies. `applyRan` now governs the tag, the address and the
workdir contents together, which is the fix the second pass really made — not two
patches, one predicate applied consistently.

## Nothing declined this pass.
