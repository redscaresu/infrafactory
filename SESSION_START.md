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

Quick repo-state preflight before selecting a ticket:
```bash
git status --short
git branch --show-current
git log -1 --oneline
```
- If you see unexpected local changes you did not make, stop and ask how to proceed.
- Keep exactly one `in_progress` ticket in `BACKLOG.md` during execution.

## Fresh Context Briefing (Current)

Before writing code, confirm these facts are still true in `STATUS.md`/`BACKLOG.md`:
1. `S9-T8` (sandbox/live deploy, real Scaleway) remains permanently blocked by governance policy (ADR-0003).
2. Output and logs must explicitly surface: `(real deployment skipped for cost reasons for now)` for sandbox/live-blocked behavior.
3. Slice 12 feedback-loop hardening is the active milestone direction; prefer tickets that improve model-guided correction signal over new heuristic post-processing.
4. `run` is criteria-aware and includes criteria-only holdout completion checks; do not regress to coarse stage-only convergence behavior.
5. `dns_resolution` remains auto-pass informational output while sandbox/live deploy is blocked; do not treat it as a hard-fail criterion.
6. Default runtime now uses concrete generator transports; `claude-code` requires `agent.claude.command` in `PATH` and `openrouter` requires `OPENROUTER_API_KEY` plus `agent.openrouter.model`.
7. Slice 13 (queued after Slice 12) is dedicated to full app-logic logging/observability; instrumentation should follow a contract-first approach (`S13-T1`) before broad command-path changes.

Minimal startup verification commands:
```bash
go test ./...
bash scripts/check_all.sh
```

If either command fails, restore the repo to a green baseline before starting a new ticket.

### Fresh Context Addenda (Operational)
- Prefer `run` over manual `generate` + `test` when diagnosing/repairing generation failures; only `run` feeds prior iteration failures into LLM generation (`FeedbackJSON`).
- Avoid introducing new provider-specific string normalization rules in `generate`; prioritize improving feedback quality so the model corrects output itself.
- Mockway startup failures are commonly local port collisions (`0.0.0.0:8080` already allocated); resolve the conflicting container/process before retrying `mock start`.
- Use `http://127.0.0.1:8080` for local Mockway checks in this repo context (more reliable than `localhost`).
- Debug iterative behavior from run artifacts:
  `.infrafactory/runs/<scenario>/<run-id>/iterations/<n>/iteration.json` records stage/failure snapshots per iteration.
- Keep output semantics in mind:
  `output/<scenario>/` is latest generated IaC and is overwritten each run; historical evidence lives under `.infrafactory/runs/`.
- For upcoming Slice 13 logging work:
  preserve secret redaction guarantees while increasing observability depth; logs must remain deterministic and correlation-friendly (`run_id`, `iteration`, `stage`, `check`).
  require explicit sink definitions so operators can always inspect full app logic flow from terminal output and run artifacts.
  include deterministic inspection commands in docs (for example, `tail`/`rg` against run-scoped log artifacts).

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
  `S9-T8` (sandbox/live deploy) is permanently blocked by ADR-0003; do not implement unless ADR-0003 is superseded.
- Documentation-only lane:
  `S9-T9` is already done and must remain explicit in docs.
- Critical implementation prerequisite:
  expand `internal/scenario.Scenario` runtime model before criteria orchestration wiring (`S9-T10` before `S9-T2+`).
- Criteria support/defer matrix:
  `connectivity`, `http_probe`, `policy`, and `destruction` are wired for scenario-driven execution; `dns_resolution` currently emits deterministic auto-pass informational output while sandbox is blocked.
- Runtime state reality:
  `test` executes criteria-driven mock deploy + destroy lifecycle checks; `run` is criteria-aware orchestration with holdout completion checks.
- Canonical validation scenario:
  use `scenarios/training/web-app-paris.yaml` for criteria wiring checks unless the ticket requires a narrower fixture.
- Smoke/runtime caveats:
  prefer `http://127.0.0.1:8080` over `localhost` for Mockway checks; if image pull is denied, use `make smoke-mockway-local MOCKWAY_BIN=/path/to/mockway`.
- Real-tool smoke preconditions:
  `tofu` must exist in `PATH`; set `INFRAFACTORY_ENABLE_REALTOOL_SMOKE=1` and/or `INFRAFACTORY_ENABLE_REALTOOL_MOCKWAY=1` with `INFRAFACTORY_MOCKWAY_URL`.
- Halt protocol reminder (from execution prompt):
  halt only for missing mandatory input, sandbox/permission limits, or decision-impacting CLI/schema/architecture changes; blocker output must be exactly:
  `## Blocker`, `## What Was Tried`, `## Needed Input`.

### Slice 10 default execution constraints
- Canonical order:
  `S10-T2/S10-T3 -> S10-T6 -> S10-T1 -> S10-T4/S10-T5 -> S10-T7`
- Contract hardening rule:
  do not freeze final goldens (`S10-T1`) before error-taxonomy (`S10-T2`), run-artifact versioning (`S10-T3`), and explainability fields (`S10-T6`) stabilize.
- Backward compatibility rule:
  run artifact readers must remain compatible with pre-versioned records.
- Hermetic default rule:
  new performance/idempotency checks must run without mandatory network/external cloud dependencies.

### Slice 11 default execution constraints
- Canonical order:
  `S11-T1 -> (S11-T2 || S11-T3) -> S11-T4 -> (S11-T5 || S11-T7) -> S11-T6`
- Parallelization rule:
  `S11-T2` and `S11-T3` are parallel after `S11-T1`; `S11-T5` and `S11-T7` are parallel after adapter implementation.
- Current state:
  `S11-T1`..`S11-T4` are done; remaining closure is `S11-T5`, `S11-T7`, then `S11-T6`.
- Contract rule:
  preserve existing CLI/output error taxonomy while replacing default transport stubs with concrete adapters.
- Safety rule:
  transport adapters must not leak raw secrets (`API keys`, tokens, prompt bodies) in surfaced errors/logs.
- CI rule:
  hermetic tests remain default; real transport smoke coverage stays opt-in.

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
