# Codex review pass 129 — S162c

**Clean.** No actionable correctness issues; the preview flag, the confirmation
flow and the tests are consistent with the API and the safety model.

Eight passes, seven findings, two P1. Every finding was one sentence:
**asynchrony lets a UI separate a statement from its subject.**

A command line cannot do this. `infrafactory deploy <scenario>` names its target
in the same breath as the decision, and nothing arrives later to change it. A page
holds a description, an action, a URL and several in-flight responses, and any of
them can be about a different thing than the others.

The sequence is worth keeping because the *fixes* tell the story better than the
bugs:

| # | finding | fix | kind |
|---|---|---|---|
| 1 | described A, deployed B (P1) | act on `preview.scenario` | bind the action to the statement |
| 2 | stale dialog after navigation | clear on navigate | guard the occurrence |
| 3 | in-flight preview reopened it | compare paths | guard the occurrence |
| 4 | deploy result on the wrong page | name the scenario | make the statement self-describing |
| 5 | page lagged its own URL | clear `detail` | remove the state |
| 6 | loads resolving out of order (P1) | navigation token | remove the state |
| 7 | two clicks, two previews | disable the button | remove the state |

Findings 2 and 3 were guards. 5, 6 and 7 removed the state that permitted the
problem — and each of those closed a *category*, where the guards closed a path.
Pass 126 wrote that lesson down and pass 127 still had to learn it again.

The thing I would keep from this slice is not the code. It is that **writing the
generalisation is not applying it**, and the gap between them is exactly where
findings 5, 6 and 7 lived.
