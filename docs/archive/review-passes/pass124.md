# Codex review pass 124 — S162c

One finding, accepted. It is the third fix to the same property, and each
previous one was necessary and insufficient.

## [P2] A slow preview could open a confirmation on the page you moved to

`afterNavigate` clears the dialog when the route changes. A preview takes a round
trip, so a response arriving *after* that sets `preview` and `confirmingDeploy`
again — re-opening a confirmation for a scenario the reader is no longer looking
at.

The request now captures the path it belongs to and discards its own response if
that changed.

## Three fixes, one property

| pass | fix | what it did not cover |
|---|---|---|
| 123 | deploy `preview.scenario`, not `detail.name` | the dialog could still be *shown* for the wrong page |
| 123 | clear the dialog on navigation | a response in flight re-opens it |
| 124 | discard responses that outlive their page | — |

The property is **"the confirmation on screen describes the thing this page is
about, and accepting it does that thing"**, and it needed all three. Two of them
look like the same fix at a glance, which is presumably why I wrote one and
stopped.

Worth naming the general shape: **asynchrony gives a UI a way to separate a
description from its subject that a command line does not have.** `infrafactory
deploy <scenario>` names its target in the same sentence as the decision, and
nothing can arrive later to change it. Every dialog in this project that is
populated by a fetch inherits this risk, and the deploy one inherits it while
holding the ability to spend money.
