# Codex review pass 103 — S161

One finding, accepted, and it is **the page's own thesis violated in the page.**

## [P2] A failed first read said "Nothing is deployed."

On a failed initial load, `loaded` became true while `deployments` was still the
empty array, so the summary line rendered `estateSummary([], [])` — *"Nothing is
deployed."* — directly above a red banner saying the estate could not be read.

Every other part of this slice exists to stop exactly that: an empty list under a
failure means **we do not know**, not that nothing is running. The banner said so.
The line above it said the opposite, and the line above it is the one a person
reads first.

## The test that should have caught it checked the wrong half

`a failed read says so rather than showing an empty table` asserted the banner
was visible and stopped there. The summary was two elements away, contradicting
it, and the test passed.

That is the same mistake as S156d's synthetic detail: a test written alongside
the code, asserting the thing the code was written to do rather than the property
the system must hold. The property here is *"nothing on this page may claim the
estate is empty unless we know it is"* — so the test now asserts the summary text
too.

## Three states, not two

`estateSummary` takes `loading | failed | loaded`. All three produce an empty
list and they mean different things:

- **loading** — we have not asked yet
- **failed** — we asked and could not find out
- **loaded** — we asked, and the answer is what you see

Only the last may say "Nothing is deployed". This is the same three-state
distinction as `unobserved` versus `unhealthy` versus `healthy`, one level up: the
absence of an answer is not an answer.
