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
- Keep sandbox/live deploy layer deferred (blocked) due current cost implications.

10. Slice 10: Reliability and contract hardening
- Freeze command/output contracts via golden snapshots and schema assertions.
- Normalize CLI error taxonomy and deterministic failure messaging.
- Version run artifact schema with backward-compatible readers.
- Add idempotency/retry safety and performance regression guardrails.
- Finalize permanent sandbox/live deploy block governance docs and ADR.

## Near-term execution order

1. Complete Slice 1.
2. Complete Slice 2.
3. Stabilize with focused tests before Slice 3.

## Live progress tracking

Use `BACKLOG.md` and `STATUS.md` for day-to-day progress; keep this file focused on stable milestones and sequencing.
