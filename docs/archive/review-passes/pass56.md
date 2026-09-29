# Codex review pass 56 — S155b `live upgrade`

Two findings, both accepted. Neither is a nit: one is a **false confirmation**,
which is the exact failure this slice exists to detect.

## [P2] A prefix match confirmed the wrong version

`strings.Contains(body, tag)` confirms tag `1.2` against a service reporting
`nginx/1.27.4`. That is a service running something other than what the record
claims — the precise drift S155a was built to catch — reported as confirmation.

`mentionsVersion` requires version boundaries: a match may not be flanked by
digits, and may not directly follow a dot. So `1.27` is confirmed by `1.27.4` (a
patch of the same version), `1.2` is not, and `1.27` is not confirmed by `11.27`.

## [P2] The upgrade could overwrite a concurrent teardown

The same lost-update race fixed in `live observe` earlier, on a path I did not
carry it to — and worse here, because an upgrade holds its record across a **real
apply**: minutes, not the microseconds a probe takes. A `live teardown` finishing
in that window would be overwritten and the deployment resurrected.

Re-read before writing, and a record released mid-apply now produces a loud
failure naming the project, because whatever the apply created is **not** tracked
by a released record. The window is narrowed, not closed — the store has no
compare-and-swap, and the comment says so rather than implying otherwise.

## Seven passes

This is the third time in this slice that a fix already written elsewhere was not
carried to a new path: the marker guard (pass 53), the `applyRan` predicate
(passes 50–52), and now the re-read-before-write race. Each was solved correctly
somewhere in the codebase before this slice began.

The lesson is not "review harder". It is that **new code touching an existing
mechanism should start by reading how that mechanism is already used**, not by
reimplementing the parts of it that seem needed.

## Nothing declined this pass.
