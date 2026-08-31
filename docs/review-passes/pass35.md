# S166 design review pass 35 — clean

`codex exec review --base main`:

> The patch is documentation-only and consistently updates the handoff, status,
> ADR amendment, and plan documents to reflect the S166/S167 cutover decision. I
> did not identify a discrete correctness issue introduced by these changes.

The S166 design and its four decisions are reviewed. **The implementation is not
written**, and the cutover it now describes — guard, shape gate, prompts,
pitfalls, fixtures and recorded generation in one change — is the next slice.

## Passes 30–35, in one line

Six passes on a documentation-only change, and every finding was the same shape:
**a document describing a state that had moved.** The decision was recorded in
one place and the sentences that framed it were left standing elsewhere — four
separate times, across the handoff, the plan, the design's own scope section and
the status file.

The cheap lesson, worth more than the individual fixes: when a decision changes,
grep for every sentence that assumed the old one, rather than editing the place
where the new fact belongs. Consolidate same-day entries instead of appending
corrections above them — a correction stacked on a wrong statement leaves both on
the page.
