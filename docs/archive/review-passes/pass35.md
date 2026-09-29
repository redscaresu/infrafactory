# Codex review pass 35 — S166+S167 cutover

`codex exec review --base main` on `s166-cutover` at `d1afa4a`.

One finding, accepted — with the fix deliberately shaped **not** to reintroduce
a second model.

## [P1] Gating `reclaimable` on the marker made pre-cutover records forgettable

Pass 31 changed `reclaimable()` from "state on disk" to "marker on disk", which
fixed the post-cutover shape (project, no state) and broke its mirror image: a
workdir written **before** the cutover has state and no marker. Those records
became unreclaimable, and unreclaimable is what routes the operator to
`infrafactory live forget` — which retires the record while the resources keep
running.

`live forget` is the one command here that turns a leak into a clean slate, so
"unreclaimable" is the answer to be stingy with.

## The fix, and what it deliberately is not

Not a compatibility path. The cutover decision stands: one model, no transition.
What changed is that **nothing is deleted on no evidence, and nothing is
forgettable on no evidence either.**

- `reclaimable()` is now marker **or** state. Either one means teardown still
  owns the record.
- `releaseRunProject` **skips** when there is no marker at all, reporting a skip
  rather than a failure: no marker means nothing here created that project
  through the Account API, so there is nothing for it to delete. The orphan
  sweep asks the API, which is the only answer worth having.
- A marker that names a *different* project is unchanged — still a refusal. The
  argument must not win over the record.

A pre-cutover record therefore destroys through tofu, skips the Account delete,
and fails the sweep loudly because without a marker the blast radius is unknown.
Kept, visible, and not forgettable — which is the property that was lost.

## Removed: the guard on the destroy itself

`live teardown` called `assertRunProjectDeletable` before `destroySandbox`. That
was redundant and, as of this pass, harmful: `tofu destroy` acts on the state in
its own workdir, so its blast radius is bounded by the state and not by a
project id, while both things that DO reach the API by project — the purge and
the Account delete — carry the guard themselves. Its only remaining effect was
to stop a pre-cutover record being destroyed at all.

`TestTeardownRefusesWhenTheRecordDisagreesWithTheMarker` now pins the property
that actually matters: a tampered `ProjectID` cannot aim the delete, and cannot
aim the sweep either — both take the marker's id.

## Nothing declined this pass.
