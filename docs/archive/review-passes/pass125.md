# Codex review pass 125 — S162c

One finding, accepted. **Fourth instance of one class in three passes.**

## [P2] A deploy result could appear on a different scenario's page

An apply takes minutes. Navigate during it and the outcome message lands on
whatever page is showing — an unattributed *"Deployed."* that is a claim about the
wrong thing.

**Attributed rather than discarded.** The deploy really did create
infrastructure, and throwing the news away because the reader moved is the worse
of the two failures. Every outcome now names its scenario, success and failure
alike, so the message is true wherever it appears.

## The class, and why it took four passes

Every one of these is *"asynchrony lets a UI separate a statement from its
subject"*:

1. the confirmation described A and deployed B (P1)
2. a stale confirmation stayed visible after navigation
3. an in-flight preview re-opened one on the page you moved to
4. an in-flight deploy reported its result on the page you moved to

Passes 123 and 124 each fixed the instance in front of them and I did not go
looking for the others — after having written, in pass 124's own note, that this
risk belongs to *"every dialog in this project that is populated by a fetch"*.
Writing the generalisation is not the same as applying it, and the gap between
those two is where finding 4 lived.

The durable rule: **anything a page says about a long-running action must name its
subject**, because the page's own context can change before the action finishes.
That is not a UI style preference. On this page it is the difference between "you
created infrastructure" and "something created infrastructure, possibly this".
