# Arc: fakegenesys — Genesys Cloud CCaaS mock + infrafactory integration

Status: complete (S115 close-out — 2026-06-06)
Owner: next-session claude (designed for autonomous execution)
Follows: `fakeaws-kms-soft-delete-plan.md` (closed 2026-06-05 with S106) and `sustain-under-renamed-vocab-plan.md` (closed 2026-06-05 with S105). S107 reserved-but-shelved per `docs/plans/pitfall-pruning-automation-plan.md`.
Shape: goal-named variable-length arc per AGENTS.md (8 slices, ~5-7 days focused effort / ~4-6 weeks calendar)

## Big picture — why this arc

Build a fourth sibling mock for the Genesys Cloud CCaaS Terraform provider (`mypurecloud/genesyscloud`). Three reasons:

1. **No existing OSS fake**. Mockway / fakegcp / fakeaws all replaced LocalStack / similar; nothing comparable exists for Genesys Cloud. Genuine novelty.
2. **Generalizes infrafactory beyond IaaS**. The system has shipped against three IaaS providers (AWS / GCP / Scaleway). A SaaS / CCaaS provider proves the architecture isn't accidentally tied to networking primitives.
3. **Simpler wire format**. Pure REST/JSON, single auth mechanism (OAuth2 client_credentials → Bearer). No fakeaws-style 5-protocol dispatcher; no GCP-style operations model; no Scaleway-style nested-page list response.

Mature scope: ~15 resources balanced across identity / routing / architect categories. 5 training scenarios passing deterministically in a sustain sweep. OSS-ready from day one (parity with the three existing sibling repos). infrafactory dispatches to fakegenesys as the 4th cloud — `make sweep-N` includes the new scenarios.

## Win condition

A `make sweep-N` (where N is the expanded scenario count post-integration) sustains three consecutive times with all 5 genesys training scenarios in `target_reached`. This is the same baseline pattern the other three clouds passed before they were considered shipped.

## What this arc tests in the learning system

Distinct from the fakegenesys deliverable itself: **this is the strongest end-to-end test of the auto-learning loop the project has ever run.** The other three clouds (AWS / GCP / Scaleway) have years of accumulated pitfalls — every sustain sweep there validates the loop's *stability* on a pre-warmed corpus. Genesys starts at zero (`pitfalls/genesys.yaml` is empty per S114-T2). The arc tests:

1. **Cold-start prompt sufficiency.** Can Claude write working Genesys HCL from just the phase-2 prompt + provider docs, with no learned pitfalls? If sweep 1 shows all 5 genesys scenarios in `target_reached`, the answer is yes — and the descriptive/fix/avoid extractors have nothing to fire on. If sweep 1 has failures, the next two sweeps are where the learning loop gets exercised.
2. **Cold-start learning velocity.** When sweep 1 fails on (say) 3 of 5 genesys scenarios, the loop should: (a) emit pitfalls into `pitfalls/genesys.yaml` (likely `descriptive` initially, possibly `fix` or `avoid` if iter N+1 self-corrected), (b) preserve `avoid` entries through `bin/pitfall-merge --keep avoid`, (c) load those entries on sweep 2 prompts so failures don't repeat. Sweep 2 should show measurable improvement over sweep 1.
3. **Mock-server-bug classifier on new ground.** The `IsMockServerBug` substring list (`internal/generator/pitfalls_learn.go::mockActionableSignals`) was tuned for AWS/GCP/Scaleway. Genesys failures may surface shapes that should-be-mock-actionable but aren't classified — those would leak into `pitfalls/genesys.yaml` as `descriptive` entries the `TestPitfallsNoMockServerBugSeeds` ratchet would flag. Watch for this in S115.
4. **Topology derivation on a non-IaaS cloud.** S114-T6 builds a derivation for queue-member / flow-queue graphs (no networking primitives). If the topology check fails on a scenario whose other assertions pass, the derivation needs widening — same fix-forward shape as any other rename-era discovery.

**Implication for sweep results:** sweep 1 may legitimately have genesys failures and that's diagnostic data, not arc failure. The structural win condition (5/5 + 39/39 in all 3 sweeps) is still the gate, but a sweep-1-fails / sweep-2-improves / sweep-3-clean trajectory is itself a positive signal — it proves the loop closes from cold start.

Track per-sweep in S115-T1:
- `pitfalls/genesys.yaml` line count delta (sweep N→N+1 should grow then plateau).
- `AVOID_EMISSIONS=N` from the genesys cloud specifically (separate from the aggregate count — `bin/pitfall-merge` reports per-cloud).
- Any `IsMockServerBug`-classified failures routed to `docs/mock-gaps.md` with `discovered_from: genesys-*` — these are genuine fakegenesys fidelity gaps to fix at source mid-arc (same as the S106 KMS soft-delete recovery from S105).

## Standing rules inherited

