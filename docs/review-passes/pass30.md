# S166 design review pass 30 — one finding, acted on

### [P2] The fresh-session handoff still pointed at S165

`STATUS.md` records S165 complete and canaried and S166 as the design to review,
while `docs/NEXT_SESSION.md` still opened with **START HERE — S165**. Since
`AGENTS.md` makes that file the first thing a fresh session reads, the next
engineer would have picked up an already-merged slice.

Correct, and the second time this exact class has been caught (pass 14 was the
same file, one arc earlier) — which says the coupling between "record the status"
and "repoint the handoff" is one a reviewer catches and I do not. `AGENTS.md:43`
already requires it; the requirement is not the problem, remembering it is.

Repointed to S166, with the design doc named as the thing to read *and disagree
with* before anything is built, plus the S165 caveats a user of the flag needs
today (two projects before S167; the auto-destroy path keeps its project).
