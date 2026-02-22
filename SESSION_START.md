# Session Start (Fresh Agent)

## 1) Load minimal context
1. `README.md`
2. `AGENTS.md`
3. `STATUS.md`
4. `BACKLOG.md`
5. `CURRENT_TICKET.md`
6. `docs/process/TICKET_TEMPLATE.md`
7. `docs/process/EXECUTION_PROMPT.md`
8. `docs/decisions/README.md` (+ relevant ADRs)
9. `docs/mockway-contract.md`
10. `CONCEPT.md` only if major design context is needed

## 2) Define target
- Confirm active milestone in `ROADMAP.md`.
- Confirm blockers/next actions in `STATUS.md`.
- Pick next uncompleted ticket from `BACKLOG.md`.
- Fill `CURRENT_TICKET.md` for session execution.

### Slice 7 default execution constraints
- Canonical order:
  `S7-T1 -> S7-T2 -> S7-T12 -> S7-T16 -> S7-T3/S7-T4/S7-T5 -> S7-T6 -> S7-T7 -> S7-T8 -> S7-T9 -> S7-T11 -> S7-T15`
- Parallel lane:
  `S7-T10` can proceed after `S7-T12`.
- Supporting/optional lane:
  `S7-T13` is supporting infrastructure; `S7-T14` remains opt-in real-tool smoke.
- Output contract defaults:
  deterministic human summary + deterministic JSON schema/output ordering.
- Test policy:
  hermetic tests by default, real-tool smoke opt-in only.

### Slice 9 default execution constraints
- Canonical order:
  `S9-T1 -> S9-T10 -> S9-T2 -> S9-T11 -> S9-T3 -> S9-T4 -> S9-T5 -> S9-T6 -> S9-T7`
- Blocked lane:
  `S9-T8` (sandbox/live deploy) is intentionally blocked due cost and credentials-governance implications; do not implement unless explicitly unblocked by policy/ADR.
- Documentation-only lane:
  `S9-T9` is already done and must remain explicit in docs.
- Critical implementation prerequisite:
  expand `internal/scenario.Scenario` runtime model before criteria orchestration wiring (`S9-T10` before `S9-T2+`).
- Criteria support/defer matrix:
  `connectivity`, `http_probe`, `policy`, and `destruction` are in-progress for scenario-driven wiring; `dns_resolution` remains deferred while sandbox is blocked.
- Runtime state reality:
  `test` currently executes mock deploy + destroy lifecycle checks; `run` is criteria-incomplete skeleton orchestration.
- Canonical validation scenario:
  use `scenarios/training/web-app-paris.yaml` for criteria wiring checks unless the ticket requires a narrower fixture.
- Smoke/runtime caveats:
  prefer `http://127.0.0.1:8080` over `localhost` for Mockway checks; if image pull is denied, use `make smoke-mockway-local MOCKWAY_BIN=/path/to/mockway`.
- Real-tool smoke preconditions:
  `tofu` must exist in `PATH`; set `INFRAFACTORY_ENABLE_REALTOOL_SMOKE=1` and/or `INFRAFACTORY_ENABLE_REALTOOL_MOCKWAY=1` with `INFRAFACTORY_MOCKWAY_URL`.
- Halt protocol reminder (from execution prompt):
  halt only for missing mandatory input, sandbox/permission limits, or decision-impacting CLI/schema/architecture changes; blocker output must be exactly:
  `## Blocker`, `## What Was Tried`, `## Needed Input`.

## 3) Execute
- Implement + test.
- Run `go test ./...` (or explain why not).

## 4) Mandatory sync before handoff
- Update `STATUS.md`.
- Update `BACKLOG.md` ticket status.
- Update `CURRENT_TICKET.md` (final state + notes).
- If decision-impacting: update ADR + ADR index.
- If major architecture changed: update `CONCEPT.md`.
- If workflow changed: update `AGENTS.md`.
- Run `bash scripts/check_all.sh`.

## 5) Handoff format
- What changed
- What was verified
- Open blockers/risks
- Exact next step
