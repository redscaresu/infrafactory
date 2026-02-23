# ADR-0006: Run Loop Uses Failure-Only Retry Control

## Status
Accepted

## Context

The dual-control run contract (`repair_iterations_max` and `iterations_target`) added a second operator mental model that is not needed for this product stage.

Desired behavior is simpler:
- stop on first successful iteration;
- retry only after failures, bounded by a single repair budget and existing stuck/transport guards.

## Decision

- Remove `agent.iterations_target` from config.
- Remove `--iterations-target` from CLI.
- Keep `agent.repair_iterations_max` / `--repair-iterations-max` as the single run-loop control.
- Keep deterministic terminal reasons:
  - `target_reached` (first success)
  - `repair_budget_exhausted`
  - `stuck`

## Consequences

- Operator workflow is simpler: one retry knob for failure handling.
- CLI/config contract is smaller and less error-prone.
- ADR-0005 is superseded by this decision.
