---
description: Decompose an agreed HLD (docs/hld/) into epics with a herdr swarm — every agent in its own pane; proposal only
argument-hint: "<hld-file-name-without-.md>  e.g. 2026-09-27-firewalled-by-default"
---

Decompose `docs/hld/$1.md` into epics. It must be `status: agreed`; if not, say so and stop.
Requires herdr. Run it as a swarm (`docs/operations.md` § The planning chain); every agent writes
its answer as JSON to `.swarm/$1/<name>.json`, the only file it may write, and replies with the
path. Briefs go in `.swarm/briefs/<name>.md`; start each with
`scripts/swarm.sh agent plan-$1 <name> "$PWD" <role> <brief>` and supervise with
`scripts/swarm.sh wait <name>` in the background.

Every brief begins: *You are decomposing the agreed HLD docs/hld/$1.md; read it first. Read-only —
write nothing but your output file, never commit, never touch real cloud or credentials. Cite
evidence as file:line. Keep free text short.*

1. **Survey** — role `survey`, three in parallel, output `{summary, facts:[{fact, evidence}]}`:
   `survey-code` (what the design changes and where), `survey-constraints` (ADRs, policies, Layer 3
   safety, hygiene and audit tests), `survey-overlaps` (existing epics, stories, open PRs).
2. **Decompose** — one `hld-lead`, role `hld-lead`, given the surveys. Output
   `{epics:[{slug, goal, done_when, out_of_scope, constraints, depends_on, areas}], order, contradictions}`.
   Each epic is independently shippable and has a **Done when** that could fail; `areas` names the
   packages or directories it changes, so epics that can be scoped and built in parallel are
   visible; every goal in the HLD belongs to exactly one epic; every disagreement between the HLD
   and the code is a contradiction.
3. **Verify** — one `skeptic-<slug>` per epic, role `skeptic` (capped at five; name any unchecked),
   output `{sound, problems:[{why}], fix}`: a gap no epic covers, two epics overlapping, an epic too
   big to scope, a **Done when** that passes on broken work. Plus one `codex`, role `codex`, on the
   same questions across the whole set.
4. **Critic** — one `critic`, role `critic`: an HLD goal, risk or rollout step that no epic owns.

Report in this order: refuted epics, contradictions, codex and critic findings the plan does not
answer, then the surviving epics and their order. Ask before writing. On approval write one
`docs/epics/<slug>.md` per epic (front matter `status: active` or `later`, `hld: $1`; body: goal,
**Done when**, out of scope, constraints), add a link to each under the HLD's `## Epics`, open a PR
through the codex loop, and `scripts/swarm.sh close plan-$1`. Then scope each epic with
`/plan-epic <slug>`, one at a time: each is its own ten-agent swarm.
