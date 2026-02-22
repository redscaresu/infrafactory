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
8. Slice 14 (queued after Slice 13) is dedicated to run-loop feedback fidelity for model-guided regeneration; avoid regressing to coarse retry payloads such as generic `validation failed` when structured validate/test/generate failures are available.
9. Slice 15 (queued after Slice 14) is dedicated to adaptive retry and transport resilience; transport-dominated failures should stop with deterministic actionable reasons instead of consuming full iteration budgets.

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
- For upcoming Slice 14 feedback-fidelity work:
  prefer `run` feedback payload enrichment over prompt-only wording tweaks; preserve structured failure fields (`layer`, `stage`, `check`, `command`, `detail`, optional `policy`/`resource`) into `FeedbackJSON`.
  classify retry guidance by failure type (IaC validation/policy defects vs transport/timeout defects) so model retries are actionable.
  for planning/refinement tickets in this area, perform refinement passes until two consecutive passes yield no improvements, and record each pass outcome in `CURRENT_TICKET.md` and `STATUS.md`.
- For upcoming Slice 15 adaptive-retry work:
  codify deterministic continuation/stop behavior by failure class and avoid retry churn when failures are transport-dominated.
  persist per-iteration transport diagnostics in run artifacts so operators can separate IaC defects from runtime/dependency constraints.
- For any future planning refinement over unfinished slices (`todo`/`blocked` backlog work):
  require the same refinement protocol: continue until two consecutive no-change passes, and explicitly record pass outcomes in both `CURRENT_TICKET.md` and `STATUS.md` for fresh-context continuity.
  when refining blocked slices (for example `S9-T8`), constrain work to governance/docs/risk messaging only unless the blocking ADR/policy is explicitly superseded.
- Optimized post-slice review prompt (apply to each unfinished slice after implementation):
  "After completing this slice, run a dedicated review-improve pass over code, tests, docs, and artifacts. Apply any improvements that increase correctness, determinism, observability, and operator clarity. Repeat review-improve passes until two consecutive passes find no further improvements. Record each pass outcome in `STATUS.md` and `CURRENT_TICKET.md`."

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

### Slice 12 default execution constraints
- Canonical order:
  `S12-T1 -> (S12-T2 || S12-T3) -> S12-T6 -> S12-T4 -> S12-T5`
- Contract-first rule:
  do not implement config/CLI migration work before `S12-T1` freezes naming/default/compatibility semantics.
- Compatibility rule:
  when both `--iterations` and legacy `--max-iterations` are present, enforce deterministic precedence and warning behavior per contract tests.
- Stop-signal rule:
  emit one canonical terminal stop reason for a single stop event; avoid dual stuck/max markers in output/logs/artifacts.

### Slice 13 default execution constraints
- Canonical order:
  `S13-T1 -> S13-T2 -> (S13-T3 || S13-T4) -> S13-T5 -> S13-T6`
- Contract rule:
  freeze logging field/level/redaction contract before broad instrumentation rollout.
- Sink rule:
  preserve deterministic sink behavior (`stderr` + run-scoped artifact path) and stable correlation fields (`run_id`, `iteration`, `stage`, `check`).

### Slice 14 default execution constraints
- Canonical order:
  `S14-T1 -> S14-T2 -> S14-T3 -> S14-T4 -> S14-T5`
- Feedback-signal rule:
  prioritize structured failure payload fidelity over prompt phrasing tweaks; avoid coarse-only retry payloads when structured failures exist.
- Classification rule:
  preserve failure-class tagging (`iac_validation`, `transport_runtime`, `orchestration_control`) in `FeedbackJSON`.

### Slice 15 default execution constraints
- Canonical order:
  `S15-T1 -> (S15-T2 || S15-T3) -> S15-T4 -> S15-T5 -> S15-T6`
- Adaptive-retry rule:
  transport-dominated failure runs should terminate with deterministic actionable reasons rather than exhausting iteration budget.
- Diagnostics rule:
  run artifacts must include stable transport diagnostics (phase/timeout/signal/stderr summary/duration) with backward-compatible reads.

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
