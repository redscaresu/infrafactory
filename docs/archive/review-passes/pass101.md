# Codex review pass 101 — S159a

**Clean.** No correctness regression; the new tests cover the serialisation and
method-handling behaviour.

Three passes, three findings, and all three were one defect: **the zero value is
a lie in a view.** `""` for a version that was never checked, `0001-01-01` for a
moment that never happened, twice.

The slice was planned around exactly that — the UI arc plan warns that rendering
`unobserved` and `unchecked` as blank cells rebuilds the falsehood the three-state
design exists to prevent. Knowing the defect by name did not stop me shipping it
twice more in fields adjacent to the one I had just fixed.

What ended it was not a third careful fix but a test that asserts the *class*:
marshal a deployment with every optional field unset, and fail if `0001-01-01`
appears anywhere in the payload.
