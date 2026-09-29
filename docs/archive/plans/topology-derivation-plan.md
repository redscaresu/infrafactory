# Plan: Topology Derivation from Raw Mock State (Slice 31)

## Context

The topology evaluator expects pre-computed `connectivity` and `http_probe` maps, but mockway returns raw resource state. Topology-dependent acceptance criteria (`http_probe`, `connectivity`) fail because the expected keys don't exist. This affects scenarios like `compute-lb-multi-paris` where `http_probe["load_balancer:80"]` always returns false.

ADR-0011 decides to fix this in infrafactory (not mockway), keeping mock servers as simple resource CRUD.

## Quick Reference

| Key | Value |
|---|---|
| Slice | 31 |
| Ticket IDs | S31-T1 through S31-T6 |
| Depends on | Slice 30 (done) |
| ADR | 0011 (topology derivation layer) |
| Core file | `internal/harness/topology_derive.go` (new) |

## Architecture

```
  Mockway GET /mock/state
         |
         v
  Raw resource state JSON
  {instance: {servers, nics}, lb: {lbs, frontends, backends, ips}, rdb: {instances}, ...}
         |
         v
  DeriveTopology()                    <-- NEW (Slice 31)
  Walks raw resources, computes:
  - http_probe: LB + frontend(port) + backend + IP = reachable
  - connectivity: server NIC + RDB endpoint on same private network = connected
         |
         v
  Derived topology JSON
  {connectivity: {"compute->database:5432": true}, http_probe: {"load_balancer:80": true}}
         |
         v
  EvaluateTopology()                  <-- EXISTING (unchanged interface)
  Checks each acceptance criterion against the topology maps
         |
         v
  Pass / Fail per criterion
```

## Resource Relationship Diagram

```
  http_probe path:
  ┌──────────┐    ┌────────────┐    ┌───────────┐    ┌────────┐
  │  LB IP   │───>│  Frontend  │───>│  Backend  │───>│ Server │
  │ (lb_id)  │    │ (lb_id,    │    │ (lb_id)   │    │        │
  │          │    │  inbound_  │    │           │    │        │
  │          │    │  port=80)  │    │           │    │        │
  └──────────┘    └────────────┘    └───────────┘    └────────┘
  state.lb.ips    state.lb.          state.lb.        state.instance.
                  frontends          backends          servers

  connectivity path (private network):
  ┌────────┐    ┌────────────┐    ┌─────────────────┐    ┌──────────────┐
  │ Server │───>│ PrivateNIC │───>│ Private Network │<───│ RDB Endpoint │
  │        │    │ (server_id,│    │ (id)            │    │ (private_    │
  │        │    │  pn_id)    │    │                 │    │  network.id) │
  └────────┘    └────────────┘    └─────────────────┘    └──────────────┘
  state.instance. state.instance.   state.vpc.            state.rdb.
  servers         private_nics      private_networks      instances
                                                          [].endpoints[]
```

## Mockway State Fields Used

| Path | Join field | Used for |
|------|-----------|----------|
| `state.lb.lbs[].id` | — | LB existence |
| `state.lb.frontends[].lb_id` | matches `lb.id` | Frontend on this LB |
| `state.lb.frontends[].inbound_port` | — | Port check (float64→int) |
| `state.lb.backends[].lb_id` | matches `lb.id` | Backend exists for LB |
| `state.lb.ips[].lb_id` | matches `lb.id` | LB has public IP |
| `state.instance.servers[].id` | — | Server existence |
| `state.instance.private_nics[].server_id` | matches `server.id` | Server's NIC |
| `state.instance.private_nics[].private_network_id` | — | NIC's network |
| `state.instance.ips[].server.id` | matches `server.id` | Server has public IP |
| `state.rdb.instances[].endpoints[].private_network.id` | matches NIC's `pn_id` | RDB on same network |
| `state.rdb.instances[].endpoints[].port` | — | RDB port (float64→int) |
| `state.redis.clusters[].endpoints[].private_network.id` | matches NIC's `pn_id` | Redis on same network |
| `state.redis.clusters[].endpoints[].port` | — | Redis port (float64→int) |

## Derivation Rules

### http_probe

`http_probe["load_balancer:PORT"]` = true when ALL of:
1. At least one LB exists in `state.lb.lbs`
2. LB has a frontend in `state.lb.frontends` with `inbound_port == PORT`
3. LB has at least one backend in `state.lb.backends`
4. LB has an IP in `state.lb.ips`

### connectivity (same private network)

`connectivity["compute->database:PORT"]` = true when:
1. At least one server exists in `state.instance.servers`
2. Server has a private NIC in `state.instance.private_nics` on a private network
3. RDB instance in `state.rdb.instances` has an endpoint with `private_network.id` matching that same private network
4. RDB endpoint port matches PORT

