# ADR-0011: Topology Derivation Layer

## Status
Accepted

## Context
The topology evaluator (`EvaluateTopology` in `internal/harness/topology.go`) expects pre-computed `connectivity` and `http_probe` maps in the mock state JSON. Mockway's `GET /mock/state` returns raw resource state — servers, LBs, frontends, backends, private NICs, RDB instances, etc. — but no topology maps. As a result, topology-dependent acceptance criteria (`http_probe`, `connectivity`) always fail because the expected keys don't exist in the state.

Two approaches were considered:

1. **Option A: Add topology computation to mockway** — mockway computes connectivity/http_probe maps and includes them in `GET /mock/state`. Rejected because it couples topology semantics to the mock server, requires duplicate work in fakegcp, and makes the mock harder to maintain.

2. **Option B: Add topology derivation to infrafactory** — a new `DeriveTopology` function in infrafactory transforms raw resource state into the topology structure the evaluator already consumes. Mockway and fakegcp stay simple (resource CRUD only).

## Decision
Add a topology derivation layer in infrafactory (Option B).

1. **New function `DeriveTopology(rawStateJSON) -> topologyJSON`** in `internal/harness/topology_derive.go`. Takes raw mockway/fakegcp state and produces `{"connectivity": {...}, "http_probe": {...}}`.

2. **Auto-detection in `EvaluateTopology`** — if the unmarshaled state has empty `connectivity` and `http_probe` maps, call `DeriveTopology` automatically. This is backward-compatible with pre-computed topology JSON (e.g., test fixtures).

3. **Derivation rules**:
   - `http_probe["load_balancer:PORT"]` = true when: LB exists + frontend on PORT + at least one backend + IP assigned.
   - `connectivity["compute->database:PORT"]` = true when: server and RDB instance share a private network.
   - `connectivity["compute->redis:PORT"]` = true when: server and Redis cluster endpoint share a private network (same pattern as RDB).
   - `connectivity["public_internet->database:PORT"]` = true only when RDB has a public endpoint.
   - `connectivity["public_internet->compute"]` = true only when server has a public IP.

4. **Mock backends unchanged** — mockway and fakegcp continue to return raw resource state. All topology intelligence lives in infrafactory.

## Consequences
**Benefits**:
- Single source of topology logic — easy to test, evolve, and debug.
- Mockway and fakegcp remain simple resource CRUD servers.
- New mock backends (fakegcp, future providers) get topology evaluation for free.
- Backward-compatible — existing test fixtures with pre-computed maps still work.

**Tradeoffs**:
- Topology derivation adds a processing step between state fetch and evaluation.
- Derivation rules must be kept in sync with mockway's resource structure (field names, endpoint formats).
- Edge cases in resource state (missing fields, unexpected formats) need defensive handling.

**Follow-up**:
- Verify all 12 training scenarios work with derived topology.
