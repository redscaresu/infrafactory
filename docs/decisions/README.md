# Decision Records

This folder stores stable architecture decision records (ADRs).

Use ADRs for decisions that affect long-term behavior, interfaces, or contributor workflow.

## Index

- `0001-foundations.md`: base stack and execution model.
- `0002-cli-command-contract.md`: frozen Slice 7 CLI args/flags/exit-code contract.
- `0003-permanent-sandbox-live-deploy-block.md`: superseded by ADR-0010. Previously: permanent governance policy to keep real sandbox/live deploy out-of-scope.
- `0004-generator-transport-contract.md`: Slice 11 transport/config contract for claude/openrouter selection and phase semantics.
- `0005-dual-iteration-controls.md`: superseded by ADR-0006.
- `0006-run-failure-only-retry-control.md`: run-loop contract uses one retry control (`repair_iterations_max`) and stops on first success.
- `0007-scenario-schema-resource-expansion.md`: Slice 18 schema extension adding kubernetes, iam, registry, redis resource definitions.
- `0008-ui-command-and-noui-api-mode.md`: Slice 21 CLI/UI wiring with always-registered `ui` command and `noui` API-only behavior.
- `0009-incremental-deployment-model.md`: Incremental deployment support — single evolving scenario, mockway state persistence, snapshot/restore for feedback iterations, auto-detection.
- `0010-layer3-real-scaleway-deploy.md`: Enable Layer 3 real Scaleway deploy — Layer 2 gates Layer 3, dual-apply with separate tfstate, project bootstrap in HCL, auto-destroy on failure.
- `0011-topology-derivation-layer.md`: Derive topology (connectivity, http_probe) from raw mock state in infrafactory instead of requiring pre-computed maps from mockway/fakegcp.
- `0012-dynamic-pitfalls.md`: Externalize provider pitfalls into `pitfalls/{cloud}.yaml` with auto-learning from run feedback.
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
