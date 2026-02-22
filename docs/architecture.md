# Architecture Overview

InfraFactory is a scenario-driven infrastructure factory for Scaleway.

## Core flow

1. Human writes a scenario YAML file.
2. Generator produces OpenTofu files from that scenario.
3. Harness validates generated infrastructure through static checks and deployment checks.
4. Structured feedback is fed back into the generator until convergence or max-iteration stop.

## Main components

- `internal/config`: loads and validates runtime config (`infrafactory.yaml`).
- `internal/scenario`: loads and validates scenario YAML against `scenario.schema.json`.
- `internal/generator`: seed generation pipeline and output file parsing.
- `internal/harness`: OpenTofu and policy/topology validation orchestration.
- `internal/feedback`: structured failure reporting for iteration loops.
- `internal/runstore`: run artifacts and iteration history.

## Design goals

- Deterministic and testable behavior.
- Explicit contracts via schema and typed errors.
- Thin CLI layer with reusable internal packages.
- Layered validation with fail-fast behavior.

## Validation layers (target model)

1. Static: `tofu validate`, `tofu plan`, OPA on plan JSON.
2. Mock deploy: `tofu apply` to Mockway, topology + state policy checks.
3. Sandbox deploy: real Scaleway probes (stub in early slices).
4. Destroy verification: ensure no orphans remain.

## Canonical references

- Full design log and rationale: `CONCEPT.md`
- Mockway integration contract: `docs/mockway-contract.md`
- Formal scenario contract: `scenario.schema.json`
- Runtime config example/shape: `infrafactory.yaml`
- Prompt templates: `prompts/*.md`
