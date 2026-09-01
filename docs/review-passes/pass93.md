# Codex review pass 93 — S156d

One finding: *"the repair path misses deletion-as-fix upgrades even though the
project already has an extractor for that shape."*

**Accepted in substance, and the answer is more interesting than the fix.**

`ExtractAvoidPitfall` is now tried when `ExtractFixPitfall` finds nothing, which
is eight lines and cheaper than arguing about scope. But it will rarely fire on a
live signal, and the reason is structural rather than incidental.

That extractor attributes **strictly**: the removed attribute's name must appear
in the failure detail, a rule added after a false positive on `aws_subnet` in the
S63 sweep. A provider error names the offending attribute. A health probe
reporting `returned HTTP 503` names nothing at all — which is the same
attribution gap pass 91 found, appearing again one layer down.

Loosening that attribution to suit live signals would weaken a guard the **run
loop** depends on, in service of a different caller. **Declined**, and the limit
is pinned by `TestLearnCannotAttributeARemovalTheProbeDidNotName` rather than
left to be rediscovered.

The finding did surface a real defect alongside it: the skip message said "no
single attributable change was found" even when exactly one change *was* found
and simply could not be turned into a rule. Two different situations calling for
two different responses, reported as one. They now read differently.
