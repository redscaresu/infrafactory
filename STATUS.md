# STATUS

Current state, and the one place to start. It changes only when **Now** does, and stays under
150 lines (CI-enforced).

## Now

No arc in flight. The queue is the `ready` stories in `docs/stories/`; several can run at once
as a herdr wave (`docs/operations.md` § Parallel agents). Multi-slice arcs get a plan in
`docs/plans/<arc>-plan.md`.

## Open work

One file per item in `docs/stories/`, with `status: ready | blocked | later`. The PR that
finishes an item deletes its file.

    grep -H '^status:' docs/stories/*.md

## Recent

Not stored here: the squash-commit titles are the record, so this cannot go stale or conflict.

    git log --first-parent -10 --format='%ad %s' --date=short main

History: `docs/status/ARCHIVE.md` (per-arc close-outs) and `docs/status/STATUS_HISTORY.md`.
