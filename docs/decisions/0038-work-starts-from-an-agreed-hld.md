# ADR-0038: Work starts from an agreed high-level design

## Status
Accepted — 2026-09-27 (S205)

## Context

ADR-0037 made epics and stories files, scoped by a visible swarm. Nothing sat above an epic: a goal
went straight from conversation into epics, and the design reasoning lived only in the chat that
produced it.

## Decision

1. **Planning is a chain: HLD → epics → stories → built in parallel**, and the user approves each
   level before the next is made.
2. **An HLD is a file** — `docs/hld/YYYY-MM-DD-<slug>.md`, status `draft | agreed | superseded` —
   co-written with the user on the most capable model (Fable, `high`, since the user waits on each
   turn) and attacked by a second Fable pass at `xhigh` before it is agreed. Only an agreed HLD is
   decomposed.
3. **HLD → epics is a Fable-led swarm** (`/plan-hld`); epics → stories stays Opus-led (`/plan-epic`).
   The HLD split shapes every piece of work below it, so it gets the most capable model; each epic
   split is smaller and well bounded by then.
4. Epics link to their HLD with `hld:`. The hygiene check fails a link to an HLD that does not
   exist, an HLD without a status, and an HLD file name with spaces, which Obsidian would write
   into links as `%20` and the link test could not resolve.

## Consequences

- The design reasoning is a reviewed file, not a transcript.
- A full chain for one HLD runs one design pane, one decomposition swarm, and one scoping swarm per
  epic before anything is built. That is deliberate for work large enough to need an HLD; a
  single-story change still starts from a story.
