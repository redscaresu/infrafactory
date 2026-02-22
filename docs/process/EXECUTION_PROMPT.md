# Reusable Execution Prompt

```text
Autonomous mode. Execute without confirmation unless blocked by missing mandatory input or sandbox restrictions. Execute only the next uncompleted ticket (no scope expansion).

Bootstrap (strict order):
README.md -> SESSION_START.md -> AGENTS.md -> STATUS.md -> ROADMAP.md -> docs/process/TICKET_TEMPLATE.md -> docs/decisions/README.md -> docs/mockway-contract.md -> only relevant sections of CONCEPT.md.

Planning:
- Build/refresh full Slice 1-6 backlog from ROADMAP.md.
- Per ticket include: priority, dependencies, in-scope files/packages, out-of-scope, acceptance criteria, required tests.
- Select the next uncompleted ticket from `BACKLOG.md` as the execution target.
- If ADR decision is unclear, default to no ADR unless schema/CLI/architecture boundaries changed.

Execution (single ticket):
- Implement end-to-end (code + focused tests).
- Keep CLI runnable and handlers thin.
- Run `bash scripts/check_all.sh`.

Doc sync (AGENTS.md rules):
- Always update STATUS.md.
- ADR only if decision-impacting.
- CONCEPT.md only for major architecture/durable design changes.

Output (exact headings, in order):
## Backlog
## Selected Ticket
## Changes
## Verification
## Blockers/Risks
## Next Step

If blocked and cannot proceed, stop implementation and return only:
## Blocker
## What Was Tried
## Needed Input
```
