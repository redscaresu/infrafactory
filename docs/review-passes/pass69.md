# Codex review pass 69 — S158 plan

One finding, accepted — **and it was wider than the finding.**

## [P3] The plan link in STATUS.md did not resolve

`STATUS.md` lives at the repository **root** while almost everything it points at
lives under `docs/`, so `](plans/live-lifecycle-e2e-plan.md)` looks right and
resolves to nothing.

Checked rather than fixed-in-place: **all four** relative links in STATUS.md were
broken, and I wrote three of the others earlier in the same session —
`layer3-coverage.md`, `status/s155b-upgrade-canary.md`,
`status/s168-cutover-canary.md`. Codex flagged one; the same mistake had been
made four times.

## Closed at the seam rather than by inspection

The repository was already clean once those four were fixed, so a ratchet costs
nothing and starts green: `TestDocLinksResolve` walks `docs/` plus the root
documents and asserts every relative markdown link resolves. Verified against
synthetic drift — reverting one link fails it with the file, the target and the
path it resolved to.

It also refuses to pass if the walk finds fewer than two files, so it cannot
quietly become vacuous. That guard exists because of pass 34: an audit that
narrowed its own scope read as coverage for three review passes while the defect
it was meant to catch landed in three other files.

## Why a P3 got a ratchet

This is a documentation nit by severity and a durable one by nature: nobody
notices a dead link in review, and the reader who finds it is the person the
handoff was written for. The cost of prevention was thirty lines.

## Nothing declined this pass.

## Addendum, pass 70: the ratchet failed on the prose describing it

The first version matched links inside **code spans**, so this very file — which
writes the broken form in backticks as an example of the mistake — failed the
check it documents. So did STATUS.md's illustration of the same thing.

The pre-commit hook caught it before it landed, which is the guard working: the
commit was refused rather than the suite going red on main.

`stripCode` blanks fenced blocks first (a fenced region can contain unbalanced
single backticks that would otherwise throw the inline pass off), then inline
spans, replacing with spaces so nothing outside a span shifts position.
Re-verified both ways: it still fails on a genuinely broken link, and no longer
fails on an illustration of one.

**A checker that cannot tell an example from an instance will always fail hardest
on the document explaining it.**
