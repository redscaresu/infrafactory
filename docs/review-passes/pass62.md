# Codex review pass 62 — S156a

One finding, accepted. **A guard that already exists in this codebase, not
carried across.**

## [P2] The cloud name was joined into a path unvalidated

`pitfalls retire <cloud>` takes its argument from the command line and joins it
straight onto the pitfalls directory. So `retire ../../something` reads **and
rewrites** a YAML file outside the corpus — a write, not merely a read.

`livestore.validateID` guards deployment ids against exactly this, for exactly
this reason: a name that came from a caller decides a path. `assertCloudName` now
applies the same rule, on all three entry points — retire, its dry-run, and
touch.

## The count for this slice

Four passes, four findings, and **three of them were mechanisms that already
existed somewhere in this repository**: the source ratchets, the sweep's
preservation rule, and now the path guard. The fourth was the runtime wiring.

That is the same shape as S155b, and it is worth naming precisely because S156a
is a *small* slice — so slice size is not the whole story. The other half is:
**new code that touches an existing mechanism should start by finding every place
that mechanism is already enforced.** Adding `LiveSource` was three lines; the
work was the four places that had to learn about it.

## Nothing declined this pass.
