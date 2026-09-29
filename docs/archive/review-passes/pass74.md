# Codex review pass 74 — S156b

One finding, **declined**.

## [P2] "Handle historical version-drift observations" — declined

The finding: observations written before pass 73 have `Status: healthy`,
`Version: unconfirmed` and an **empty detail**, and the gate skips them on
`o.Detail == ""`, so a real class of reproduced observation stays invisible.

Two reasons, and the second is the one that decides it.

### There are none

Checked rather than assumed: 4 observations exist on disk, **0** of them
detail-less drift. `version_path` shipped in S155a earlier today, so the window
in which such a record could have been written is a few hours on one machine, and
nothing in it drifted.

### Including them would make the output worse

This is the argument that would hold even if there were hundreds.

A candidate's `Example` is what an eventual rule is written from. A detail-less
observation has no words — so promoting one produces a candidate that **S156c
cannot turn into a pitfall**, and the gate's whole contract is that a candidate is
something worth learning from.

The alternatives are worse still:

- **Synthesise a detail** — fabricating words nobody observed, in a system whose
  central discipline is that a record states what was seen.
- **Group them under a placeholder** — every historical drift, whatever it
  actually was, merging into one candidate whose text is a placeholder. That
  manufactures reproduction out of ignorance, which is precisely what the
  reproduction gate exists to prevent.

The right fix was the one pass 73 already made: **new observations carry the
detail**. A record written before the system could describe itself is not
evidence that was lost, it is evidence that was never captured — and the gate
saying nothing about it is correct.

Recorded rather than dismissed: if a future migration ever wants to backfill
observation details, the constraint is that it must not invent them.

## Where the slice stands

Two passes, two findings — one accepted (and the most important of the slice),
one declined.
