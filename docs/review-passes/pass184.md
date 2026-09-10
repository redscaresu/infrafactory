# Review passes 184+ — every run reconciles what it did not clean up

`codex exec review --base main` on `fix/reconcile-stray-run-projects`.
**Five findings, all accepted.** Three were the same defect at three different
levels, which is the interesting part.

## The change

Eight stray `if-run-*` projects accumulated over two days, four still running an
instance, found only because a human listed projects on a hunch. Every Layer 3
run now performs `live reconcile`'s comparison at the end and fails on a stamped
project nothing explains — including strays inherited from earlier runs.
ADR-0024 promised this and S157a built the comparison; nothing called it.

## The same false green, three times

**[P2] the failures were appended after `status` was decided.** A
`target_reached` run would have written `success` with a stray outstanding —
precisely the thing the check exists to prevent, reintroduced by where the call
sat. Moved above the decision.

**[P2] the JSON said `failed` and the process exited 0.** The final error path
still tested only `terminalReason` and `holdoutBlocked`. Every caller that is a
script — the PR gate included — would have read success. The status decision is
now one function, `runCommandStatus`, used by both, so the two cannot drift.

**[P2] unreadable live records were discarded.** `store.List()` returns them and
this threw them away, so a record it could not read produced a clean pass. "Could
not check" and "nothing leaked" must never look alike.

## Also accepted

**[P3] the pass line counted every project in the organization**, not the
stamped ones — "12 projects carry infrafactory's stamp" when eleven belong to
somebody else. That line is the evidence an operator reads for "nothing leaked",
so overstating what was checked is the whole problem in miniature.

## Why `runCommandStatus` exists now

It was an inline boolean expression. An expression cannot be tested without
driving the entire command, which is why the first version shipped with the new
clause in the status but not in the exit code. Each clause is a promise this
project has broken before; making them a named function made the mutation test
possible, and the mutation is caught.

## Mutation checks

| mutation | result |
|---|---|
| never report strays | `…FailsOnAnUnexplainedProject` fails |
| treat a list error as clean | `…FailsWhenItCannotList` fails |
| strays no longer block the status | `TestRunReportsFailureWhenAStrayProjectSurvives` fails |

## What it still will not do

It reports and never destroys, for the reason `live reconcile` does not: a
project the records cannot explain is by definition not understood, and deleting
what you do not understand is how a reconciler becomes the incident. The blast
radius is somebody's running infrastructure.
