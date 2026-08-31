# Codex review pass 47 — S155a verify the running version

Two findings, both accepted. Both are the **stated doctrine of this slice,
violated by the slice itself.**

## [P2] A mismatch could be called on a partial body

`ServiceProbe` read a bounded body and discarded the read error. So a truncated
or failed read produced a partial body, and `strings.Contains` on it reported
**unconfirmed** for a service that would have confirmed.

That is precisely the error the slice was written against, inverted. The ADR
says a probe that fails is `unchecked`, never `unconfirmed`, because *claiming a
contradiction on evidence nobody gathered* is as wrong as treating unchecked as
confirmation. A partial body is evidence nobody fully gathered.

Fixed with `ServiceProbeResult.BodyComplete`, and the asymmetry made explicit:

- **finding** the tag proves it is there, whatever was cut off → `confirmed`
- **not finding** it in a partial body proves nothing → `unchecked`

The probe reads one byte past the limit so hitting it is distinguishable from a
body that merely ends there.

## [P2] "no version_path declared" was a lie for a declared path

A healthy deployment whose declared version path was unreachable printed
`running version unchecked (no version_path declared)` — misstating the case,
and hiding exactly the distinction this slice adds.

`checkRunningVersion` now returns a reason with every `unchecked`, and the pass
line prints it: `/version is unreachable`, `the record names no tag to check
against`, `its response was truncated or unreadable`.

## What the test fake caught

The existing `versionProbe` did not set `BodyComplete`, so the mismatch test
failed the moment the field existed. That is the fix working: a fake must now
state whether its evidence is complete, which makes a test that asserts a
contradiction declare the evidence it is asserting from.

## Re-reading against the class, per the one-pass rule

Class: *concluding more than the evidence supports*. The other conclusions in
this path were checked — `Healthy` is set only from an actual 2xx, `Reachable`
only from an actual response, and the `unchecked`-on-probe-error path already
refused to conclude. The body was the one place a conclusion outran its input.

## Nothing declined this pass.