`connectivity["compute->redis:PORT"]` = true when:
1. Server has a private NIC on a private network
2. Redis cluster in `state.redis.clusters` has an endpoint with `private_network.id` matching that same private network (same pattern as RDB — the provider stores `private_network` objects in `endpoints[]`)

### connectivity (public internet)

`connectivity["public_internet->database:PORT"]` = true when:
- RDB instance has a public endpoint (endpoint has no `private_network`)

`connectivity["public_internet->compute"]` = true when:
- Any server has a public IP (check `state.instance.ips` where `server.id` matches)

## Tickets

### S31-T1: ADR-0011 + CONCEPT.md + BACKLOG/ROADMAP (DONE)

### S31-T2: Implement `DeriveTopology()`

**New file**: `internal/harness/topology_derive.go`

```go
type rawMockState struct {
    Instance struct {
        Servers     []map[string]any `json:"servers"`
        IPs         []map[string]any `json:"ips"`
        PrivateNICs []map[string]any `json:"private_nics"`
    } `json:"instance"`
    LB struct {
        LBs       []map[string]any `json:"lbs"`
        IPs       []map[string]any `json:"ips"`
        Frontends []map[string]any `json:"frontends"`
        Backends  []map[string]any `json:"backends"`
    } `json:"lb"`
    VPC struct {
        PrivateNetworks []map[string]any `json:"private_networks"`
    } `json:"vpc"`
    RDB struct {
        Instances []map[string]any `json:"instances"`
    } `json:"rdb"`
    Redis struct {
        Clusters []map[string]any `json:"clusters"`
    } `json:"redis"`
}
```

- `DeriveTopology(stateJSON []byte) ([]byte, error)` — main function
- Helper functions: `deriveHTTPProbe()`, `deriveConnectivity()`
- `jsonInt(v any) int` — safely converts `float64` to `int` for port comparison
- `jsonStr(m map[string]any, key string) string` — safe string extraction
- Note: `instance.ips[]` entries have a nested `server` object (`ip["server"].(map[string]any)["id"]`) — handle with two-level map access
- Defensive: missing fields → skip (don't crash), empty state → empty maps

### S31-T3: Wire into `EvaluateTopology()`

**Modify**: `internal/harness/topology.go`

Auto-detect raw state: if `connectivity` and `http_probe` maps are both `nil` after unmarshal (keys absent from JSON, not empty `{}`), call `DeriveTopology` on the raw bytes. Backward-compatible with pre-computed topology JSON.

### S31-T4: Unit tests + fixtures

**New files**:
- `internal/harness/topology_derive_test.go`
- `internal/harness/testdata/topology/raw_state.json` (web-app scenario)
- `internal/harness/testdata/topology/raw_state_no_lb.json`
- `internal/harness/testdata/topology/raw_state_public_db.json`

**Test cases** (8+):
1. Full web-app: http_probe true, compute→database true, public_internet→database false
2. No LB: empty http_probe map
3. LB no backend: http_probe false
4. LB no IP: http_probe false
5. Public database: public_internet→database true
6. Empty state: empty maps, no crash
7. MySQL port 3306: correct port derivation
8. Redis connectivity: compute + redis on same private network derives `connectivity["compute->redis:6379"] = true`
9. End-to-end: raw state through `EvaluateTopology` auto-derivation

### S31-T5: Playwright e2e — topology results on Live page

The Live page iteration timeline already shows failure details. After topology derivation works, verify that:
- A successful topology check shows in the stage pills
- A failed topology check shows the retry reason with the specific check detail

### S31-T6: Integration verification

Run `infrafactory test` against each training scenario with topology criteria. Verify:
- `web-app-paris`: http_probe + connectivity all pass
- `compute-lb-multi-paris`: http_probe passes (currently fails)
- `mysql-ha-paris`: connectivity passes
- No regressions in policy/destruction checks

## Execution Order

```
S31-T1 (docs) --------- DONE
S31-T2 (derive logic) -- next
S31-T3 (wire) --------- depends on T2
S31-T4 (tests) -------- depends on T2+T3 (can parallel with T5)
S31-T5 (e2e) ---------- depends on T3 (can parallel with T4)
S31-T6 (integration) -- last
```

## Verification

```bash
make test                    # Go + UI unit + Playwright e2e
go test ./internal/harness   # Topology derivation unit tests
```

## Out of Scope

- Modifying mockway or fakegcp (they stay as-is per ADR-0011)
- Security group rule evaluation (simplified: assume allow-all within private network)
- DNS topology (dns_resolution uses real probes in Layer 3, auto-pass in Layer 2)
