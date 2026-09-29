# Codex review pass 95 — S156d

Two findings, both accepted. The first is the subtlest defect in the slice.

## [P2] The upgrade's own downtime read as the failure it fixed

`upgraded_at` is written **after** the apply returns. `live observe` can run
during an apply that takes minutes — S158's journey test does exactly that — so
those probes are stamped *before* `upgraded_at` and landed in the "before" bucket.

The last one wins under pass 91's failure-must-persist rule. So a
`connection refused` captured mid-changeover became "the failure the upgrade
repaired", and the corpus would have gained a prescriptive rule **teaching a
remedy for an outage this tool caused**.

Every ingredient was already in the codebase and none of them pointed at it:
S155b knew the apply is long, S158 deliberately observes during one, and pass 91
had just tightened the "before" side in a way that made the most recent
observation decisive.

Records now carry `upgrade_started_at`, stamped before the apply. Probes inside
`[started, finished)` are **discarded** rather than assigned to either side —
they describe neither configuration, only the changeover. A record without the
boundary is declined rather than guessed at.

## [P3] The rule overstated its own evidence

`ObservationsBefore` was `len(before)` — the whole pre-upgrade history. So
healthy → healthy → 503 wrote "3 probe(s) had reported" when one did.

Accepted despite the P3, because the corpus is read as **guidance**: an inflated
confidence is a small lie that survives, and this rule's whole value is that it
states what is known. It counts the trailing adverse run now.
