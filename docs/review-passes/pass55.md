# Codex review pass 55 — S155b `live upgrade`

One finding, accepted. **ADR-0025's rule, half-implemented for the third time.**

## [P1] Local files decided where a real apply landed

Pass 53 took the project from the marker. Pass 54 made the marker required. Both
were about *which local file to trust* — and the marker and the record are **both
local files**, so together they prove only that two local files agree.

ADR-0025 never said "use the marker". It said **two checks that must both pass**:
the marker for identity, and API provenance for class, *because the second is the
one that cannot be forged locally*. Editing two files was enough to point a real
apply at somebody's production project.

`assertRunProjectOurs` now asks the API before applying. It is deliberately not
the deletion guard: `AssertRunProjectDeletable` treats a project the API reports
**gone** as fine, because gone is the outcome it wants. Applying is the opposite —
a project that does not exist, or exists without the stamp, is one this command
must not touch.

## A wording bug found while fixing it

The first version wrapped `ErrProtectedProject`, which renders as *"refusing to
delete a project this run did not create"* — on a path that applies. A guard whose
message describes the wrong operation sends the reader looking in the wrong place.
Dropped, with the reason recorded so it is not reintroduced for the sentinel's
convenience.

## Six passes, and the same shape every time

| passes | the question | how it went wrong |
|---|---|---|
| 50–52 | did anything reach the cloud? | answered for the tag, then the address, then the workdir, then the ordering |
| 53–55 | which project do we trust? | the marker, then *required*, then finally the API too |

Ten of eleven findings in this slice are one incomplete answer, not eleven
mistakes. **The rule already existed and was written down**; each pass applied a
bit more of it. Reading ADR-0025's own words before writing the guard would have
collapsed three passes into one.

## Nothing declined this pass.
