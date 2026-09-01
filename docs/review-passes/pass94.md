# Codex review pass 94 — S156d

One finding, accepted, and the most dangerous of the slice.

## [P2] An absent "before" is not an empty one

`loadResourceBlocks` returns an empty map for a directory that does not exist —
sensible on its own, and load-bearing in the wrong direction here.

If `.infrafactory-previous/` is missing, every resource in the current
configuration reads as newly **added**. With a single resource in the file,
`singleChangedResource` returns it and the extractor emits the whole body as "the
fix" — so the corpus gains a rule asserting that the entire configuration was the
remedy for a failure whose before-state was never seen.

That is corpus corruption, and it is silent: the entry looks exactly like a
correct one.

A record reaches that state for ordinary reasons — it predates S155b, or the
working directory was cleaned since the upgrade — so `Repairs` checking
`WorkDir != ""` was never enough. The stash must exist.

## Five findings, one shape

Every finding in this slice is the same: **a rule the surrounding code already
enforced, which the new path did not inherit.** The ambiguity gate (deletions
uncounted), the per-cloud handling, failure-must-persist, attribution-from-detail,
and now "an absent input is not an empty input" — which the D6 purge, the orphan
sweep and `live reconcile` all state in their own words as *never let missing data
read as clean data*.

S156a's lesson was that a change lands in every place that reads it. This is its
mirror: a new path inherits every rule the existing paths enforce, and none of
them are written down where the new path can see them.