- **Option C arc shape**: goal-named, variable-length, close-out folded into the final slice.
- **Fix at source**: any mock-side bug surfaced during infrafactory integration goes back to fakegenesys, never hand-edited into `pitfalls/genesys.yaml`.
- **Smoke harness pattern (now canonical, see AGENTS.md cross-link)**: every resource handler ships with `examples/working/<resource>/` + `examples/updates/<resource>/` + `examples/misconfigured/<resource>/` directories. The real provider binary validates wire shape; no real Genesys tenant needed.
- **Per-bundle PR rule**: handler + tests + examples (all three trees) + scenario anchors + coverage_matrix updates land in one PR per slice.
- **Renamed-vocab consistency** (post-S104): any pitfall entries that surface organically during integration use `descriptive` / `fix` / `avoid` source values.
- **Pitfall ratchets**: `TestPitfallsNoHumanSeeding`, `TestPitfallsNoMockServerBugSeeds`, `TestPitfallsNoOPADuplication`, `TestPitfallsSourceEnum` all apply to the new `pitfalls/genesys.yaml`.

## Fidelity strategy — spec-driven, not reactive

The three existing fakes diverge on how they discover wire shapes:

- **mockway** uses Scaleway's OpenAPI specs (downloaded into `specs/`) as the primary source of truth, cross-references handler routes against them, and has an explicit "Reverse fidelity — don't over-correct" rule: never add validation the real API doesn't enforce. Verify against the spec first.
- **fakegcp** uses GCP discovery docs informally but doesn't institutionalize them as a tree.
- **fakeaws** declined Smithy codegen and built reactively via `TF_LOG=DEBUG` capture of provider HTTP traffic. Slower per-resource; works because AWS APIs are well-documented in the SDK source.

**fakegenesys adopts mockway's pattern**, not fakeaws's. Genesys Cloud publishes a comprehensive OpenAPI spec at `https://api.mypurecloud.com/api/v2/docs/swagger` (Swagger 2.0 JSON, ~5MB). This is the primary source of truth for every handler.

**Per-slice contract**:

1. Before writing a handler in S109/S110/S111, download the relevant OpenAPI section into `specs/<resource>.json` (or a single `specs/genesys-openapi.json` filtered to the endpoints we implement).
2. Build the handler against the spec, not against guessed shapes.
3. **Reverse-fidelity rule applies**: never enforce constraints (required fields, format validators, cascade blockers) the OpenAPI spec doesn't declare. If unsure, omit the validation — the real provider will reject genuinely-bad configs before they reach fakegenesys.
4. Add a small static check in S108: `examples/spec_cross_reference_test.go` walks `specs/` + the handler route map and asserts every implemented route exists in the spec. Catches typos and stale handlers.
5. The provider smoke harness (`examples/provider_smoke_test.go`) remains the correctness gate — but it's now the *second* line of defense, not the first.

## Anti-nitpick rule (S112 / S113 codex review)

Codex review loops are a real correctness tool but they will produce style nitpicks indefinitely. **Filter:**

- ✅ **Act on**: wire-shape correctness, missing test coverage that hides a behavioral assumption, broken auth/security, missing 404 fidelity, response-field-shape mismatch with the provider's expectations, idempotency violations, FK integrity gaps.
- ❌ **Ignore**: "could be more idiomatic", "consider renaming X to Y for clarity", "this comment could be tightened", repeat findings on patterns that match the other three siblings' conventions, suggestions to add tests that don't pin a behavior we'd accept changing later.

Stop iterating when codex returns either (a) two consecutive `NOTHING_TO_IMPROVE`, or (b) two consecutive passes where every finding is in the "Ignore" category. **Document the ignore-rationale in `docs/review-passes/passN.md` so future-me sees why specific findings were declined.**

## Slice table

| Slice | Title | Effort | PRs |
|---|---|---|---|
| S108 | Repo scaffold + OSS-mature layout + OAuth2 + smoke harness | ~3-4 hr | fakegenesys (new repo) |
| S109 | Identity resources (5) — `user`, `group`, `location`, `role`, `oauth_client` | ~6-8 hr | fakegenesys |
| S110 | Routing resources (5) — `queue`, `skill`, `wrapupcode`, `language`, `utilization` | ~6-8 hr | fakegenesys |
| S111 | Architect + responsemanagement (5) — `architect_datatable`, `architect_user_prompt`, `flow`, `responsemanagement_response`, `idp_generic` | ~10-12 hr | fakegenesys |
| S112 | Codex review pass 1 — broad triage | ~4-6 hr | fakegenesys |
| S113 | Codex review pass 2 (if needed) OR consolidation + final OSS polish | ~2-4 hr | fakegenesys |
| S114 | infrafactory integration — prompts, policies, scenarios, dispatch wiring, topology | ~8-10 hr | infrafactory |
| S115 | Sustain sweep + sibling-repo cross-link blurbs + arc close-out | ~4-6 hr | infrafactory + 3 sibling cross-link PRs |

**Total: ~43-58 hr focused / ~5-7 working days / ~4-6 calendar weeks.**

## Per-cloud reference table (for slice context)

