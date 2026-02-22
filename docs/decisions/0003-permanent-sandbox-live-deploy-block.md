# ADR-0003: Permanent Sandbox/Live Deploy Block

## Status
Accepted

## Context

`S9-T8` originally represented potential wiring for real Scaleway sandbox/live deploy orchestration.
The project has consistently treated this path as blocked because:

- real deploy operations introduce non-hermetic cost exposure;
- credential handling requirements are materially different from mock/static paths;
- default contributor and CI workflows are intentionally deterministic and local-first.

The active governance decision is now explicit: sandbox/live deploy support is not a deferred near-term implementation item; it is a permanent non-goal for this repository scope.

## Decision

InfraFactory permanently blocks real sandbox/live deploy orchestration in this repository.

Policy details:

- `validation.layers.sandbox_deploy.enabled` remains a blocked path that fails closed with deterministic output.
- Criteria requiring sandbox/live execution (for example `dns_resolution`) remain unsupported and are surfaced deterministically via support-matrix behavior.
- No ticket in the active roadmap should implement real sandbox/live deployment wiring unless this ADR is explicitly superseded.
- Any future change to this policy requires a new ADR that supersedes ADR-0003 and documents cost, credentials, and safety governance.

## Consequences

- The default project contract remains hermetic and reproducible for contributors and CI.
- Documentation can remove ambiguity around “temporarily deferred” wording and treat sandbox/live deploy as permanently out-of-scope.
- Backlog tracking keeps `S9-T8` as intentionally blocked by governance rather than pending implementation approval.
