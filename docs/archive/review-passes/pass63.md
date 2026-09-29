# Codex review pass 63 — S156a

One finding, accepted. **The third consecutive pass to land on the same merge.**

## [P2] The refresh crossed source boundaries

`entryKey` is `(resource, rule)` and deliberately ignores `source`, so the same
rule can exist as `descriptive` in pre and `live` in post. Pass 60's refresh then
copied the live timestamp onto the **descriptive** entry — attaching a lifetime
to a source that has none, while the live record vanished as a duplicate. Two
errors from one line, and the timestamp would have sat there meaning nothing,
because only live entries are ever retired.

Refresh is now like-with-like. A source mismatch keeps the pre-existing
behaviour: skip it, exactly as this merge did before timestamps existed.

## Passes 59, 60, 63 are one thing

| pass | the merge | what was missed |
|---|---|---|
| 59 | preserve live entries | their freshness |
| 60 | carry a newer timestamp | that the key ignores source |
| 63 | refresh like with like | — |

Each fix was correct and each was too narrow, on **fifteen lines of code**. The
lesson S156a has actually taught is not about slice size — this slice is small.
It is that a change to a data model lands in every place that reads it, and the
reliable way to find those is to enumerate the readers *before* the first edit
rather than after each review.

## Nothing declined this pass.