| Property | Value |
|---|---|
| Provider namespace | `mypurecloud/genesyscloud` |
| Provider version | Latest stable; pin at S108-T1 (e.g. `~> 1.55`) |
| API base | `https://api.mypurecloud.com` (US East default; configurable via env or provider region attribute) |
| Auth | OAuth2 client_credentials grant → Bearer token. Mock issues tokens at `POST /oauth/token` and verifies on every other route. |
| Wire format | REST/JSON only. No XML, no protocol multiplexing. |
| Pagination | Query string `pageNumber` + `pageSize`; response `{ entities: [...], pageCount, pageNumber, pageSize, total }`. |
| Default mock port | `:8083` (next available after fakeaws's `:8082`) |
| State backend | SQLite repository (mirror fakeaws layout) — `repository/repository.go` |
| Smoke gate env var | `FAKEGENESYS_ENABLE_E2E=1` |

---

## S108 — Repo scaffold + OSS-mature layout + OAuth2 + smoke harness

### Motivation

Establish the fakegenesys repo with OSS parity to the three existing siblings (Apache-2.0 + SECURITY + CONTRIBUTING + CODE_OF_CONDUCT + CHANGELOG + release workflow + branch protection + gitleaks pre-commit) and the canonical provider smoke harness skeleton from day one. OAuth2 client_credentials grant is the auth that everything else depends on, so it's S108 not later.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S108-T1 | Create `../fakegenesys/` repo. Initial commit with Apache-2.0 LICENSE. Pin `mypurecloud/genesyscloud` provider version (check latest stable; bake into examples). | P0 | — |
| S108-T2 | OSS-ready layout (mirror fakeaws): `SECURITY.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `CHANGELOG.md` (Keep a Changelog format with v0.1.0 unreleased section), `.gitleaks.toml` (allowlist `examples/.*\.tf$`), `.pre-commit-config.yaml`, `scripts/install-hooks.sh`. | P0 | T1 |
| S108-T3 | `.github/workflows/ci.yaml` (lint + build + test + gitleaks jobs), `.github/workflows/release.yaml` (semantic-release-style on tag push). Mirror fakeaws workflows. | P0 | T2 |
| S108-T4 | Go module: `go.mod` (`github.com/redscaresu/fakegenesys`), `go.sum`. Standard deps: `chi/v5`, `mattn/go-sqlite3`, `stretchr/testify`. | P0 | T1 |
| S108-T5 | `cmd/fakegenesys/main.go` — CLI flags (`--port` default `8083`, `--db` default `:memory:`), HTTP server bootstrap, graceful shutdown. | P0 | T4 |
| S108-T6 | `handlers/handlers.go` — chi router scaffold; OAuth Bearer auth middleware applied to all routes except `POST /oauth/token` and `/mock/*`; admin routes (`/mock/reset`, `/mock/snapshot`, `/mock/restore`, `/mock/state`, `/mock/state/{service}`). | P0 | T5 |
| S108-T7 | `handlers/oauth.go` — `POST /oauth/token` accepts `grant_type=client_credentials` + `client_id` / `client_secret` (form-encoded per OAuth2 RFC 6749). Issues a Bearer token (UUID) with `expires_in: 3600`. In-process token store with TTL; `/mock/reset` purges. The Bearer middleware validates the token from `Authorization: Bearer <token>` header. | P0 | T6 |
| S108-T8 | `handlers/oauth_test.go` — pin: (a) token issued on valid client_credentials, (b) 401 on missing/invalid Bearer for protected routes, (c) `/mock/reset` invalidates tokens, (d) malformed grant_type rejected. | P0 | T7 |
| S108-T9 | `models/errors.go` — `ErrNotFound`, `ErrConflict`. `models/types.go` — Genesys API response wrappers (`PagedResponse[T]` with `Entities`, `PageCount`, `PageNumber`, `PageSize`, `Total`). | P0 | T4 |
| S108-T10 | `repository/repository.go` — SQLite-backed state. `Reset()`, `Snapshot()`, `Restore()`, `FullState()`, `ServiceState(name)`. In-process cache registration (`Cache` interface mirroring fakeaws). Tables created in `init()` migration. | P0 | T4 |
| S108-T11 | `testutil/testutil.go` — `NewTestServer(t)` returns httptest.Server with in-memory SQLite + a pre-issued Bearer token. Helpers: `DoCreate`, `DoGet`, `DoList`, `DoPatch`, `DoDelete`. Mirror fakeaws's signature so tests look familiar. | P0 | T7, T10 |
| S108-T12 | `examples/provider_smoke_test.go` — auto-discovery harness gated by `FAKEGENESYS_ENABLE_E2E=1`. Three walkers for `examples/{working,misconfigured,updates}/`. Per-test fresh-port fakegenesys binary spawn (mirror fakegcp's approach since it isolates state). Rewrite `localhost:8083` in `providers.tf` to per-test port. | P0 | T5 |
| S108-T13 | `examples/known_broken.yaml` (empty initially), `examples/README.md` (explains the three trees). | P0 | T12 |
| S108-T14 | `README.md` — scaffold with one-line description, Apache-2.0 badge, quickstart (`make build && ./fakegenesys --port 8083`), link to AGENTS.md. **No marketing fluff.** Mirror fakeaws's README structure. | P0 | T1 |
| S108-T15 | `AGENTS.md` — fresh-agent entry point. Architecture section, project structure, OAuth model, API conventions, testing section, **canonical Provider smoke harness section verbatim from the cross-repo docs PR**, anti-patterns, safe workflow. | P0 | T14 |
| S108-T16 | `Makefile` — `build`, `test`, `lint`, `install-hooks`, `up` (start fakegenesys on `:8083` for local infrafactory dev). | P0 | T5 |
| S108-T17 | First commit-tagged release: `v0.1.0` with CHANGELOG entry. (Tagged after S108 PR merges; do this in S115 close-out rather than mid-arc to avoid stale tags during build-out.) | P1 | S115 |
| S108-T18 | Download the Genesys Cloud OpenAPI spec from `https://api.mypurecloud.com/api/v2/docs/swagger` into `specs/genesys-openapi.json`. Commit. Add a Makefile target `make specs-refresh` that re-downloads (so anyone updating to a newer provider version can refresh the spec). | P0 | T1 |
| S108-T19 | `examples/spec_cross_reference_test.go` — walks the handler route map (post-S109/110/111 it'll cover all 15 resources) and asserts every implemented route exists in `specs/genesys-openapi.json`. In S108 it's a skeleton that returns no errors when the route map is empty; gets exercised as resources land. | P0 | T18 |
| S108-T20 | `AGENTS.md` § "Fidelity strategy" — copy the relevant section from this plan doc into the repo's AGENTS.md so anyone (human or codex) writing a new handler sees the spec-driven rule before touching code. Include the "Reverse fidelity — don't over-correct" rule verbatim from mockway. | P0 | T15, T18 |

### Exit criteria

- `go test ./...` green (OAuth tests pass, repo round-trip tests pass).
- `make build && ./fakegenesys --port 8083` starts cleanly; `curl -X POST localhost:8083/oauth/token -d 'grant_type=client_credentials&client_id=x&client_secret=y'` returns 200 with Bearer token.
- `examples/provider_smoke_test.go` builds and `t.Skip`s without the env var.
- All OSS-mature files present (audit checklist below).
- Repo pushed to `github.com/redscaresu/fakegenesys` (private at this point; flipped public in S115).

### OSS-mature audit checklist (per S108-T2 / T3)

- [ ] `LICENSE` (Apache-2.0)
- [ ] `SECURITY.md`
- [ ] `CONTRIBUTING.md`
- [ ] `CODE_OF_CONDUCT.md`
- [ ] `CHANGELOG.md` (Keep a Changelog format)
- [ ] `.github/workflows/ci.yaml`
- [ ] `.github/workflows/release.yaml`
- [ ] `.gitleaks.toml`
- [ ] `.pre-commit-config.yaml`
- [ ] `scripts/install-hooks.sh`
- [ ] `README.md` (with quickstart + link to AGENTS.md)
- [ ] `AGENTS.md`
- [ ] `Makefile` (build / test / lint / install-hooks targets)

---

## S109 — Identity resources (5)

### Motivation

Identity resources are independent (no inter-resource FKs within this set) and exercise the basic CRUD pipeline. Land them first because everything in S110/S111 depends on `user` / `group` for queue membership, role assignment, etc.

### Resources (each gets handler + handler tests + examples/working/ + examples/updates/ + examples/misconfigured/)

1. **`genesyscloud_user`** — Genesys API: `/api/v2/users` (POST/GET/PUT/DELETE/list). Required: `name`, `email`, `password` (on create only). Optional: `title`, `department`, `manager`, `locations`. Email is the natural key but Genesys assigns a UUID `id`.
2. **`genesyscloud_group`** — `/api/v2/groups`. Required: `name`, `type`. Optional: `members` (user IDs).
3. **`genesyscloud_location`** — `/api/v2/locations`. Required: `name`. Optional: `address`, `coordinates`.
4. **`genesyscloud_auth_role`** — `/api/v2/authorization/roles`. Required: `name`, `permissions[]`. Permissions reference well-known strings (e.g. `routing:queue:edit`).
5. **`genesyscloud_oauth_client`** — `/api/v2/oauth/clients`. Required: `name`, `authorizedGrantType` (`CLIENT_CREDENTIALS` / `CODE`). On create, returns `clientId` + `clientSecret` (the secret is only visible once, like real Genesys).

### Tickets (per resource — replicate this pattern 5×)

| ID | Description | Priority |
|---|---|---|
| S109-T0.{1-5} | **Spec lookup first**: locate the resource's endpoints in `specs/genesys-openapi.json`. Capture request/response shapes + pagination contract + any documented error codes into a per-resource scratch note (committed under `docs/spec-notes/<resource>.md`). Required reading before T1. | P0 |
| S109-T1.{1-5} | Handler implementation in `handlers/<resource>.go`. CRUD + list with pagination. Bearer-required (covered by middleware). 404 on FK violations. 409 on dependent-resource delete. **Match the spec shape verbatim; reverse-fidelity rule applies — don't add validation the spec doesn't declare.** | P0 |
| S109-T2.{1-5} | Handler tests in `handlers/<resource>_test.go`. Lifecycle (Create→Get→List→Delete→404), FK rejection (404 on bad FK, 409 on parent with children), pagination boundary, idempotent create-then-update. | P0 |
| S109-T3.{1-5} | `examples/working/<resource>/main.tf` + `providers.tf` — exercises CRUD against the running mock. | P0 |
| S109-T4.{1-5} | `examples/updates/<resource>/main.tf` + `v1.tfvars` + `v2.tfvars` — change a mutable attribute (e.g. `description`, `email`, `address`) and verify plan no-op after v2 apply. | P0 |
| S109-T5.{1-5} | `examples/misconfigured/<resource>/main.tf` + `expected.txt` — exercises a documented error path (e.g. duplicate email, missing required field). | P0 |
| S109-T6 | `models/state_export.go` — extend `FullState()` to include the 5 new resource types under their service keys (`users`, `groups`, `locations`, `auth_roles`, `oauth_clients`). Topology-derivation hook for infrafactory in S114. | P0 |
| S109-T7 | Coverage matrix update: add the 5 resources to `coverage_matrix.yaml`. | P0 |

### Exit criteria

- All 5 handlers + tests green: `go test ./...` returns ok.
- `INFRAFACTORY_ENABLE_E2E=1 go test ./examples/...` (or `FAKEGENESYS_ENABLE_E2E=1`) green — all 15 example dirs (5 resources × 3 trees) pass their respective contracts.
- `FullState()` returns the 5 resource types when populated.
- One PR landing the whole bundle.

---

## S110 — Routing resources (5)

### Motivation

Routing resources are where the Genesys-specific complexity starts: queues depend on users/skills/wrap-up codes/languages; skills+languages+wrap-up codes are independent; utilization is a singleton per-org. This slice is where the FK / dependency-ordering test surface gets exercised.

### Resources

1. **`genesyscloud_routing_queue`** — `/api/v2/routing/queues`. Depends on `routing_skill`, `routing_wrapupcode`, `routing_language`, `auth_role`, `user`. Members are joined via `/api/v2/routing/queues/{queueId}/members` (separate endpoint).
2. **`genesyscloud_routing_skill`** — `/api/v2/routing/skills`. Independent.
3. **`genesyscloud_routing_wrapupcode`** — `/api/v2/routing/wrapupcodes`. Independent.
4. **`genesyscloud_routing_language`** — `/api/v2/routing/languages`. Independent.
5. **`genesyscloud_routing_utilization`** — `/api/v2/routing/utilization`. Singleton (no `id`). PUT-only.

### Tickets

Same pattern as S109 (5 sub-tickets per resource × 5 resources + state-export + coverage-matrix updates). Notable additions:

| ID | Description | Priority |
|---|---|---|
| S110-T-Q1 | `routing_queue` Update — Genesys uses different field names on PATCH vs POST (e.g. `mediaSettings` flattened on create, nested on update). Capture the provider's actual PATCH body via TF_LOG and pin the difference. | P0 |
| S110-T-Q2 | `routing_queue` members endpoint — separate `/queues/{id}/members` route. Idempotent member-set semantics (PUT replaces the set; no delete-then-add). | P0 |
| S110-T-U1 | `routing_utilization` is a singleton. No POST. PUT returns the updated config. Reset via `/mock/reset` returns it to the default values. | P0 |
| S110-T-FK | Verify FK chain via integration tests: create skill → create queue referencing skill → delete skill → 409 on queue's parent-skill. Mirror fakegcp's FK test depth. | P0 |

### Exit criteria

- All 5 handlers + tests green.
- All 15 example dirs pass.
- FK / dependency-ordering tests pin the cross-resource integrity.
- Queue + member relationship exercised in `examples/working/routing_queue/`.
- One PR.

---

## S111 — Architect + responsemanagement + IDP (5)

### Motivation

The harder slice. `flow` resources are inline YAML inside HCL (the architect flow definition), which requires careful round-tripping. `idp_generic` exercises the SAML/OIDC config surface. `responsemanagement_response` exercises the rich-text content type.

### Resources

1. **`genesyscloud_architect_datatable`** — `/api/v2/flows/datatables`. Has a `schema` attribute with nested JSON.
2. **`genesyscloud_architect_user_prompt`** — `/api/v2/architect/prompts`. Voice prompt resources; audio files referenced by URI.
3. **`genesyscloud_flow`** — `/api/v2/flows`. The hardest resource. Provider sends multipart upload of the flow YAML to `/api/v2/flows/{id}` (PUT). Mock needs to accept the multipart, store the YAML opaquely, and return it on GET. Lock semantics: a flow can be in `locked` state during publish; the provider handles the lock/unlock flow.
4. **`genesyscloud_responsemanagement_response`** — `/api/v2/responsemanagement/responses`. Body is a rich-text blob; just store opaquely.
5. **`genesyscloud_idp_generic`** — `/api/v2/identityproviders/generic`. PUT-only singleton-per-org (similar to utilization).

### Tickets

Same per-resource pattern as S109/S110 plus:

| ID | Description | Priority |
|---|---|---|
| S111-T-FLOW1 | `flow` multipart upload handler: accept `multipart/form-data` POST/PUT, persist the file content opaquely, return a flow snapshot on GET. Pin via test that round-trips a 4KB YAML doc. | P0 |
| S111-T-FLOW2 | `flow` lock/publish state machine: `POST /api/v2/flows/{id}/publish` transitions a `locked` flow to `published`. Mock the minimum state transitions the provider relies on; don't model the actual flow execution. | P0 |
| S111-T-DT1 | `architect_datatable` row endpoint — datatable rows are CRUD'd via `/api/v2/flows/datatables/{id}/rows`. Pin the schema-typed validation (rows must match the parent's schema). | P0 |

### Exit criteria

Same shape as S109/S110, plus: `examples/working/flow/` exercises a non-trivial flow YAML round-trip (apply → plan no-op → destroy).

---

## S112 — Codex review pass 1 (broad triage)

### Motivation

Three slices in, the codebase is feature-complete but unaudited. Codex review pass 1 finds wire-shape divergences, missing test coverage, broken patterns, and nitpicks. Triage substantive findings, ignore nitpicks per the anti-nitpick rule above.

### Tickets

| ID | Description | Priority |
|---|---|---|
| S112-T1 | Run codex against the full fakegenesys codebase. Capture verbatim output to `docs/review-passes/pass1.md`. | P0 |
| S112-T2 | Triage findings into substantive / nitpick. For each substantive finding, file a sub-ticket S112-T3.N. For nitpicks, archive verbatim under `docs/review-passes/pass1.md` § "Declined — nitpick rationale". | P0 |
| S112-T3.{N} | Address each substantive finding. One commit per finding for atomic-revert capability. | P0 |
| S112-T4 | If T3 changes break any handler test, add a regression test pinning the new correct behavior. | P0 |
| S112-T5 | Re-run `go test ./...` + `INFRAFACTORY_ENABLE_E2E=1 go test ./examples/...`. Both must remain green. | P0 |

### Exit criteria

- `docs/review-passes/pass1.md` documents every finding (substantive AND declined) with rationale.
- All substantive findings addressed.
- Tests still green.
- One PR (with the per-finding commit granularity).

---

## S113 — Codex review pass 2 OR consolidation

### Motivation

Pass 2 verifies pass 1 fixes didn't introduce new issues. If pass 2 returns `NOTHING_TO_IMPROVE` (or only nitpicks), this slice becomes consolidation: README polish, examples README, contributor docs.

### Tickets

| ID | Description | Priority |
|---|---|---|
| S113-T1 | Run codex pass 2. Capture to `docs/review-passes/pass2.md`. | P0 |
| S113-T2 | If substantive findings remain: same triage + fix pattern as S112-T3. | P0 |
| S113-T3 | If pass 2 returns NOTHING_TO_IMPROVE (or only nitpicks): stop. | P0 |
| S113-T4 | Consolidation pass — README polish, examples/README polish, any AGENTS.md gaps, version v0.1.0 in CHANGELOG. | P0 |
| S113-T5 | Verify all OSS-mature audit checklist items still present (re-run S108-T2 audit). | P0 |

### Exit criteria

- Two consecutive codex passes without substantive findings (or the second pass with only nitpicks).
- Consolidation polish complete.
- One PR.

---

## S114 — infrafactory integration

### Motivation

fakegenesys is feature-complete in its own repo; the win condition needs infrafactory to dispatch to it. This slice wires the 4th cloud into every dispatch point that AWS/GCP/Scaleway use.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S114-T1 | `prompts/genesys/phase1_architecture.md`, `phase2_generate.md`, `phase3_self_review.md`. Mirror the structure of `prompts/aws/*.md`. Phase-2 introduces the OAuth2 token expectation and the Genesys API base URL. | P0 | — |
| S114-T2 | `pitfalls/genesys.yaml` — empty initial. Loader hooks check that the file exists. | P0 | — |
| S114-T3 | `policies/genesys/*.rego` — minimum 3 policies: (a) `region_restriction` equivalent (queue must specify a supported region), (b) `oauth_client_least_privilege` (oauth_client.scopes must not include `*`), (c) `queue_must_have_wrapup` (every queue must reference at least one wrapupcode). OPA package naming mirrors `policies/aws/*.rego`. | P0 | — |
| S114-T4 | Training scenarios under `scenarios/training/`: `genesys-basic-queue.yaml`, `genesys-multi-queue-skills.yaml`, `genesys-architect-flow.yaml`, `genesys-rbac-and-oauth.yaml`, `genesys-full-stack.yaml`. Each declares its acceptance criteria (region_restriction policy, a topology assertion, optionally http_probe-equivalent for queue reachability — TBD if Genesys exposes a poll endpoint we can hit). | P0 | T1-T3 |
| S114-T5 | `internal/cli/run_command.go` dispatch wiring: `detectCloud("genesys")`, `cloudMockStateRouter` for `genesys` → `http://127.0.0.1:8083/mock/state`, `cloudConstraintPolicies` for `genesys` → `policies/genesys/`, `filterPolicyPathsByCloud`, `ExtractProviderSchemaForCloud` for `mypurecloud/genesyscloud`. | P0 | T4 |
| S114-T6 | `internal/harness/topology_derive.go` — Genesys topology rules. Likely: queue→member graph, flow→queue references, oauth_client→role assignments. Mirror `topology_derive_gcp.go` structurally. | P0 | T5 |
| S114-T7 | `infrafactory.yaml` — add genesys to mocks list with port `8083` and OAuth credentials (`client_id` / `client_secret` env vars referenced; mock accepts anything). | P0 | T5 |
| S114-T8 | `Makefile` updates — `make up` includes fakegenesys; `make mocks-up` / `make mocks-down` / `make mocks-status` / `make mocks-restart` all cover fakegenesys. Sweep target renamed if needed: `sweep-39` → `sweep-N` where N = current total (44 after adding 5 genesys scenarios). | P0 | T7 |
| S114-T9 | `README.md` — extend the Cloud Coverage table with a Genesys row. Note the wire-format + auth specifics. | P0 | — |
| S114-T10 | `AGENTS.md` — add fakegenesys to the siblings list with port, scope description, parity status. | P0 | — |
| S114-T11 | `cmd/infrafactory/main.go` (if needed) — register any new flags for the genesys mock connection. | P1 | T7 |
| S114-T12 | End-to-end validate: `./bin/infrafactory run scenarios/training/genesys-basic-queue.yaml --clean` runs to `target_reached`. | P0 | T1-T8 |

### Exit criteria

- `make sweep-N` discovers all 44 scenarios (39 existing + 5 genesys).
- All 5 genesys scenarios converge `target_reached` in single-iteration runs (sanity check before S115's 3-sweep sustain).
- One PR landing the whole bundle.

---

## S115 — Sustain sweep + sibling-repo cross-link blurbs + arc close-out

### Motivation

The arc's win condition. Run `make sweep-N` three consecutive times under the integrated 4-cloud configuration; verify the 5 genesys scenarios pass deterministically. Sibling-repo READMEs get a cross-link blurb for fakegenesys. Arc close-out + branch protection click-ops note.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S115-T1 | `make mocks-restart` for clean baseline. Run `make sweep-N` 3× into `/tmp/sweep-s115-{1,2,3}`. Capture per-scenario pass/fail, AVOID_EMISSIONS (per-cloud breakdown), panic count, RETRY_TRANSPORT. Also capture `pitfalls/genesys.yaml` line-count delta between sweeps — this is the cold-start learning signal. | P0 | S114 merged |
| S115-T1b | **Learning-system observation table** (genesys-specific): per-sweep `pitfalls/genesys.yaml` line count + source-value breakdown (descriptive / fix / avoid counts). Any sweep where a genesys scenario fails but learning prevented the repeat on the next sweep is recorded as the cold-start loop closing. | P0 | T1 |
| S115-T1c | **Mock-gap audit** (genesys-specific): grep `docs/mock-gaps.md` for entries with `discovered_from: genesys-*`. Each such entry is a genuine fakegenesys fidelity gap surfaced by the classifier — fix at source in fakegenesys before continuing if the count is non-zero. Mirror the S106 fix-forward pattern. | P0 | T1 |
| S115-T2 | Build comparison table across the 3 sweeps. Confirm trajectory: any sweep-1 genesys failures should be addressed by learned pitfalls in sweep 2/3. Flag any flapping (passes once but not consistently — that's the signal that prompts need a static rule the loop can't extract). | P0 | T1, T1b, T1c |
| S115-T3 | If any genesys scenario flaps: investigate in-slice. Likely culprit candidates: OAuth token TTL too short under load, FK ordering in destroy, idempotent-set semantics off. Fix in fakegenesys; ship a quick PR; re-run sweep. | P0 | T2 |
| S115-T4 | `../mockway/README.md`, `../fakegcp/README.md`, `../fakeaws/README.md` — add a cross-link line in their docs section pointing at fakegenesys (mirror the existing infrafactory cross-link blurb shape). | P0 | T1-T3 |
| S115-T5 | `fakegenesys/README.md` — add cross-links pointing at the three other siblings + infrafactory (full sibling pentagon). | P0 | T1 |
| S115-T6 | Tag `fakegenesys v0.1.0`. Release workflow runs. CHANGELOG locked. | P0 | T5 |
| S115-T7 | STATUS.md + docs/NEXT_SESSION.md + docs/status/ARCHIVE.md — folded arc close-out per Option C. | P0 | T1-T6 |
| S115-T8 | docs/NEXT_SESSION.md note: "User click-ops pending — flip `github.com/redscaresu/fakegenesys` to public + enable branch protection on `main` matching the three other siblings' rules." | P0 | T7 |

### Exit criteria

- 3 consecutive `make sweep-N` runs across **all 44 scenarios** (39 existing + 5 genesys). Required: 44/44 deterministic in each sweep (transport retries acceptable per S101 semantics). Specifically:
  - **5/5 genesys scenarios** in `target_reached` (the arc's direct deliverable).
  - **39/39 existing scenarios** still in `target_reached` (regression protection — S114's dispatch wiring touches code paths the existing scenarios traverse; a regression there would silently break AWS/GCP/Scaleway).
- Zero panics across all 3 sweeps.
- All 4 sibling READMEs cross-link to fakegenesys; fakegenesys cross-links to all 3 + infrafactory.
- `v0.1.0` tag pushed.
- Arc close-out lands.

If a non-genesys scenario regresses, fix-forward in-slice — the regression is in S114's dispatch wiring, not in the existing scenario. Either correct the dispatch or amend S109/S110/S111 if a fakegenesys behavior change is rippling through shared infrastructure.

---

## Why this order, in one paragraph

S108 first because OAuth + smoke harness + OSS layout are the foundation everything else stands on. S109/110/111 because resources land in dependency-respecting batches (identity → routing references identity → architect references both). S112/113 because two-pass codex review catches what fresh-eyes reviewers catch, and pass 2 verifies pass 1 didn't regress. S114 because infrafactory integration is the only place the architecture decisions surface in `make sweep-N` output — the win condition. S115 because 3 sustain sweeps are the same standard the other three clouds had to pass.

## Autonomous-execution loop prompt

```
/loop until all 8 slices (S108-S115) in docs/plans/fakegenesys-arc-plan.md are complete: every slice's exit criteria met, every PR merged with green CI, STATUS + NEXT_SESSION + ARCHIVE updated, fakegenesys repo public-ready, sustain sweep showing 5/5 genesys scenarios passing 3 consecutive times.

Read docs/NEXT_SESSION.md first for the prior arc's handoff, then docs/plans/fakegenesys-arc-plan.md for the slice definitions. All prior standing rules apply (slices-54-62 through fakeaws-kms-soft-delete).

Anti-nitpick rule (S112/S113): codex review findings get triaged. Act only on substantive findings (wire shape, missing tests pinning behaviors, broken patterns, FK integrity, auth/security). Ignore style nitpicks, "could be more idiomatic", "consider renaming X to Y". Stop iterating when two consecutive passes have no substantive findings. Document declined findings with rationale in docs/review-passes/passN.md.

Smoke-harness reminder (canonical post-2026-06-05 cross-repo docs sweep): every handler ships with examples/working/, examples/updates/, examples/misconfigured/. The real terraform provider validates wire shape — no real Genesys tenant needed.

S114 + S115 dispatch into infrafactory (the meta-repo). Earlier slices stay in fakegenesys. Cross-repo cascade: mock changes land in fakegenesys; the matching infrafactory test or dispatch update follows in the same arc (S114 onward).

S115 is also the cold-start test of the auto-learning loop: `pitfalls/genesys.yaml` starts empty. Sweep 1 may legitimately have genesys failures; that's diagnostic data, not arc failure. Watch for sweep 2 / sweep 3 improvement driven by learned pitfalls (proves the loop closes from cold start). If a genesys scenario fails with a `IsMockServerBug`-classified shape, the entry lands in `docs/mock-gaps.md` — fix at source in fakegenesys (mirror S106's KMS soft-delete pattern) before the next sweep.

Authority: open + merge PRs with `gh pr merge <N> --squash --admin --delete-branch` once CI is green, in all five repos (infrafactory + 4 siblings).

Stop only when: (a) all 8 slices complete OR (b) you genuinely cannot proceed (document the blocker in NEXT_SESSION + stop).

Before stopping for any reason, update docs/NEXT_SESSION.md with: which slices closed, which are blocked, what next session should do FIRST.
```

## Fresh-context checklist

The autonomous prompt assumes the agent reads, in order:

1. `AGENTS.md` (Option C goal-named arcs; sweep-protocol bullet; smoke-harness pattern cross-link)
2. `docs/NEXT_SESSION.md`
3. This file (`docs/plans/fakegenesys-arc-plan.md`)
4. `STATUS.md`
5. `docs/status/ARCHIVE.md` § "2026-06-05 fakeaws KMS soft-delete" (the immediately-prior arc context)
6. `docs/auto-learning-loop.md` (architecture of the learning system — relevant for S114 pitfall integration)
7. `docs/decisions/0019-learning-system-vocabulary.md` (renamed vocab to use in S114 prompts/pitfalls)
8. `../fakeaws/AGENTS.md` § "Provider smoke harness" (canonical pattern reference)
9. `../fakeaws/CHANGELOG.md`, `../fakeaws/SECURITY.md` (mature OSS layout reference)
10. `../fakeaws/handlers/iam.go`, `../fakeaws/handlers/oauth*.go` (handler patterns — note: fakeaws doesn't have OAuth; S108-T7 implements OAuth fresh)
11. `../fakegcp/examples/provider_smoke_test.go` (per-test fresh-port spawn pattern recommended for fakegenesys)
12. `internal/cli/run_command.go::detectCloud` (S114-T5 dispatch extension point)
13. `internal/harness/topology_derive.go` + per-cloud derivers (S114-T6)
14. `prompts/aws/*.md`, `prompts/gcp/*.md` (prompt structure reference for S114-T1)
15. `policies/aws/*.rego`, `policies/gcp/*.rego` (OPA policy reference for S114-T3)
16. Provider docs at https://registry.terraform.io/providers/MyPureCloud/genesyscloud/latest/docs (resource list — confirm latest stable version + resource inventory before S109/T110/T111)
17. The `mypurecloud/genesyscloud` provider source if any wire shape is ambiguous — repo at https://github.com/MyPureCloud/terraform-provider-genesyscloud
18. **`specs/genesys-openapi.json`** in the fakegenesys repo (downloaded in S108-T18). This is the primary source of truth for handler shapes per the Fidelity strategy section above.
19. **mockway's `specs/` directory + "Reverse fidelity" rule** (`../mockway/AGENTS.md` § "Anti-patterns" #14) — the pattern fakegenesys mirrors.
20. **fakeaws's `concepts.md` § "Why no Smithy codegen"** — the *anti-pattern* to avoid for fakegenesys (reactive build-then-debug is slower than spec-driven build).
