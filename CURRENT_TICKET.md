# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: none
- title: No active ticket — Slices 1-20 complete
- status: idle
- classification: n/a

## 1) Problem Statement
- What is broken or missing?
  Slice 20 scenario combination expansion is complete. 6 new scenarios created and all pass on first iteration.
- Why does it matter now?
  Coverage expanded from 6 to 12 training scenarios covering all schema parameter combinations.

## 2) Scope
- In scope: Slice 20 complete.
- Out of scope: n/a.

## 3) Acceptance Criteria
1. All 6 new scenarios pass `infrafactory run` on first iteration. DONE
2. All 6 existing scenarios still pass (no regressions). DONE
3. `go test ./...`, mockway tests, and `bash scripts/check_all.sh` all pass. DONE

## Progress notes

### S20-T1 (complete)
- `mysql-ha-paris.yaml`: first-iteration pass. MySQL engine, medium DB, HA=true, private networking.

### S20-T2 (complete)
- `compute-lb-multi-paris.yaml`: first-iteration pass after prompt fixes.
- Added pitfalls: LB backend/frontend zone argument, assign_flexible_ipv6 conflict, compute type mapping.
- Strengthened phase1 prompt to enforce exact size mappings from mappings.yaml.

### S20-T3 (complete)
- `k8s-medium-override-paris.yaml`: first-iteration pass. Medium K8s with node_type/node_count overrides.

### S20-T4 (complete)
- `private-lb-db-paris.yaml`: first-iteration pass. Private LB, large PostgreSQL with overrides.

### S20-T5 (complete)
- `public-registry-iam-paris.yaml`: first-iteration pass. Public registry, IAM with policy=false.

### S20-T6 (complete)
- `redis-xlarge-session-paris.yaml`: first-iteration pass after mockway server type catalog expansion.
- Added GP1-L, GP1-XL, DEV1-L to mockway instance type catalog and marketplace compatible types.
- Rebuilt Docker.

## Blocker (if any)
- blocker: none.
