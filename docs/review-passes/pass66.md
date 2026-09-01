# Codex review pass 66 — S156a

One finding, accepted. **It reverses pass 63's decision, correctly.**

## [P2] A live entry could still be swallowed as a duplicate

Pass 63 stopped the merge refreshing across sources, and chose to keep the
historical behaviour for the mismatch case: skip the post entry. That preserved
the old dedup — and silently dropped the live entry whenever an older
`descriptive` rule happened to share its `(resource, rule)`.

Losing it loses the `last_seen` that retirement runs on. The observation, and the
timestamp deciding when it retires, are gone with nothing to rebuild them from —
which is the failure this whole slice exists to prevent, arrived at from the
other side.

`mergeKey` now includes the source **for `live` only**. The asymmetry is the
argument: dropping a duplicate `avoid` loses a rule the corpus already states in
other words; dropping a duplicate `live` loses information nothing can
reconstruct. Every other source keeps the historical identity.

## A correction to the previous commit

Pass 65's commit message said the declined genesys finding was "noted in the S156
plan". It was not — the anchor I edited against lives on the unmerged
`s156-plan` branch, and the edit silently failed while the commit went ahead.
The note is now in the plan on this branch, under *"Known gap: genesys is outside
the sweep's pitfall handling"*.

Worth stating rather than quietly fixing: the commit message claimed a record
that did not exist, which is the same class of thing this slice keeps finding in
the code.

## Nothing declined this pass.
