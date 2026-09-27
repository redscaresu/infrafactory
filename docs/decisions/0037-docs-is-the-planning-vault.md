# ADR-0037: docs/ is the planning vault, and epics are scoped and built by a visible swarm

## Status
Accepted — 2026-09-27 (S204)

## Context

Open work became one file per story in S203 (ADR-0035 amendment). The next need was to scope
goals larger than one PR into stories, and to see the work as a board. Moving planning into a
separate Obsidian vault was considered and rejected: agents, CI and PRs only see the repository,
and a copy outside it would drift, as the old handoff file had.

## Decision

1. **`docs/` is an Obsidian vault.** The files stay the source of truth; Obsidian is a view.
   `docs/Board.base` derives the board from story front matter, so there is still no shared file
   to edit. Only `docs/.obsidian/app.json` is committed, fixing links to markdown form so the
   link test keeps checking them; the rest of `.obsidian/` is per-user and ignored.
2. **Epics are files** (`docs/epics/<slug>.md`: goal, **Done when**, out of scope, constraints).
   A story joins one with `epic:`; the doc-hygiene check fails a reference to an epic that does
   not exist, and an epic without a status.
3. **An epic is scoped by `/plan-epic`**: survey, decompose into one-PR stories with `kind`,
   `touches` and `depends_on`, a skeptic per story, an independent codex pass, and a critic. It
   proposes; the user approves before story files are written.
4. **Every agent runs in its own herdr pane** (`scripts/swarm.sh`), scoping and building alike,
   so each one can be followed. A background workflow was built first and replaced for that reason:
   its subagents cannot be watched.
5. **Model and effort are chosen by role, in one place** (`policy()` in `scripts/swarm.sh`):
   Opus for judgment, Sonnet for reading and running, codex as the cross-model check, Fable only
   on escalation. A story's `kind` and `risk` pick its role; `lead` and `operator` stories are
   refused, so real cloud and human steps never go to a swarm agent.

## Consequences

- A scoping run costs about ten agents, so it is for epics, not for single stories. Each is a full
  interactive session, so a swarm costs more than background subagents; visibility is the trade.
- `touches` is what makes parallel waves safe to choose; a story without it cannot be checked for
  overlap and should run alone.
- The Kanban community plugin was not adopted: it stores the board as one shared file, the
  conflict S203 removed.
