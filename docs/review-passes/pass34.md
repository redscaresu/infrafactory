# S166 design review pass 34 — one finding, acted on

### [P2] Three S165 entries in STATUS contradicted each other

Recording the canary left the two earlier S165 entries in place, still saying
"not yet exercised against real Scaleway" and "`create_run_project` not yet
honoured" — both false by then. A reader could no longer tell whether the flag
had been wired or exercised, in the file `AGENTS.md` makes part of the
fresh-session source of truth.

Fixed by **consolidating the three same-day entries into one accurate entry**
rather than patching the stale sentences. Appending a correction above a wrong
statement leaves both on the page; STATUS's own update policy already says
historical detail belongs in `ARCHIVE.md`, not stacked in the current phase.

Fourth finding this session about a document describing a state that had moved.
The recurring cause is the same: I record the new fact and leave the sentence
that framed the old one. Consolidating beats appending when the entries are from
the same day and the same slice.
