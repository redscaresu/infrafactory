# ADR-0001: Foundations

## Status
Accepted

## Context

InfraFactory needs a deterministic, contributor-friendly path to generate and validate Scaleway infrastructure with minimal hidden behavior.

## Decision

- Focus cloud implementation on Scaleway first.
- Use OpenTofu as the IaC engine.
- Implement CLI and core logic in Go.
- Use a stateful Scaleway API mock (Mockway) for fast, repeatable deploy validation.
- Implement work in vertical slices with a runnable CLI at all times.
- Make schema contracts explicit (`scenario.schema.json` and `infrafactory.yaml`).

## Consequences

- Faster iteration and lower ambiguity for contributors.
- Strong testability due to deterministic contracts and explicit boundaries.
- Initial scope is constrained to speed delivery; portability and deeper runtime features are added incrementally.
