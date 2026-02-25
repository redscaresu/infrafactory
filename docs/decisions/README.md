# Decision Records

This folder stores stable architecture decision records (ADRs).

Use ADRs for decisions that affect long-term behavior, interfaces, or contributor workflow.

## Index

- `0001-foundations.md`: base stack and execution model.
- `0002-cli-command-contract.md`: frozen Slice 7 CLI args/flags/exit-code contract.
- `0003-permanent-sandbox-live-deploy-block.md`: permanent governance policy to keep real sandbox/live deploy out-of-scope.
- `0004-generator-transport-contract.md`: Slice 11 transport/config contract for claude/openrouter selection and phase semantics.
- `0005-dual-iteration-controls.md`: superseded by ADR-0006.
- `0006-run-failure-only-retry-control.md`: run-loop contract uses one retry control (`repair_iterations_max`) and stops on first success.
- `0007-scenario-schema-resource-expansion.md`: Slice 18 schema extension adding kubernetes, iam, registry, redis resource definitions.
- `DECISION_RUBRIC.md`: yes/no gate for deciding when ADR is required.
- `ADR_TEMPLATE.md`: copy/paste template for new ADRs.

## When to add an ADR

Add an ADR when a change affects one or more of:
- cross-package architecture or boundaries
- external tool/service contracts (OpenTofu, Mockway, OPA, CLI behavior)
- source-of-truth precedence or schema semantics
- long-term contributor workflow or governance
- irreversible or expensive-to-revert implementation choices

If unsure, run the rubric in `DECISION_RUBRIC.md`.

## ADR template

Use this structure for new ADRs:

```md
# ADR-XXXX: Title

## Status
Accepted | Proposed | Superseded

## Context
Problem and constraints.

## Decision
Chosen approach.

## Consequences
Benefits, tradeoffs, and follow-up work.
```
