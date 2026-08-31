# Codex review pass 31 — S166+S167 cutover

`codex exec review --base main` on `s166-cutover` at `ff72968`.

Three findings, all accepted. All three are the same defect class this arc
keeps producing: **the condition was right, the place it ran was wrong.**

## [P1] `CaptureSweepTarget` swallowed state-read failures

`internal/harness/orphan_sweep.go`. The marker rewrite treated *any* error
from `loadLiveTerraformState` as "nothing was applied", so a truncated or
unreadable state file produced a sweep target with **no strays computed** and
the sweep reported clean. That is the false green the sweep exists to prevent,
and it contradicts the rule the same file states about an unreachable API.

Fixed: only `os.ErrNotExist` means "nothing applied". Anything else fails with
`ErrOrphanSweepFailed`. Covered by
`TestCaptureSweepTargetRefusesAnUnreadableState`.

## [P1] `reap` never deleted the project it then verified was gone

`internal/cli/reap_command.go`. The cutover commit moved `releaseRunProject`
ahead of the sweep in `test` and `live teardown` and **missed `reap`**. Since
ADR-0025 `tofu destroy` cannot delete the project, so reap destroyed the
resources, deleted nothing, and then asked the API whether the project was
gone. It was not. Every clean reap would have reported a leak.

Worth noting: this is the *third* time in this arc that a correct condition
was placed on an incomplete set of paths.

Fixed: `releaseRunProject` before `appendOrphanSweepResult`, guarded by
`destroyErr == nil` as elsewhere. Covered by an assertion added to
`TestReapDestroysThenVerifies`.

## [P2] Marker-only deployments were reachable by neither command

`internal/cli/deploy_command.go` / `live_teardown.go`. Registering a
deployment that has a project but no state — the ordinary shape of an apply
that failed at preflight, init or plan — created a record that `live teardown`
refused (no state file) and `reclaimable()` also refused, so the only route was
`live forget`, which retires the record while the project keeps existing.
Registering it was supposed to make the leak reclaimable; it made it visible
and nothing more.

Accepted as more than P2, because it is reachable on any failed deploy.

Fixed:
- the no-state early bail is gone; that case falls through to the
  no-resources path, which now runs the teardown guard, deletes the project,
  sweeps and releases.
- `reclaimable()` gates on the **marker**, not the state file — the marker is
  what the guard reads, so it is the real precondition.
- The empty-state path also deletes the project now. It previously assumed
  `tofu destroy` had already removed it, which stopped being true at the
  cutover.

Covered by `TestTeardownDeletesTheProjectWhenNoStateWasEverWritten`,
`TestTeardownRefusesWhenNeitherStateNorMarkerExists` (fail-closed without a
marker), and an added assertion in the already-destroyed test.

## Nothing declined this pass.
