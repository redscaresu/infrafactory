# Review passes 180–182 — the guard that would not say why

`codex exec review --base main` on `fix/poweroff-says-why-it-skipped`. Three
passes, four findings, all accepted; the third pass clean.

> I did not identify any discrete correctness, safety, or maintainability issues
> introduced by this diff.

## What prompted the branch

Run `20260910T141427Z` — the first on the S181 binary — failed its destroy with
**no `instance_poweroff` stage anywhere in the log**. The implementation had
three silent `return nil` paths, so a real apply against real Scaleway bought no
information: "no project id", "guard refused" and "found nothing to stop" were
indistinguishable.

That is the Layer 3 arc's closing finding — *a guard that stops without saying
why is half a guard* — reproduced inside the change whose own ADR quotes it.

## Findings

**[P2] the purge did not get the recovery.** The marker fallback was added
inside `powerOffRunInstances`, so `destroySandbox` still passed the original
empty id to the purge. Since both remediations are scoped by that one id, fixing
only the poweroff would have fixed half the outage — and the missing purge stage
in that run is the evidence both were disabled together. Resolution hoisted into
`destroySandbox`.

**[P2] a committed pitfall still prescribed the standalone NIC.** Not the Go
fallback this branch fixed — an entry the loop had *learned and written to
`pitfalls/scaleway.yaml`* during that same run, from the old fallback. Deleted,
with a comment kept in its place because the mechanism is worth remembering: the
loop recorded a durable rule contradicting its own enforcement while its Layer 1
was telling the generator the opposite.

**Doc hygiene** required an ADR touch; ADR-0030 amended rather than a new record
written, since this refines the same decision.

## A boundary that moved, deliberately

`TestDestroyWithoutProjectIDDoesNotPurge` asserted "without a project id, do not
purge" as a safety property. It was not one. Every caller derives that id from
`CaptureSweepTarget`, so an empty value means *the capture failed*, not *the
caller declined to act* — and treating it as a refusal turned a recoverable
teardown into a leak. The check that makes purging safe is
`AssertProjectDeletable`, and it is untouched. The test is now
`TestDestroyWithoutProjectIDOrMarkerDoesNotPurge`, with a companion asserting the
recovery.

## Mutation checks

| mutation | result |
|---|---|
| marker fallback removed | `TestPowerOffFallsBackToTheRunProjectMarker` fails |
| VPC fallback restored to the standalone-NIC wording | `TestVPCFallbackPrescribesTheShapeTheGateAccepts` fails |

## The thing worth keeping

Two of today's defects were the same shape: **a prescriptive rule frozen in
source, drifting from the code that enforces it.** The Go VPC fallback said
"declare a `scaleway_instance_private_nic`" while the gate refused that resource,
the policy argued against it, and a real teardown could not delete one.

Refusals extracted by `ExtractGatePitfall` cannot fail this way — they are
emitted by the enforcing code itself, so they cannot disagree with it. That is
the argument for deriving pitfalls from gates rather than writing them, and it
now has an audit test behind it.
