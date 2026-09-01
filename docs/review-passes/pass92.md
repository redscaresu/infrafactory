# Codex review pass 92 — S156d

Two findings, both accepted, and both are the *same shape as pass 91's* — a rule
stated in a comment that the code did not actually enforce.

## [P2] A deletion is a change, and was not being counted

`ChangedResourceAddresses` walked only the passing side, so a resource **removed**
between the two configurations did not register. An upgrade that deleted one
resource and modified another therefore reported "exactly one changed" and
sailed through the ambiguity gate the previous pass had just installed.

Worse than a miscount: the deletion may be what cleared the failure.
Deletion-as-fix is a real shape — it is why ADR-0019 has `avoid` entries at all —
so the remedy would have been attributed to whichever resource happened to
survive.

## [P2] One partial record could fail the whole command

The descriptive path partitions by cloud and reports records that name none. The
repair path iterated every deployment and would reach `AppendLivePitfall` with an
empty cloud, where `assertCloudName` fails — taking the entire `live learn`
command down over one incomplete record. `Cloud` is optional on the schema, so
these exist.

It skips and reports now, the same way the path beside it already did.

## The pattern across passes 91 and 92

Four findings in this slice, and every one is a case where **a second code path
did not inherit a rule the first one had already established**: the ambiguity
gate, the per-cloud handling, the failure-must-persist rule, the attribution
source. The S156a lesson is that a change lands in every place that reads it —
this is the same lesson from the other direction, where adding a path means
inheriting every rule the existing paths enforce.
