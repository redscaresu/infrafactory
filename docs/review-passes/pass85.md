# Codex review pass 85 — S156c

One finding, accepted.

## [P2] The persisted identity was narrower than the gate's

Pass 77 keyed live entries on the **normalized detail**, which fixed the
instability. But the gate does not group on the detail alone — it groups on
`(status, drift, detail)`, and it does that deliberately:

- `unhealthy` is "it told us it is broken"; `unreachable` is "we got no answer".
  Different facts, and ADR-0024 says they must not merge.
- A health failure and a version mismatch are different problems with different
  fixes, and their details come from different probes.

So two candidates the gate had just been careful to keep apart could land on the
same corpus key, and the second would refresh over the first. **A reproduced
failure would be silently lost** — the exact outcome the promotion gate exists to
prevent.

## Derived, not reassembled

`Candidate.Key()` now lives beside the gate and composes the same three fields it
groups by, and `live learn` persists that. The alternative — building the key in
the CLI — is how the two identities drift apart the next time the gate learns to
distinguish something new.

Same reason `StaleLivePitfalls` and `RetireStaleLivePitfalls` share one
`partitionStale`: when two places must agree, make one of them the other's
source rather than asking a reader to keep them in step.

## Nothing declined this pass.
