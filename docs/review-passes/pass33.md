# S166 design review pass 33 — three findings, one root cause, all acted on

Three separate findings, all the same mistake: the cutover decision was recorded
in some places and not in the ones that framed it.

- **`s166-teardown-guard-design.md`** — "out of scope: removing
  `scaleway_account_project`, that is S167" directly contradicted the decision
  that they are one slice. An implementer following it would have built a guard
  with no input.
- **`run-owned-project-plan.md`** — the whole "Migration, and why both paths
  coexist" section, plus two risk rows describing a gap that no longer exists.
- **`NEXT_SESSION.md`** — the S165 caveat and the blocker section both still
  ordered S166 before S167.

Fixed by grepping for every surviving mention of the old ordering rather than
patching the three that were reported — the previous two passes on this file were
each one-at-a-time fixes for the same class, which is how it took three passes.

ADR-0025's stale parts were **superseded, not rewritten**: an ADR records what was
decided when, so the original Migration paragraph and the pass-24 ordering
amendment stay, with a new amendment saying explicitly that it supersedes both.
