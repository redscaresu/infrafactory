# Review pass 211 — S204: planning vault, epics, swarm

`codex exec review --base main`, five passes, 2026-09-27. Two findings, both accepted, the second
by moving the fix to the choke point rather than patching another caller:

1. **P2.** A story slug over 32 characters started its agent under a truncated name that `wait`
   did not know. Fixed with one `agent_name` mapping.
2. **P2.** Manually started swarm agents (the `/plan-epic` flow) registered the raw name while
   `wait` mapped it. The mapping now happens inside `start_agent`, the one function every agent
   goes through, so no caller can disagree.

An earlier revision used a background workflow for scoping; it was replaced by herdr panes so every
agent can be followed (ADR-0037 §4). No findings were declined; this file records the replacement.
