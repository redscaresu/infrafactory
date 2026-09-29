# Codex review pass 40 — S166+S167 cutover

`codex exec review --base main` on `s166-cutover` at `43144b3`.

Two findings, both accepted. Both are about `deploy`, which was written before
the cutover gave it a project of its own to lose.

## [P1] The project was created outside the interrupt guard

`runDeployApply` wraps the apply in a `signal.NotifyContext`, and
`ensureRunProject` ran **before** it. The project is real from the moment the
Account API returns, and the deployment record is written only after the apply —
so a Ctrl-C in between left a real project with no record and nothing coming for
it. That is the exact leak the record exists to prevent, in the window just
before the record is written.

Fixed by moving project creation and the environment build inside the guarded
closure. The interrupt message now names what it is recording and points at
`live teardown`, which owns the record; it previously said "during apply", which
is no longer the only thing the guard covers.

## [P2] The failure that names the leaked project was thrown away

`ensureRunProject` handles a marker-write failure by deleting the project again
and, if that also fails, reporting **the project id and how to remove it by
hand**. Deploy's early return discarded those staged failures and reported a
generic *"could not create the deployment's project, so nothing was applied"* —
on the single path where that id is the operator's only handle.

Fixed: the staged stages and failures are written to the command output before
returning. The generic sentence stays as the *error*, where it is accurate.

Accepted as more than P2 for the reason this project keeps rediscovering: a
guard that stops without saying why is half a guard.

## Coverage

- `TestDeployCreatesTheProjectInsideTheInterruptGuard` — whatever creates the
  project sees the signal-derived context.
- `TestDeploySurfacesWhyTheRunProjectFailedRatherThanAGenericMessage` — the
  reason reaches the operator, and no deployment is recorded when there is no
  project.

## Nothing declined this pass.
