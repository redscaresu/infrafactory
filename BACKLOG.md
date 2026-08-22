# BACKLOG

Cross-arc maintenance tickets (M-numbered). **Active slice work lives in `docs/plans/<arc-name>-plan.md`** (see `AGENTS.md` § "Planning a New Arc").

Legend: `todo` | `in_progress` | `blocked` | `done` | `wontfix`

| id | title | priority | status | owner |
|---|---|---|---|---|
| M99 | Scenario-page visual baselines are environment-dependent (pass/fail flips on whether mockway is listening on :8080) | P2 | todo | — |

**M99 detail.** The `Next Run Mode` card in `ui/src/routes/scenarios/[...path]/+page.svelte` renders a variable number of elements: one grid tile when the run-mode API is unreachable, three when it responds, plus a conditional `runModeError` line. Page height therefore differs by tens of px depending on whether mockway happens to be running, and `toHaveScreenshot` compares full-page height. Masking does not help — the Makefile comment on `ui-baseline-update` already notes masks "DON'T constrain natural-flow layout height". Committed baselines currently encode the **mockway-down** state, so `make test` fails for anyone who happens to have mockway up. Fix is to make the card render a fixed structure (always three tiles, "unavailable" as the value) rather than conditionally, so the layout is stable either way. Surfaced 2026-08-22 while landing the Layer 3 arc.

Historical M-tickets (~100 done entries) live in `BACKLOG_ARCHIVE.md` § "Maintenance M-tickets (archived 2026-06-03)". S1–S52 slice tickets also live in that file.

## Operating notes

- This file is for **maintenance work that doesn't fit an arc** (e.g. dependency bumps, lint sweeps). Routine fixes-and-features go in the active arc plan instead.
- Don't pile work here speculatively. If no active M-tickets exist, the table stays empty — that's correct.
- Schema is intentionally smaller than the historical 7-column version (id / slice / title / priority / status / deps / owner). Active M-tickets rarely need slice or deps fields; drop them when re-introducing.
