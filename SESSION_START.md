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
