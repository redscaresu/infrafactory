# S153a/b review pass 11 — three findings, all acted on

`codex exec review --base main` after the pass-10 fixes. No nitpicks this time;
all three were real.

### [P1] Sweep-failure marker write was dropped — `live_teardown.go:153`

`_ = store.Put(d)`. If the write failed (read-only store, full disk), the sticky
flag stayed false, so the next pass would take the empty-state path and release
while the unseen strays kept billing — silently undoing the pass-10 fix. A failed
marker write is now itself a teardown failure, and says not to expect a re-run to
refuse.

### [P2] `forget` rejected exactly the record `teardown` refused

A dead end created while closing the previous one: teardown refuses a sticky
empty-state record and tells the operator to use `live forget`, but `reclaimable`
returned true for any existing state file — so forget bounced it back to
teardown. No CLI escape hatch at all without hand-editing files. `reclaimable`
now returns false for that exact combination.

### [P2] No kill fallback for a cancelled command

Dropping `WaitDelay` in pass 10 removed the false-failure risk but also removed
the escape: a `tofu` that ignores SIGINT would hang forever, and on `deploy` that
prevents registration — recreating the unrecorded-live-resource path the signal
guard exists to close. `Cancel` now sends SIGINT and arms a SIGKILL fallback,
scoped to cancellation only, so a normal exit can never trip it.
