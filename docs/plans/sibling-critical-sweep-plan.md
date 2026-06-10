# Arc: sibling CRITICAL sweep (finish v0.2 durability)

Status: planned (2026-06-10)
Owner: next-session claude (designed for autonomous execution)
Follows: `fakegenesys-v0.2-hardening-plan.md` (S123–S127 v0.2 hardening; contract audit shipped in all 4 siblings as empty-state for 3 of them).
Shape: 3-PR cross-repo sweep (6th instance of `reference_cross_repo_docs_sweep.md`).

## Big picture

S127 rolled out `handlers/contract_audit_test.go` to mockway, fakegcp, and fakeaws as **empty-state** — current handler surfaces had ~zero `CRITICAL[<id>]:` docstring tags. The audit passes today because there's nothing to enforce. The convention is "live for future contracts but unenforced on existing surface."

This arc closes that gap by bridging the **known wire-shape invariants** in each sibling into the `CRITICAL[<id>]:` / `TestContract_<id>` form. After this arc, each sibling's audit reports `>0 contracts paired` — the durability story is real, not aspirational.

### Scope discovery (2026-06-10 pre-flight)

A literal `grep -nE 'CRITICAL[^[]|MUST[^[]' handlers/*.go` across the 3 siblings finds almost nothing pre-existing (mockway + fakegcp = zero; fakeaws = 1 `MUST` in rds.go). The `CRITICAL:` / `MUST:` framing wasn't the convention before S127 — invariants lived in informal docstrings, in test names, or only in MEMORY.md's "Key Bug Patterns" list.

**So the sweep is BROADER than literal grep**: identify invariants from THREE signals, paired with existing tests where possible:

1. **MEMORY.md "Key Bug Patterns"** — documented historic regressions (mockway LB `ip_ids` array vs deprecated string; mockway Redis endpoint default port 6379; fakeaws KMS soft-delete; etc).
2. **Existing test names that already lock in a wire-shape invariant** (e.g. mockway `TestRedis_EndpointPortDefault` would convert to `TestContract_redis_endpoint_port_6379` + a paired `CRITICAL[redis-endpoint-port-6379]:` docstring on the Redis handler).
3. **Docstring patterns that imply a contract** (e.g. "the Scaleway Terraform provider expects ...", "default to X in Create + Set + Update", "must round-trip").

The per-sibling matrix populates from these signals, not from a literal CRITICAL/MUST grep. Same three coverage bars from `feedback_test_coverage_metrics.md` apply.

## Slices

| Slice | Title | Effort |
|---|---|---|
| S128 | mockway: bridge existing wire-shape invariants into CRITICAL[id]: form | ~1.5–2 hr |
| S129 | fakegcp: same | ~1–1.5 hr |
| S130 | fakeaws: same + the existing rds.go:23 MUST note | ~1–1.5 hr |

(Slice numbers continue from S127 in the v0.2 hardening arc. Each slice = 1 PR.)

**Total**: ~3.5–5 focused hours.

## Standing rules

Inherit from `fakegenesys-v0.2-hardening-plan.md` + `feedback_test_coverage_metrics.md`:

- **Don't pad.** If a sibling genuinely has zero documented wire-shape contracts, document that explicitly in its PR description and ship a near-empty PR (just a comment in the audit's docstring saying "current scope: 0 contracts; sibling reserves the convention for future additions"). DO NOT invent contracts to hit a row count.
- **Per-contract matrix in each PR description** — same shape as S123's matrix.
- **Existing test rename strategy**: if a test already covers an invariant, rename it to `TestContract_<id>` (e.g. `TestLB_IPIDsArray` → `TestContract_lb_ip_ids_array`). Don't add a duplicate test; rename + add the docstring tag on the handler.
- **Naming**: kebab-case contract IDs. Lead with the resource family (`redis-`, `lb-`, `kms-`, `rds-`) for greppability.
- Codex anti-nitpick (`feedback_codex_anti_nitpick.md`).
- Never hand-edit `pitfalls/*.yaml` (stash before commits).

## S128 — mockway

### Motivation

Mockway is the oldest sibling and has the deepest test surface (9k lines of tests). Several known regressions live in MEMORY.md's "Key Bug Patterns" but have no `CRITICAL[id]:` tags. Convert what's clearly contractual.

### Inventory (estimated; verify on read)

| Contract candidate | Source signal | Action |
|---|---|---|
| `lb-ip-ids-array` | MEMORY + `handlers_test.go:2466` ("Create an LB using ip_ids (array) instead of ip_id (string)") + `instance.go:494` docstring | Add `CRITICAL[lb-ip-ids-array]:` on LB handler + paired `TestContract_lb_ip_ids_array` (likely rename of existing) |
| `redis-endpoint-port-6379` | MEMORY + `handlers_test.go:3414, 5177, 5189, 5217, 5226` (multiple assertions on port default) | Add `CRITICAL[redis-endpoint-port-6379]:` on Redis Create + Set + Update paths + `TestContract_redis_endpoint_port_6379` (rename or wrap existing) |
| `k8s-version-stability` | MEMORY "K8s version oscillation: LLM gets stuck alternating configs. Needs static pitfall." | This is an LLM-side pitfall, NOT a mock-server contract. SKIP — not a wire-shape invariant. |
| `dns-zone-prerequisite` | MEMORY "DNS zone prerequisite: LLM creates record without zone." | Also LLM-side. SKIP. |

Expected final size: 2 contracts (LB + Redis).

