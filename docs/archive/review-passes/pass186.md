# Review pass 186 — released deployments need no attention

`codex exec review --base main` on `fix/released-deployments-need-no-attention`.
Two passes, one finding, accepted; second pass clean.

## The defect

The Deployments page announced **"10 deployments, 10 needing attention"** and
offered a **Tear down** button on each — while `live reconcile` reported the
cloud and the store in agreement, and the account held zero servers.

All ten records were `state: "released"`. The domain already has the right
predicate:

```go
func (d Deployment) Reapable(now time.Time) bool {
	if d.State == StateReleased { return false }
	return d.Expired(now)
}
```

`!released && expired`. The UI had only the second half. The payload has carried
`state` all along; nothing read it.

This is the inverse of the false green this project keeps removing, and just as
bad on the most-looked-at screen: **"already gone" and "needs attention" must
not look alike**, for the same reason "we could not check" and "nothing leaked"
must not.

## The finding

**[P2] the confirmation branch outranked the released branch.** The released
check sat *after* `confirming === d.id`, and the table refreshes every 30s — so
a row torn down by another tab or by the CLI while its confirmation was open
would keep offering **Destroy** on infrastructure that no longer existed.
Reordered.

## Mutation check

Removing the `released` guard fails two tests: the attention predicate and the
estate summary count.

## Found by looking, not by testing

Nothing failed. The page was reported from a screenshot, and the discrepancy
only surfaced because `live reconcile` was run alongside and disagreed.

Worth stating why no test caught it: **the UI unit suite and the Playwright
suite do not run in CI.** `ci.yml` runs `go test -race -count=2 ./...` and
nothing else; the browser tests are guarded only by a local pre-commit hook.
Addressed separately — it is the more valuable fix of the two.
