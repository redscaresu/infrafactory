# ADR-0005: Dual Iteration Controls for Run Loop

## Status
Superseded by ADR-0006

## Context

The run loop needs two independent controls:

- retry on failures with model feedback, bounded by a retry budget;
- continue for a fixed number of total passes even when an iteration succeeds.

A single max-iterations control conflates these behaviors and makes operator intent ambiguous.

## Decision

- Define two config keys:
  - `agent.repair_iterations_max`: maximum failure-triggered retries that feed failure signal to the model.
  - `agent.iterations_target`: total desired pass count, including passes after success.
- Define matching CLI overrides:
  - `--repair-iterations-max`
  - `--iterations-target`
- Do not preserve legacy `max_iterations`/`--max-iterations` compatibility in this migration.
- Enforce deterministic terminal control signaling with one reason per stop event:
  - `target_reached`
  - `repair_budget_exhausted`
  - `stuck`

## Consequences

- Slice 12 implementation work can model failure-repair and fixed-pass objectives independently.
- Run behavior is easier to reason about and test.
- Planning and docs must shift from migration/deprecation language to dual-control semantics.
