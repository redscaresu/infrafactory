# Codex review pass 126 — S162c

One finding, accepted. **Fifth instance of the same class, in four passes.**

## [P2] The page could still be showing the previous scenario

`afterNavigate` sets the new path and starts loading, but `detail` kept the
**previous** scenario until that load resolved. In that window the page — title,
badges, Deploy button — still described the old one, at a URL that said otherwise.

Clicking Deploy there previewed and deployed the old scenario. Internally
consistent, since pass 123 made the action follow the preview, and still wrong:
the address bar named something else.

## The structural fix, finally

`detail = null` on navigation. The whole page is inside `{#if detail}`, so nothing
renders — button included — until the new scenario is really loaded. The cost is a
blink of empty page; the gain is that it is not possible to act on a scenario the
address bar no longer names.

That is the first fix in this sequence that removes the *possibility* rather than
handling an occurrence, and it is the one I should have reached for at finding 2.

## Five findings, one sentence

**Asynchrony lets a UI separate a statement from its subject.** The confirmation
described A and deployed B; a stale confirmation survived navigation; an in-flight
preview re-opened one; an in-flight deploy reported onto the wrong page; and the
page itself lagged behind its own URL.

Four of the five were fixed one at a time, each with a guard around the specific
sequence that had been found. The fifth was fixed by making the state impossible.
The lesson is not "clear state on navigation" — it is that **when the third
instance of a class arrives, the next move is to remove the state that permits it,
not to guard the path that reached it.**
