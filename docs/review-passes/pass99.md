# Codex review pass 99 — S159a

One finding, accepted. **It is the slice's own bug, one field over.**

## [P2] A never-observed deployment reported being observed in the year 1

`omitempty` does not omit a zero `time.Time`. It is a struct, so the tag has no
effect and it marshals as `0001-01-01T00:00:00Z`. `GET /api/deployments` therefore
gave every never-probed deployment an apparent last-observed timestamp.

Worse than a blank cell, because it looks like data. A page would render a date
next to `unobserved`, and a reader trusts a date.

This slice exists *because* the zero value lies in a view: `VersionUnchecked` is
the empty string and would have rendered as a blank cell, so `HealthSummary.Version`
became a string that always says the word. I wrote that comment, then left the
field directly beneath it with the same defect in a different disguise — a struct
whose zero value serialises rather than disappearing.

`At` is a `*time.Time` now, nil when nobody looked.

## The generalisation

"The zero value is a lie in a view" is not a fact about one field. It is a
property of every optional field in a payload a human reads, and each type
expresses it differently: `""` for a string, `0001-01-01` for a time, `0` for a
count, `false` for a flag. Checking the field I was thinking about and not its
neighbours is what made a one-finding pass out of a slice whose whole subject was
that defect.