### Tickets

| ID | Description | Priority |
|---|---|---|
| S128-T1 | Read `handlers/lb.go` + locate the handler that emits LB body. Add `CRITICAL[lb-ip-ids-array]:` docstring naming the array-shape invariant. Find existing test in `handlers_test.go` covering this; rename or add `TestContract_lb_ip_ids_array`. | P0 |
| S128-T2 | Read `handlers/redis.go`. Add `CRITICAL[redis-endpoint-port-6379]:` on Create/Set/Update handlers (or on the helper that defaults the port). Find existing test; rename or add `TestContract_redis_endpoint_port_6379`. | P0 |
| S128-T3 | Run `go test ./handlers/ -run 'TestContract_|TestAllContractsHaveTests'` — audit must report `2 contracts paired`. | P0 |
| S128-T4 | Single PR. Title: `mockway: bridge LB ip_ids + Redis port invariants into CRITICAL[id]: form`. Embed contract matrix. | P0 |

### Exit criteria

- `TestAllContractsHaveTests` reports >0 contracts paired (target: 2).
- Reverting either CRITICAL note breaks its paired test.
- CI green; PR squash-merged.

## S129 — fakegcp

### Motivation

fakegcp has documented M-ticket bug fixes (M44 DNS, M45 GKE, M47 Storage, M49 numericID overflow) and a "documented inline fidelity gaps in `fk_violation_test.go`" entry in MEMORY. Plus existing `repository_test.go` exercises FK + cascade behaviors that ARE contractual.

### Inventory (estimated; verify on read)

| Contract candidate | Source signal | Action |
|---|---|---|
| `pubsub-fk-cascade-on-topic-delete` | Existing FK violation tests; provider's iterative read expects cascade | Convert |
| `numeric-id-no-overflow` | M49 fix — IDs must fit `string` (not `int64` w/ overflow) | Convert if it has a handler-level docstring; else skip |
| `dns-records-after-zone-delete-404` | M44 fix | Convert |
| Possible: `storage-bucket-conditional-create` | M47 | Convert if visible in handler docstring |

Expected final size: 2-4 contracts. Will narrow during T1 read.

### Tickets

| ID | Description | Priority |
|---|---|---|
| S129-T1 | Inventory pass: grep `handlers/*.go` for docstrings referencing M-tickets + read MEMORY's "Follow-up Items" for `fk_violation_test.go` gaps. Build the actual matrix (could be 1-4 rows). | P0 |
| S129-T2 | Per row: add CRITICAL[id]: docstring + paired TestContract_<id>. | P0 |
| S129-T3 | `go test ./handlers/...` audit must report >0 paired. | P0 |
| S129-T4 | Single PR. | P0 |

### Exit criteria

Same as S128 but per fakegcp.

## S130 — fakeaws

### Motivation

fakeaws has 1 pre-existing real `MUST` (rds.go:23 — DbiResourceId distinct from identifier) + documented KMS soft-delete invariant (S106) + the S43–S48 wire-shape parity that survived 17 codex review passes.

### Inventory (estimated; verify on read)

| Contract candidate | Source signal | Action |
|---|---|---|
| `rds-dbi-resource-id-distinct-from-identifier` | Existing MUST at rds.go:23 + the `dbiResourceIDFor` helper docstring | Direct conversion — add `[id]` tag + paired test |
| `kms-soft-delete-state-pending-deletion` | S106 / MEMORY "Last arc complete: fakeaws KMS soft-deletes (state=PendingDeletion, DescribeKey returns 200) matching real AWS lifecycle" | Convert |
| `secrets-manager-soft-delete` | S89 (same pattern as KMS, fixed earlier) | Convert if visible in docstring |
| `route53-records-sorted-before-maxitems-filter` | S96 (fakeaws#7) | Convert if visible in handler |

Expected final size: 2-4 contracts.

### Tickets

| ID | Description | Priority |
|---|---|---|
| S130-T1 | Convert `rds.go:23` MUST to `MUST[rds-dbi-resource-id-distinct-from-identifier]:`. Add `TestContract_rds_dbi_resource_id_distinct_from_identifier`. | P0 |
| S130-T2 | Read `handlers/kms.go` (S106 fix). Add `CRITICAL[kms-soft-delete-state-pending-deletion]:` on the relevant handler + paired test. | P0 |
| S130-T3 | Scan `handlers/secretsmanager.go` + `handlers/route53.go` for the S89/S96 fix sites. Convert if the invariant is clearly contractual. | P1 |
| S130-T4 | `go test ./handlers/...` audit must report >0 paired. | P0 |
| S130-T5 | Single PR. | P0 |

### Exit criteria

Same as S128 but per fakeaws.

## Out of scope

- Inventing contracts to pad the matrix. If a sibling genuinely has zero documented wire-shape invariants worth locking in, that's the correct outcome — document it and move on.
- Backfilling CRITICAL[id]: tags on EVERY handler docstring. Only the ones that are real wire-shape contracts (revert → provider breaks).
- Adding NEW invariants discovered during this arc — that belongs in a future coverage-growth arc, not here.

## Arc close-out

S130-T5 (final fakeaws PR) folds in:
- Update `infrafactory/STATUS.md` to mark sweep complete
- Append entry to `docs/status/ARCHIVE.md`
- Update `MEMORY.md` "Latest" entry
- Tag this as S128–S130 (not a new release; no fakegenesys-equivalent tag needed since these are sibling-side)
