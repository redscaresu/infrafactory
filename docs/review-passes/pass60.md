# Codex review pass 60 — S156a

One finding, accepted. **The incomplete half of pass 59's own fix.**

## [P2] The sweep merge discarded refreshed timestamps

Pass 59 made `pitfall-merge` preserve `live` entries. It preserved the *entries*
and not their *freshness*: `merge` skips anything whose `(resource, rule)` is
already in pre, so a live rule **re-observed during the sweep** kept pre's older
`last_seen`.

The effect is quiet and bad. Retention silently reverts to *first observed*
instead of *last observed* — the exact thing `TouchLivePitfall` exists to prevent
— and the entry then retires early, deleting a rule that is still true.

`merge` now carries forward a newer timestamp for entries it already has, and
reports `refreshed=N` alongside `kept_new=N`. An older, absent or unparseable
candidate never wins: the existing value is evidence somebody recorded, and
replacing it with something unreadable would make the entry look never-seen and
retire it.

## The pattern, one more time

Pass 59 found a vocabulary added without updating its enforcers. I then swept for
the same class, found the sweep would delete live entries, and fixed that — and
introduced this by fixing only the half I had gone looking for.

**Preserving a record is not the same as preserving what the record says**, and
that distinction is the whole of both findings.

## Nothing declined this pass.
