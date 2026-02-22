# ADR-0004: Generator Transport Contract in Config and Runtime

## Status
Accepted

## Context

Slice 11 introduces concrete generator transports (`claude -p` and OpenRouter). Before adapters are implemented, InfraFactory needs a stable, typed contract for:
- transport selection (`agent.type`)
- phase sequencing and delay semantics (`agent.phases`, `agent.phase_delay_seconds`)
- provider-specific required config and environment expectations

Without this contract, later adapter/runtime wiring can drift and produce inconsistent validation and runtime behavior.

## Decision

- Add explicit generator transport contract definitions in `internal/generator`:
  - supported agent types: `claude-code`, `openrouter`
  - supported canonical phases: `plan_architecture`, `generate_hcl`, `self_review`
  - required env/config-path metadata per transport
- Tighten config validation in `internal/config`:
  - reject unknown `agent.type`
  - reject negative `agent.phase_delay_seconds`
  - require `agent.phases` to match the canonical three-phase sequence in order
  - require `agent.claude.command` when `agent.type=claude-code`
  - require `agent.openrouter.model`, `agent.openrouter.base_url`, positive `timeout_seconds`, and non-negative `max_retries` when `agent.type=openrouter`
- Map resolved transport contract into runtime (`internal/cli/runtime.go`) so command paths have deterministic transport metadata.
- Extend root `infrafactory.yaml` example with `agent.claude` and `agent.openrouter` sections.

## Consequences

- Future transport adapters (`S11-T2`, `S11-T3`) can rely on a fixed config/runtime contract.
- Validation failures are deterministic and typed before transport execution.
- Existing config files remain compatible for `claude-code` because defaults provide `agent.claude.command`.
- `openrouter` now requires explicit model configuration, which makes the runtime contract explicit and testable.
