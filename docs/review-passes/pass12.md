# S153a/b review pass 12 — one finding, acted on

### [P2] `forget` rejected invalid records teardown cannot reclaim

The pass-11 fix closed the dead end for records whose sweep had failed, but not
for records that decode while missing what teardown needs. A record with an empty
`project_id` counted as reclaimable purely because a state file existed, so
`forget` refused it while teardown failed at `AssertProjectDeletable` — the same
no-escape loop, one class along.

`reclaimable` now also requires a project id. The general rule, now stated in
ADR-0024: **whenever a guard refuses and names a remedy, the remedy must accept
exactly that case** — and it wants a test that walks the pair, not each half
alone.
