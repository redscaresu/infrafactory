# Codex review pass 104 — S161

One finding, accepted. **The third variant of one defect in this slice**, and the
second time in two passes that I fixed the claim in front of me and left its
neighbour.

## [P2] "No live deployments" under a banner saying records could not be read

With zero decoded deployments and a non-empty `unreadable` list, the empty-state
panel still said *"No live deployments"* — directly beneath the warning that those
records may describe running, billable infrastructure.

Pass 103 fixed the **summary line** for exactly this class and I left the panel
below it alone, having just written that checking the field in front of me and
not its neighbours was what caused the previous three.

## So it is derived, not repeated

`knownEmpty(deployments, unreadable, state)` is now the single condition under
which anything on this page may claim nothing is running, and both the summary and
the panel read it. Three things must hold: the read succeeded, it returned no
deployments, and there is nothing the store could not decode.

**An undecodable record is not an absence of infrastructure. It is an absence of
knowledge**, and this page's entire purpose is keeping those apart.

## And closed on the page, not the branch

The new e2e asserts that under a failed read, and under unreadable-records-with-
no-deployments, **nothing anywhere in the body** says the estate is empty. A
fourth place to make that claim would fail it.

That is now the second class in two slices closed by asserting over the whole
payload or the whole page rather than the field or branch that happened to be
found — S159a's year-one timestamps being the first. The pattern is worth naming:
when the same defect appears three times, the fix is not a third careful
correction, it is a test that cannot be satisfied by a careful correction.
