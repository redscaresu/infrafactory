# ROADMAP

This roadmap tracks durable milestones. It avoids date-based status snapshots that become stale quickly.
This file is intentionally high-level and mostly stable; day-to-day execution tracking belongs in `BACKLOG.md` and `STATUS.md`.

## Guiding constraints

- Keep the CLI runnable at all times.
- Build in vertical slices.
- Add focused tests with each behavioral change.
- Prefer deterministic behavior and explicit contracts.

## Milestones

1. Slice 1: Bootstrap + parse/validate
- Wire commands: `init`, `generate`, `validate`, `test`, `run`, `mock start`.
- Implement `internal/config` loader + validation.
- Implement `internal/scenario` loader + JSON Schema validation.

2. Slice 2: Generator pipeline
- Define `SeedGenerator` interface.
- Implement 3-phase generation flow and prompt rendering.
- Implement robust `# File:` output parser with tests.

3. Slice 3: Static harness
- Run `tofu init/validate/plan/show -json`.
- Evaluate OPA policies against plan JSON.
- Return structured failure output.

4. Slice 4: Mock deploy harness
- Apply against Mockway.
- Run topology checks and state policy checks.

5. Slice 5: Destroy + run history
- Run `tofu destroy`.
- Verify no orphaned resources.
- Persist run/iteration data in run store.

6. Slice 6: Convergence logic
- Implement feedback loop and stuck detection.
- Add criteria-only holdout flow.

7. Slice 7: CLI orchestration
- Wire command adapters end-to-end across generate/validate/test/run/mock start.
- Freeze command/output contracts.
- Add hermetic and opt-in real-tool smoke coverage.

8. Slice 8: Developer experience
- Add make-based local workflow automation for dependencies/tests/smoke/cleanup.
- Keep default paths hermetic and make real-tool smoke opt-in.
- Document canonical developer commands.

9. Slice 9: Criteria-complete orchestration
- Wire default runtime generator behavior for `generate`/`run`.
- Expand scenario runtime model so criteria and holdout routing data are available to CLI orchestration.
- Execute scenario acceptance criteria in `test`/`run` (topology + state policy + holdout flow).
- Define and enforce criteria support/defer matrix for unsupported sandbox-only checks in current slices.
- Honor validation layer enable/disable flags consistently in CLI orchestration.
- Expand mock command lifecycle operations.
- Keep sandbox/live deploy layer permanently blocked under governance policy.

10. Slice 10: Reliability and contract hardening
- Freeze command/output contracts via golden snapshots and schema assertions.
- Normalize CLI error taxonomy and deterministic failure messaging.
- Version run artifact schema with backward-compatible readers.
- Add idempotency/retry safety and performance regression guardrails.
- Finalize permanent sandbox/live deploy block governance docs and ADR.

11. Slice 11: Generator transport integration
- Implement concrete generator transport wiring for `claude -p` and OpenRouter.
- Keep generation deterministic via strict prompt/input/output contracts and parser reuse.
- Add credential-safety guardrails so transport errors/logs do not leak secrets.
- Add hermetic transport-adapter tests and opt-in smoke tests for real transports.
- Preserve existing CLI/output contracts and failure taxonomy while replacing default transport stubs.

12. Slice 12: Feedback-driven regeneration hardening
- Ensure `run` iteration-N generation receives structured failures from iteration N-1.
- Reduce heuristic post-processing in favor of model-corrected regeneration informed by concrete harness failures.
- Strengthen run-loop convergence quality by improving failure payload fidelity and prompt integration.
- Add focused regression tests proving feedback is injected and iteration metadata is preserved.
- Migrate iteration contract to clearer naming and defaults:
  - config `agent.iterations` (default `3`)
  - CLI `--iterations` override (e.g. `10`)
  - deterministic compatibility/deprecation path for legacy `max_iterations` / `--max-iterations`.
- Ensure failed iterations emit deterministic, structured failure summaries to app logs for each pass.

13. Slice 13: Full application logic logging and observability
- Define a stable application logging contract (fields, levels, redaction, deterministic formatting).
- Define deterministic log destinations for operators and automation (stderr summary + per-run artifact log path).
- Instrument command orchestration paths so generation/validation/test/run decisions are fully traceable.
- Ensure per-pass and per-stage failures are logged with run/iteration context and actionable details.
- Preserve secret-safety/redaction guarantees while increasing observability depth.
- Add focused tests (and where needed golden fixtures) to freeze logging behavior and prevent regressions.

## Near-term execution order

1. Keep Slice 11 closed and stable (transport adapters + secret-safety + smoke gates).
2. Execute Slice 12 contract-first migration (`S12-T1`) before behavior changes.
3. Implement/configure iteration migration (`S12-T2`, `S12-T3`), add per-pass failure logging (`S12-T6`), then test/docs closure (`S12-T4`, `S12-T5`).
4. Start Slice 13 with logging-contract definition, then implement end-to-end instrumentation and docs.

## Live progress tracking

Use `BACKLOG.md` and `STATUS.md` for day-to-day progress; keep this file focused on stable milestones and sequencing.
