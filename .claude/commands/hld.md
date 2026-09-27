---
description: Start or resume a high-level design (docs/hld/) co-written with the user on the most capable model, in its own herdr pane
argument-hint: "<title>  e.g. Firewalled by default"
---

Start the HLD `$ARGUMENTS`. Requires herdr (`HERDR_ENV=1`); if not inside herdr, say so and stop.

1. If an HLD with this title exists in `docs/hld/`, resume it. Otherwise create
   `docs/hld/<today>-<slug>.md` (today from `date +%F`; slug: lowercase, words joined by `-`) from
   the template in `docs/hld/README.md`, with `status: draft`, `title:` and `date:` filled in.
2. Write the co-author's brief to `.swarm/briefs/hld-<slug>.md`:

   > You are co-writing the high-level design in `docs/hld/<file>` with the user, who is talking
   > to you in this pane. Read `AGENTS.md`, `STATUS.md`, `docs/decisions/README.md` and the code the
   > design touches before proposing anything. Ask the user the questions that decide the design,
   > one or two at a time; do not invent requirements. Write the answers into the file section by
   > section, citing evidence (file:line, ADRs, runs). Keep the design at the level of what and
   > why; stories come later. Do not commit, do not touch real cloud or credentials, and do not
   > change `status` — the user agrees the HLD, not you. When the user says the draft is ready,
   > say so and stop.

3. `scripts/swarm.sh agent hld hld-<slug> "$PWD" hld .swarm/briefs/hld-<slug>.md`, then tell the
   user which pane to work in. This session supervises; it does not co-write.
4. When the user says the draft is ready, start one reviewer, role `hld-review`, to attack it:
   goals that cannot be verified, a design that contradicts an ADR or a safety rule, missing
   alternatives, risks not named, open questions that block decomposition. It writes its findings
   to `.swarm/hld-<slug>/review.md`. Relay them to the user and the co-author.
5. When the user agrees it, set `status: agreed`, open a PR for the HLD through the codex loop,
   and `scripts/swarm.sh close hld`. Decompose it with `/plan-hld <file>`.
