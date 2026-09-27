---
description: Scope an infrafactory epic (docs/epics/<slug>.md) into adversarially-checked stories with a herdr swarm — every agent in its own pane
argument-hint: "<epic-slug>  e.g. firewall-end-to-end"
---

Scope the epic `$1` into stories. Run it as a swarm so the user can watch every agent; do not
decompose it yourself in one pass. Requires herdr (`HERDR_ENV=1`); if not inside herdr, say so and
stop. If `$1` is empty, list `docs/epics/*.md` (not the README) and ask which. If the epic does not
exist, help the user write it from `docs/epics/README.md` first.

Every agent writes its answer as JSON to `.swarm/$1/<name>.json` — the only file it may write —
and replies with that path. Put each prompt below in `.swarm/briefs/<name>.md`, then:

    scripts/swarm.sh agent scope-$1 <name> "$PWD" <role> .swarm/briefs/<name>.md
    scripts/swarm.sh wait <name>        # run in the background, one per agent

Every brief begins with the ground rules: *You are scoping docs/epics/$1.md; read it first.
Read-only — write nothing except your output file, never commit, never touch real cloud or
credentials. Cite evidence as file:line. Keep free text short.*

**1. Survey** — three agents, role `survey`, in parallel. Output `{summary, facts:[{fact, evidence}]}`.
- `survey-lands`: where the change lands — packages, functions, tests, fixtures, golden files,
  scenarios — and which existing tests must change.
- `survey-constraints`: the ADRs (`docs/decisions/README.md`), policies, Layer 3 safety rules
  (`docs/operations.md`), `AGENTS.md` rules, and hygiene and audit tests it would trip.
- `survey-overlaps`: `docs/stories/`, `docs/epics/`, open PRs, the last 20 merges; which existing
  stories this epic absorbs, blocks or depends on.

**2. Decompose** — one agent, `lead`, role `lead`, given the three survey files. Output
`{stories:[{slug, title, kind, risk, scope, done_when, touches, depends_on}], waves, contradictions}`.
One story is one PR; `done_when` must be able to fail; `kind` is code | docs | chore | verify |
lead (real cloud or credentials) | operator (a human step); `risk: high` for Layer 3, teardown,
safety or hygiene paths; no shared hot file (`AGENTS.md`, `STATUS.md`, `infrafactory.yaml`,
`scenario.schema.json`, `scripts/check_doc_hygiene*`) in more than one story; reuse existing
stories by slug; record every disagreement with the epic under `contradictions`. If a survey
failed, it says what it could not check.

**3. Verify** — in parallel:
- one `skeptic-<slug>` per story, role `skeptic`, capped at five (say which were not checked).
  It tries to refute the story: wrong about the code, too big for one PR, an unstated dependency,
  a `touches` clash inside its wave, or a `done_when` that passes while the work is broken.
  Output `{sound, problems:[{why}], fix}`; unsure means `sound: false`.
- one `codex`, role `codex` (read-only, a different model family): missing stories, stories too
  big for one PR, `done_when` criteria that could pass on broken work, and overlaps within a wave.

**4. Critic** — one `critic`, role `critic`: what the plan missed — a change the epic's
**Done when** needs that no story makes, an unrespected constraint, a test or doc nobody updates, a
real-cloud step hidden inside an agent story. Under 250 words.

**Then report**, in this order: refuted stories and why; contradictions; codex and critic findings
the plan does not answer; the surviving stories with `kind`, `touches` and waves. Ask before
writing anything. On approval write one `docs/stories/<slug>.md` per story (front matter `kind`,
`status: ready` or `blocked` with `blocked_by`, `epic: $1`, `depends_on`, `touches`, `risk` when
high; body: scope and **Done when**), open a PR through the codex loop, and `scripts/swarm.sh close
scope-$1`. Waves are then built with `scripts/swarm.sh story <slug>`.
