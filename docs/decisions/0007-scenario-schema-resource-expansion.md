# ADR-0007: Scenario Schema Resource Expansion (Slice 18)

## Status
Accepted

## Context
Slice 18 expanded mockway to cover 5 additional Scaleway API surfaces (K8s standalone, IAM, Container Registry, Redis, Composite). To support scenarios for these services, `scenario.schema.json` and the Go `Scenario` struct needed new resource type definitions.

The schema's `resources` object had `additionalProperties: false`, meaning any new resource type required explicit schema+struct additions before scenarios could validate.

## Decision
Extend `scenario.schema.json` with four new resource definitions under `resources`:
- `kubernetes` — cluster size + node override (type, count)
- `iam` — purpose + boolean flags (application, api_key, policy) with schema defaults of `true`
- `registry` — purpose + is_public flag
- `redis` — purpose + size + node override

Corresponding Go structs added to `internal/scenario/scenario.go`: `KubernetesResource`, `IAMResource`, `RegistryResource`, `RedisResource`.

IAM boolean defaults (`application`, `api_key`, `policy` all default to `true`) are declared in the JSON Schema but applied at runtime via `applyIAMDefaults()` because Go's `json.Unmarshal` does not honor JSON Schema defaults.

## Consequences
- Scenarios can now declare intent for K8s, IAM, Registry, and Redis resources using the same size-mapping pattern as compute/database.
- `additionalProperties: false` on `resources` remains enforced — future resource types require another schema extension.
- The `applyIAMDefaults` workaround is specific to IAM booleans; if other resource types need schema defaults in the future, the same pattern applies.
- Size mappings in `mappings.yaml` were extended with Redis tiers (RED1-MICRO, RED1-S, RED1-M, RED1-L).
