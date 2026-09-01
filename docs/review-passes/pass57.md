# Codex review pass 57 — S155b `live upgrade`

One finding, accepted. **The incomplete half of pass 56's own fix.**

## [P2] The re-read was checked and then discarded

Pass 56 re-read the record before writing, to avoid overwriting a concurrent
teardown. It used the fresh copy **only** for the released check, then wrote back
the stale `d` loaded before the apply — discarding anything another writer had
added in between.

An apply takes minutes. `live observe` on a cron appends observations during
exactly that window, and those observations are the input S156's promotion gate
counts. Losing them does not corrupt the record; it **quietly weakens the learning
signal**, which is harder to notice than a broken one.

Fixed by merging the three fields an upgrade owns — `Tag`, `UpgradedAt`,
`Address` — onto the fresh record. An allow-list, not a blanket copy: anything an
upgrade does not own belongs to whoever wrote it last.

`live observe` already did this correctly (it mutates the fresh copy). The pattern
existed; the new path reimplemented half of it. Again.

## Eight passes, and what they are actually saying

| passes | question | outcome |
|---|---|---|
| 50–52 | did anything reach the cloud? | four places |
| 53–55 | which project do we trust? | three passes to reach the ADR's stated answer |
| 56–57 | how do you write a record you read a while ago? | fixed, then fixed properly |

Fourteen findings, eleven of them one incomplete answer. **This is the strongest
evidence yet for the S156 split**: the problem is not review depth, it is that a
command with six interacting invariants gives every fix five other places to be
wrong. S154, S170 and S155a — one question each — converged in three passes.

## Nothing declined this pass.
