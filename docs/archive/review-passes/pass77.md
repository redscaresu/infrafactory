# Codex review pass 77 — S156c

One finding, accepted. **It is the direct consequence of pass 76 that I should
have seen while making it.**

## [P2] The rule text is not stable, so exact identity duplicated

Pass 76 gave live entries **exact-text identity** to stop fuzzy dedup discarding
distinct lessons. But a live rule *states its evidence*:

```
... Evidence: persistent, across 3 deployment(s), longest run 5.
```

Those numbers grow as the same failure keeps being observed. So the next
`live learn` produces a different string for the same lesson, exact identity sees
something new, and **the corpus gains an entry every time the counters tick** —
while appearing to refresh.

My own `TestLearnTwiceRefreshesRatherThanDuplicating` passed only because the
evidence did not change between the two runs. It does now.

## Identity separated from presentation

`ObservedKey` holds the **normalized detail the gate grouped by** — the thing that
does not change however much evidence accumulates. Refresh matches on it and
rewrites the rule text, so the corpus carries the *strongest* evidence rather
than whichever was written first.

An entry with no key is refused outright: it could never be recognised again, and
writing something unmaintainable is worse than refusing.

## And the same bug one layer out, found by enumerating readers

`cmd/pitfall-merge` keyed live entries on `resource + rule + source`. With the
text now changing, a sweep would treat a refreshed lesson as a new one and
duplicate it — the identical defect, in the identical shape, one component away.

Found by asking *who else reads this?* before running the next pass, which is
S156a's lesson applied on purpose rather than after the fact. The merge keys on
`ObservedKey` now, falling back to the text for entries written before the field
existed.

## Nothing declined this pass.
