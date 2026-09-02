# Codex review pass 115 — S162a

One finding, accepted, and the fix is the fourth time this session a defect has
been closed by construction rather than by correction.

## [P2] `resources.iam` was missing from the preview entirely

The estimator enumerated eleven resource blocks by hand and missed `iam` — which
is, of the twelve, the one whose absence matters most. An IAM application, its
API key and its policy are exactly what a **blast-radius** preview exists to
surface, and they are free, so no cost total would ever have hinted they were
missing.

## Adding it back would have been another snapshot

The list was hand-written, so the next resource type added to the schema would
vanish from the confirmation the same way — silently, and specifically from the
screen somebody reads before spending money.

`TestEveryDeclaredResourceBlockAppearsInThePreview` walks `Resources` by
reflection and asserts every block contributes at least one named component. It
**failed on IAM before the fix**, which is the point: the test found the bug the
review had just described, and would have found it a slice earlier.

Two things fell out of writing it. A `networking` block with only a VPC or a
private network produced nothing, so those are named now — a component that costs
nothing still belongs in "what will this make", because this list is a
blast-radius preview before it is a bill. And the assertion that every block is a
pointer is itself a guard: a resource type added as a value would slip past the
reflection walk.

## The pattern, stated once

Four class-closures this session: the year-one timestamps, the empty-estate
claim, the observed-key identity, and now the resource enumeration. Every one
followed the same sequence — a finding, a careful fix, the same defect again
somewhere adjacent, and only then a test that a careful fix cannot satisfy.

The lesson is not "write class tests". It is that **the second occurrence is the
signal**, and treating it as one more instance rather than as evidence about the
shape of the mistake is what costs the third.
