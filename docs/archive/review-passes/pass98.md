# Codex review pass 98 — S156d

**Clean.** No discrete correctness issue. It did go and check that a NUL byte
survives a YAML round trip, which is what the observed-key separator relies on —
it does.

## Seven passes, ten accepted findings, one declined

That is the most any slice in this project has needed, and the count is the
finding.

Not one was a coding slip. Every one was **a rule the surrounding system already
enforced that this new path did not inherit**:

| pass | the rule that was not inherited |
|---|---|
| 91 | attribution comes from the failure, and `AddressResource` is not it |
| 91 | a failure must still be present at the moment of the fix |
| 92 | a deletion is a change (the ambiguity gate) |
| 92 | a partial record skips, it does not fail the command |
| 93 | both prescriptive shapes exist, not only additions |
| 94 | an absent input is not an empty input |
| 95 | the tool's own disruption is not a signal about the service |
| 95 | evidence is what was observed, not what was available |
| 96 | an operation having run is not an operation having worked |
| 97 | identity must be as wide as the thing it identifies |

Several of those are stated, in those words, elsewhere in this codebase. None of
them are stated anywhere a new path can see.

## The slice was too big, and the evidence was there beforehand

S156 was split into five slices *because* S155b took seven passes. The split was
by dependency, not by question, and S156d had at least four questions in it: when
is an upgrade a repair, what does the diff attribute to, which observations count,
and what counts as an upgrade at all.

The plan entry names its one question as *"is the before/after pair a usable
diff?"* — and the pair was never the hard part. **The question a slice is planned
around should be the one that turns out to be hard, and being wrong about which
one that is looks exactly like this.**
