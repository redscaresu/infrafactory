# Codex review pass 127 — S162c

One finding, **P1**, and the **sixth** instance of one class in five passes.

## [P1] Route loads can resolve out of order

`loadDetail` had no ordering guard at all, so a response could overwrite the page
after the reader had moved on. And the guard I *had* written elsewhere compared
`scenarioPath` — which A → B → A restores, so a genuinely stale response passes it.

## So the state that permits it is gone

A monotonic `navigation` counter, incremented in `afterNavigate`. Every async
setter on the page captures it and discards its own response if it changed:
`loadDetail`, `loadRunMode`, `loadLayer3Status`, and the deploy preview.

A counter cannot collide the way a path can, and a setter added later has an
obvious thing to use.

## Six findings, and the shape of my mistake

1. the confirmation described A and deployed B — P1
2. a stale confirmation survived navigation
3. an in-flight preview re-opened one on the next page
4. an in-flight deploy reported its result onto the next page
5. the page itself lagged behind its own URL
6. route loads resolving out of order — P1

Findings 2 through 5 each got a guard around the exact sequence that had been
found. Pass 126's own note says, in as many words, that *"when the third instance
of a class arrives, the next move is to remove the state that permits it, not to
guard the path that reached it"* — and I then wrote finding 5's fix as another
guard.

The generalisation was correct, written down, and not acted on. That is a
different failure from not seeing the pattern, and a worse one.

## The test was wrong first

The first version used `page.goto` twice, which does full page loads that abort
each other — so it exercised nothing. The race only exists in **client-side**
routing, where the component is reused rather than rebuilt, so the test navigates
by clicking sidebar links.

Fourth red test this session that was the test's fault. Here it failed for a
reason that had nothing to do with the property, which is the kind that quietly
becomes a deleted test.
