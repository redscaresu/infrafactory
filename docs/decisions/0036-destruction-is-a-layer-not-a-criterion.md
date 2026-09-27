# ADR-0036: Destruction is a layer, not a criterion

## Status
Accepted — 2026-09-27 (S202)

## Context

`scenario.schema.json` accepted `type: destruction, expect: no_orphans` as an acceptance
criterion, and almost every scenario declared it. Nothing evaluated it. The criteria support
matrix called it supported, and the criteria dispatcher had no case for it, so it was dropped
without a stage or a failure.

The destroy, the mock orphan check and the Layer 3 orphan sweep run for every scenario whenever
the destruction layer is enabled and `--no-destroy` is not given, whether the scenario declares
the criterion or not. So the criterion never added a check. The one thing it did was look like
coverage: on a `--no-destroy` run, or with the layer disabled, a scenario that declared
`no_orphans` still reached `target_reached` with no teardown having run.

Making it evaluable would mean a criterion that repeats a layer's verdict, fails every
`--no-destroy` run (which ADR-0009's incremental workflow needs), or skips, which leaves it
decorative again.

## Decision

`destruction` is not an acceptance criterion. The schema no longer lists it, so a scenario that
declares it is refused at load, and the Go parser refuses the type too. Teardown verification is
the destruction layer (`validation.layers.destruction`), which reports its own stages, and
`--no-destroy` shows up as `destruction/disabled: skip`.

## Consequences

- Every scenario and test fixture that declared it has dropped it. Fixtures that had no other
  criterion now declare a real one, because `acceptance_criteria` still needs at least one.
- An operator's own scenario that declares it stops loading until the criterion is removed.
- ADR-0023's "`destruction: no_orphans`" means the orphan check. It is not a criterion.
