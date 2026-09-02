# Codex review pass 128 — S162c

One finding, **P3**, accepted. Seventh instance of the class, and the first with a
consequence small enough to argue about.

## [P3] Two clicks could reopen a dismissed dialog

Repeated clicks on Deploy left multiple previews in flight for the same
navigation. The token guard only checks the *route*, so an older response could
still set `confirmingDeploy = true` after the reader had cancelled.

No wrong infrastructure — the action follows the preview, and both previews were
for the same scenario. A dialog reappearing after you dismissed it, which is
annoying rather than dangerous.

Accepted anyway, on two grounds. The fix is four lines: a `previewing` flag that
disables the button, so **only one request can be in flight** — removing the state
rather than guarding it, which is what the last two passes were about. And it
gives the reader feedback that the click landed, which the button did not
previously do for the round trip.

## Where this slice stands

Seven findings, two of them P1. Every one is the same sentence: *asynchrony lets a
UI separate a statement from its subject.* The consequences ran from "creates the
wrong infrastructure" down to "a dialog reappears", and they were found in roughly
that order — which is luck rather than method, since nothing about the code made
the severe ones easier to find first.
