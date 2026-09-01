# Codex review pass 73 — S156b promotion gate

One finding, accepted, and **it was the most important one of the slice.**

## [P1] The gate excluded the signal the arc exists for

`PromotionCandidates` skipped anything `Healthy()`. A service that answers
perfectly while running the wrong version records `Status: healthy`, so **version
drift could never be promoted** — the exact failure class S155a was built to
detect, and the one the S155b canary produced from an apply that *succeeded*.

Worse, checking it turned up something the finding did not say: the mismatch was
never on the record at all. `live observe` put the version detail in the
`FailureSummary` and left `observation.Detail` empty, so the reason vanished when
the command exited. Nothing downstream — not the gate, not a human reading the
record — could see why.

Two fixes:

- **The record carries it.** An unconfirmed version writes its detail onto the
  observation, unless a health failure already claimed the field: a service that
  is both down and misreporting should say the more urgent thing first.
- **The gate treats it as adverse.** `adverse(o)` is `!Healthy() || versionDrift(o)`,
  and `unchecked` is still not adverse — nobody looked, which is not evidence.

## The key was too coarse, and the tests said so

The first fix put the raw `VersionCheck` in the grouping key, which broke
`TestAttributionIsRecordedRatherThanFiltered`: two identical health failures
stopped grouping because one happened to have its version confirmed. That is
incidental, not a distinction.

Keyed on a `drift bool` instead. And an observation that is **both** unhealthy and
version-unconfirmed is *not* drift: its detail describes the health failure, which
is the more urgent story, so it groups with other instances of that failure rather
than splitting off on a version field.

## The report names it

`version drift (service healthy)`, not `healthy`. On its own, "healthy" reads as
the opposite of a finding — and this is precisely the shape every other signal in
the system already reports as fine.

## Nothing declined this pass.
