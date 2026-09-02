# Codex review pass 121 — S162b

One finding, accepted.

## [P2] A missing scenario name answered 500

`writeActionResult` maps every non-nil error to 500, and `resolveScenarioByName`
returned a plain `fmt.Errorf`. So a client typo — or a UI holding a stale scenario
list — read as a server fault.

The teardown handler already distinguishes these, mapping `os.ErrNotExist` to 404.
The deploy path did not, so the same class of caller mistake got two different
answers depending on which endpoint they hit.

The cost is not the status code. It is that **500 stops meaning anything** when
user input can produce it: an operator who has seen "500" for a typo has no reason
to treat the next one as an infrastructure problem.

Both sides are pinned now — a missing scenario is 404, and a runtime that cannot
be built is still 500 — because the distinction only means something if both
halves of it exist.
