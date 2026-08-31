# Codex review pass 37 — S166+S167 cutover

`codex exec review --base main` on `s166-cutover` at `cb69e93`.

Three findings, all accepted, all P1 — and all three are **defects passes 33–35
introduced**, not pre-existing ones. Fixing the provider-default asymmetry on
the happy path left three error paths quietly falling back to the shared
project.

## The shared shape

`sandboxCommandEnvForProject(runtime, "")` is a valid call, and every one of
these reached it by accident: an empty string arrived from a zero value, a
failed capture, or a record field. Pass 34's audit catches a *literal* `""` and
none of these are literal.

## [P1] Teardown built its destroy env from the record, not the marker

`live_teardown.go`. `d.ProjectID` is the half a stale or hand-edited record can
change, and the half the guard refuses to trust when the two disagree — so it
should not be what scopes the destroy. Now the marker where there is one, the
record only as the pre-cutover fallback. Both the destroy path and the
no-resources path.

## [P1] `run`'s auto-destroy took the project from the sweep target

Pass 34 sourced the env from `sweepTargetProjectID(sweepTarget)`, which is `""`
when capture fails. `CaptureSweepTarget` answers **two** questions — which
project is ours, and what strays the state names — so an unreadable *state* file
took the *project id* down with it and left the destroy pointed at the shared
fallback.

Separated: the project comes from the marker directly, the strays from the
capture. If the marker cannot be read, the auto-destroy does not run — and says
so, with the reason and the recovery command, because a guard that stops without
saying why is half a guard.

## [P1] The interrupt guard used a zero-value marker

Pass 33 read the marker, and on `markerErr != nil` with state present it carried
on with the zero value — building the env from an empty project id. Now that
shape reports abandoned resources with the recovery command instead. A
post-cutover run always writes the marker (failing to is fatal at creation), so
its absence means the workdir is damaged.

## A test fixture that could not occur

`TestRunCommandAutoDestroys...` seeded state with no marker to simulate "a prior
sandbox deploy left state behind". Real prior deploys leave **both** —
`ensureRunProject` writes the marker into the output dir before the apply. The
fixture now seeds both, and a separate test covers the marker-less shape as the
refusal it now is.

New coverage: `TestInterruptGuardRefusesToDestroyWithoutAMarker`,
`TestRunCommandRefusesAutoDestroyWhenTheRunProjectIsUnknown`.

## And then the class was closed at the seam

Three passes have now fixed instances of "an empty project id reached the env
builder". The audit cannot catch them because none is a literal.

`sandboxCommandEnvForProject` now **returns an error** on an empty project id.
The builder that tolerates one is `sandboxEnvWithProjectDefault`, reachable from
the credentials preflight — which discards the environment it builds. So an
accidental empty is a loud failure rather than a destroy quietly scoped to the
shared fallback, or, with no fallback configured, to whatever `~/.config/scw`
names — typically the organization default.

The audit stays, repointed at the raw builder and re-verified against synthetic
drift in two separate files.

## Nothing declined this pass.
