# STATUS

Last updated: 2026-02-25

## Current phase
- Active milestone: Slice 20 — scenario combination expansion.
- All 6 existing scenarios pass `infrafactory run` on first iteration.
- All test suites green (`go test ./...`, mockway tests, `check_all.sh`).

## In progress
- None. Slice 20 complete (S20-T1 through S20-T6, all 6 first-iteration pass).

## Known blockers
- `S9-T8` sandbox/live deploy layer is permanently blocked by governance policy (ADR-0003).

## Next actions
1. Define next slice or maintenance work as needed.
2. `S9-T8` (sandbox/live deploy) remains permanently blocked by ADR-0003.

## Update policy
- Update at end of each meaningful coding session.
- Keep concise and factual.
- Move old detail to `docs/status/ARCHIVE.md`.
- Put durable architecture decisions in ADRs and `CONCEPT.md`.
- Keep startup/read-order instructions only in `SESSION_START.md` to avoid duplication.

## Recent updates
- **Slice 20 complete (S20-T1..S20-T6)**: 6 new scenarios exercising untested parameter combos:
  - `mysql-ha-paris`: mysql engine, medium DB, HA=true, private networking.
  - `compute-lb-multi-paris`: large compute (count=3), multi-backend LB (80/http + 443/tcp).
  - `k8s-medium-override-paris`: medium K8s with node_type/node_count overrides.
  - `private-lb-db-paris`: private LB, large PostgreSQL with node_type/engine_version overrides.
  - `public-registry-iam-paris`: is_public=true registry, IAM with policy=false.
  - `redis-xlarge-session-paris`: xlarge Redis with node_type override, xlarge compute.
  - Prompt fixes: LB backend/frontend zone pitfall, compute type mapping enforcement, phase1 exact-mapping enforcement.
  - Mockway fix: expanded server type catalog (GP1-L, GP1-XL, DEV1-L).
  - All 12 scenarios (6 existing + 6 new) pass on first iteration.
- **S19-T1 complete (round 4)**: Referential integrity and validation strictness:
  - **Delete cascades removed** — `DeleteLB`, `DeleteCluster`, `DeleteRDBInstance` now return 409 Conflict when dependents exist (per AGENTS.md contract). Exception: `lb_private_networks` cascade since the Scaleway provider doesn't detach them before LB delete.
  - **init_endpoints strict validation** — `BuildRDBEndpointsFromInit` rejects `private_network` with missing ID instead of silently falling back to public endpoint.
  - All 6 scenarios still pass on first iteration.
- **S19-T1 complete (round 2)**: Additional reliability fixes from extended review:
  - **IAM defaults not applied** — JSON Schema declared `default: true` for application/api_key/policy but Go's json.Unmarshal doesn't apply schema defaults. Fixed with `applyIAMDefaults()` that checks the raw YAML for omitted fields.
  - **LB/Frontend/Backend updates didn't persist** — same pattern as RDB update bug. Added `repo.UpdateLB()`, `repo.UpdateFrontend()`, `repo.UpdateBackend()` methods.
  - **LB list routes leaked cross-LB data** — `ListFrontends`/`ListBackends` ignored `lb_id` URL param. Added `ListFrontendsByLB`/`ListBackendsByLB` repo methods.
  - **RDB certificate endpoint didn't check instance existence** — returned 200 for nonexistent IDs. Now 404s.
  - **K8s companion test coverage** — added `scaleway_k8s_pool` auto-include test.
  - **Scenario test coverage** — added fixtures + tests for `iam`, `registry`, `redis`, `kubernetes` resource types and IAM default behavior.
  - All 6 scenarios still pass on first iteration.
- **S19-T1 complete (round 1)**: 3 bugs fixed (RDB update persistence, Redis missing fields).
- Completed Slice 18 (`S18-T1`..`S18-T5`): all 5 new scenarios pass on first iteration.
