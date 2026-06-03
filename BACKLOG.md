# BACKLOG

Cross-arc maintenance tickets (M-numbered). **Active slice work lives in `docs/plans/<arc-name>-plan.md`** (see `AGENTS.md` § "Planning a New Arc").

Legend: `todo` | `in_progress` | `blocked` | `done` | `wontfix`

| id | title | priority | status | owner |
|---|---|---|---|---|

_No active M-tickets._ Historical M-tickets (~100 done entries) live in `BACKLOG_ARCHIVE.md` § "Maintenance M-tickets (archived 2026-06-03)". S1–S52 slice tickets also live in that file.

## Operating notes

- This file is for **maintenance work that doesn't fit an arc** (e.g. dependency bumps, lint sweeps). Routine fixes-and-features go in the active arc plan instead.
- Don't pile work here speculatively. If no active M-tickets exist, the table stays empty — that's correct.
- Schema is intentionally smaller than the historical 7-column version (id / slice / title / priority / status / deps / owner). Active M-tickets rarely need slice or deps fields; drop them when re-introducing.
