# Codex review pass 76 — S156c

One finding, accepted. **It defeated the slice's main behaviour.**

## [P2] Fuzzy deduplication silently discarded distinct live lessons

`AppendLivePitfall` routed through `AppendPitfall`, whose `isDuplicate` matches on
**significant word overlap**. That is right for provider diagnostics, which vary
in phrasing between runs and should collapse.

Live rules are the opposite. They are generated from a template, so two genuinely
different failures on one resource share nearly every word:

```
Observed on a RUNNING deployment, after the apply reported success: <detail>. Evidence: <...>
```

So the second lesson for a resource would be dropped as a duplicate of the first,
and the corpus would keep whichever was observed earliest — **silently**, because
my wrapper treated "AppendPitfall deduped it" as success.

Live entries now append on **exact identity**. That is not merely narrower, it is
*sound* here in a way it is not on the fuzzy path: the rule text is derived
deterministically from the candidate, so the same candidate always produces the
same string and a different string means a different candidate.

## The general shape

This is the third time in this arc that reusing an existing mechanism was the
mistake rather than the shortcut — after the marker guard and the re-read pattern.
The rule that keeps emerging: **a mechanism's semantics travel with it.** Fuzzy
dedup is correct for text a machine varies and wrong for text a machine
generates, and reusing it without asking which kind this was is what produced the
defect.

## Nothing declined this pass.
