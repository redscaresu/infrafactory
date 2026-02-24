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
3. Slice 16 issue-driven robustness hardening is complete; preserve its guarantees while keeping the remaining blocked lane (`S9-T8`) unchanged unless ADR-0003 is superseded.
4. Slice 17 hardening follow-ups are complete (`S17-T1`, `M31`, `M32`, `M33`, `M34`); preserve opt-in capture defaults, convergence fixes, self-review canonical-only matching, and docs alignment.
5. `run` is criteria-aware and includes criteria-only holdout completion checks; do not regress to coarse stage-only convergence behavior.
6. `dns_resolution` remains auto-pass informational output while sandbox/live deploy is blocked; do not treat it as a hard-fail criterion.
7. Default runtime now uses concrete generator transports; `claude-code` requires `agent.claude.command` in `PATH` and `openrouter` requires `OPENROUTER_API_KEY` plus `agent.openrouter.model`.
8. Slices 13-15 are complete in MVP form; preserve logging contracts, feedback fidelity, and adaptive retry behavior while applying issue fixes.
9. Slice 16 includes schema-validation hardening; scenario loads must not silently skip schema validation when schema paths are unavailable.
10. For ticket-planning closures, record refinement pass outcomes and require at least one improvement pass plus a no-change verification pass in both `STATUS.md` and `CURRENT_TICKET.md`.

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
- Provider schema extraction is lazy and generate-path-only:
  first `generate`/`run` generation call attempts schema extraction once (cached in runtime); `validate`/`test`/`mock` should not pay that startup cost.
- Schema extraction is best-effort:
  if `tofu init/providers schema` fails, generation proceeds without schema-enriched prompts; inspect logs for `provider_schema` skip details before debugging prompt quality.
- Preserve Slice 13-15 guarantees while fixing Slice 16 issues:
  keep deterministic JSON-line logging fields/redaction behavior, structured run feedback (`failure_class` and detailed failure context), and adaptive transport stop behavior with persisted diagnostics.
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
  do not implement config/CLI iteration behavior before `S12-T1` freezes run-loop control semantics.
- Single-control rule:
  use only failure-triggered retry budget (`repair_iterations_max`); stop on first successful iteration.
- Stop-signal rule:
  emit one canonical terminal stop reason for a single stop event (`target_reached`, `repair_budget_exhausted`, or `stuck`); avoid dual control markers in output/logs/artifacts.
- README closure rule:
  after Slice 12 implementation closure (`S12-T5` done), run README optimization passes until two consecutive no-change outcomes are recorded.

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

### Slice 16 default execution constraints (completed)
- Canonical order:
  `S16-T1 -> (S16-T2 || S16-T3 || S16-T7) -> S16-T4 -> S16-T5 -> S16-T6 -> S16-T8`
- Cancellation rule:
  command adapters must use `cmd.Context()` for all runtime/harness/generator/mock operations; avoid `context.Background()` in command paths.
- Safety rule:
  bound external response reads and env injection behavior deterministically (no unbounded `ReadAll`, no duplicate override keys in subprocess env).
- Validation rule:
  scenario schema loading must fail deterministically (or use embedded schema) when schema files are unavailable; no silent schema-validation bypass.

### S17-T1 implementation reference (completed)
- Activation contract:
  capture is env-gated (`INFRAFACTORY_CAPTURE_LLM_RAW=1`); default behavior persists no LLM prompt/raw artifacts.
- Artifact contract:
  capture files are written under run artifacts (`.infrafactory/runs/<scenario>/<run-id>/iterations/<n>/`) with deterministic phase naming and stable metadata envelopes:
  `llm_raw_<phase>.json` and `llm_prompt_<phase>.json`.
- Safety contract:
  deterministic secret-like redaction and hard byte caps with explicit truncation markers are applied before persistence.
- Feedback-debugging guardrail:
  use paired prompt/response artifacts from the same iteration to verify whether failure feedback reached model input before changing prompt wording.
- Compatibility guardrail:
  preserve default output contract, terminal reasons, and existing run artifact readers when capture is disabled.

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
