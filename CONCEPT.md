# InfraFactory — Scenario-Driven Infrastructure for Scaleway

**A Software Factory approach to cloud infrastructure, inspired by [factory.strongdm.ai](https://factory.strongdm.ai)**

---

## The Core Idea

Humans define *what* infrastructure should do (scenarios); an AI agent generates the OpenTofu code; a multi-layer validation harness verifies it — no human writes or reviews IaC.

> Infrastructure already has a universal validation interface: the cloud API itself.

You don't need to read Terraform to know if a Kubernetes cluster is healthy. You ping it. You deploy a workload. You check if DNS resolves. You verify the firewall blocks what it should. Infrastructure is already behavioural by nature — which means a scenario-driven factory should converge faster than one for application code.

---

## Documentation Governance

To keep design context accurate as the project evolves:

- `docs/decisions/*.md` (ADRs) are the canonical log of new or changed architectural decisions.
- `CONCEPT.md` is the curated high-level design narrative and decision catalog.
- `STATUS.md` is a rolling execution log (what changed, current blockers, next actions).
- `AGENTS.md` defines mandatory agent workflow, including when ADRs and doc updates are required.

Rule: if a change alters architecture, interfaces, or long-term workflow, create/update an ADR and then update `CONCEPT.md` in the same session.

---

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Cloud | Scaleway-first, cloud-agnostic schema | EU sovereignty, domain expertise. Schema stays portable. |
| IaC tool | OpenTofu | MPL-2.0 license — safe for commercial use. Same Scaleway provider as Terraform. |
| Language | Go (everything) | Matches Scaleway ecosystem (CLI, SDK, TF provider all Go). Single language. |
| Mock server | [Mockway](https://github.com/redscaresu/mockway) — stateful Scaleway API mock | No LocalStack for Scaleway. Build our own, LocalStack-inspired. Separate repo, built first as standalone building block. |
| Mock architecture | Standalone binary, single server with path routing, SQLite state | Single port simplifies OpenTofu provider config. SQLite for persistent, queryable state. |
| Mock scope | 13 services + 1 legacy alias (Instance, VPC, LB, K8s, RDB, IAM, Marketplace, Container Registry, Redis, Block Storage, IPAM, Domain/DNS, VPC Gateways + Account legacy) | Breadth over depth. 42+ resource types, ~250 handler methods + 5 admin endpoints + 1 catch-all. S3 deferred to Mockway v2. |
| Mock generation | Hand-written handlers derived from Terraform SDK + Scaleway API spec | Initial plan was codegen-first (`oapi-codegen`) but Scaleway spec quirks (`x-one-of`, incomplete schemas) made hand-writing faster and more accurate. All ~250 handlers are hand-written. |
| Mock HTTP router | chi | Lightweight, stdlib-compatible, path-parameter routing. No framework lock-in. |
| Mock testing | Unit + integration + e2e in Mockway repo | Unit: SQLite store, FK validation, handler logic. Integration: 282 test functions (261 handler + 21 repository) (CRUD lifecycle, FK rejection, parent validation, error paths). E2E: 22 Terraform examples with double-apply idempotency check. Coverage: 74.9%. |
| Mock state model | SQLite with FK constraints + referential integrity + parent validation | On create: validate referenced resources exist (404 if not). On delete: reject if dependents exist (409). On sub-resource operations: validate parent exists (404 if not). On nested-path operations: validate child belongs to parent in URL. On cross-parent references: validate resources share same grandparent (e.g. LB route frontend+backend same LB). Exceptions: server delete detaches IPs (`SET NULL`) and cascades NICs; LB delete cascades private network attachments and detaches LB IPs. See *Mock fidelity limitations* below. |
| Mock admin API | Structured state schema, versioned contract | `GET /mock/state` returns full resource graph with relationships preserved. This schema is the interface between Mockway and InfraFactory. |
| Topology evaluation | Harness-side, not Mockway | Mockway stores resources + relationships. The harness's TopologyEvaluator queries mock state and reasons about connectivity as graph queries. Clean separation: Mockway = realistic API mock, harness = domain logic. |
| S3 mock | Deferred to Mockway v2 | Object Storage uses S3-compatible API (different auth). Separate port, Azurite-inspired. |
| Security scanning | OPA only for v1 | tfsec/checkov have poor Scaleway support. We control OPA policies — better signal, no false promises. Add tfsec/checkov when they gain Scaleway rules. |
| Agent | Claude Code CLI (`claude -p`), covered by Max plan | No API costs. Pluggable `SeedGenerator` interface for OpenRouter later. |
| Agent pipeline | Phased: plan → generate → self-review, all 3 every iteration | On feedback iterations, each phase gets failure JSON as additional context. Agent re-derives from scratch — no patching. |
| Agent rate limits | `phase_delay_seconds` config, always 3 phases | Max 15 `claude -p` calls for 5 iterations. Add delay between phases if rate limited. Don't collapse phases — consistency > call savings. |
| Agent output | Agent decides structure | No constraints on .tf file organisation. Harness just needs valid OpenTofu in a directory. |
| Provider URL | Verify as first Mockway task | Echo server smoke test proving `api_url` routes all services through single URL. Must pass before writing real handlers. |
| Scenario schema | Hand-written JSON Schema (`scenario.schema.json`) | Validates at parse time in Go loader. Enables VS Code autocompletion. Best error messages since it's crafted, not generated. |
| Stuck detection | Subset check on failure signatures | Compare (check name, resource) pairs between iterations. If N's failures are a subset of N-1 (nothing resolved), agent is stuck — bail out. |
| Run store | Flat files for v1, CXDB-ready interface | RunStore interface in Go (`internal/runstore/`). v1: filesystem. Future: CXDB for continuous reconciliation. |
| Web UI live-state source | Poll-first correctness, websocket best-effort streaming | `run.json` must exist with `status: running` early in a run so the UI can always recover active state. Websocket logs improve immediacy but are not the only source of truth. |
| Web UI live-log source | Replay-first with live append | `/live` replays persisted `app.log` for the run and appends websocket frames when available, so operators still see concrete run logs even if the browser missed the live stream start. |
| Web UI claude execution path | Resolve once, execute absolute path | The `ui` command preflight resolves `agent.claude.command` to an absolute binary path and injects that into the async run runtime so UI-triggered runs do not depend on a later `PATH` lookup. |
| Maintenance model | Model B target (continuous reconciliation) | v1: re-run factory (Model A). Architecture designed for Model B: state tracking, drift detection, self-healing. |
| LLM-as-Judge | Deferred (not in v1) | Avoids API costs. Layers 1-4 catch concrete failures. Judge is nice-to-have. |
| Prompt storage | Markdown files with Go template interpolation | `prompts/` directory. Easy to iterate without recompiling. |
| Config file | Full `infrafactory.yaml` | Covers agent, mockway, Scaleway creds, validation layers, paths. |
| Development env | Docker Compose (Mockway) | One `docker compose up` to start all services. |
| Testing | Unit + Integration + E2E | Full coverage. E2E uses mock agent for determinism. |
| Target users | Platform engineers + developers | Shapes the CLI UX and scenario complexity. |
| Scenario composition | Flat now, mixins later | Self-contained scenarios to start. Add mixins when duplication emerges (~15+ scenarios). |
| Acceptance criteria | Structured objects | Machine-parseable. 5 check types: connectivity, http_probe, destruction, policy, dns_resolution. |
| Resource definitions | Intent-driven default, prescriptive override | Developers use `size: small`; platform engineers can pin `offer: DEV1-S` via `override:`. |
| Size mapping | Config-driven (`mappings.yaml`) | Users customise size-to-offering mappings. Updated independently of CLI releases. |
| Validation chaining | Fail-fast with skip | Stop if hard dependency fails; run independent checks in parallel within a layer. |
| Holdout scenarios | Criteria-only: block, no feedback. Full: normal run. | Criteria-only holdouts are adversarial checks — agent doesn't see failures, prevents gaming. Full holdouts are independent scenarios with stricter criteria — they get the normal feedback loop via `infrafactory run`. |
| Cost checks | Deferred to post-v1 | No reliable Scaleway pricing data source. Remove `cost` acceptance criteria from v1. Add when Infracost supports Scaleway or when price table approach is needed. |
| Sandbox/live deploy (Layer 3) | Enabled, after Layer 2 pre-check (supersedes ADR-0003 via ADR-0010) | Layer 3 runs `tofu apply` against real Scaleway after Layer 2 (mockway) passes. Credentials via `SCW_ACCESS_KEY`/`SCW_SECRET_KEY` env vars. Project bootstrapped as part of the HCL. Destroy behavior: `--no-destroy` always keeps resources; without it, auto-destroy on failure, destroy-after-verification on success. See ADR-0010. |
| Layer 3 validation order | Layer 2 (mockway) gates Layer 3 (real Scaleway) | Fail fast: structural errors caught in seconds against mock before spending real API calls. Layer 3 only runs if Layers 1+2 pass. Layer 3 failures feed back into the iteration loop (which re-runs Layer 2 first). |
| Layer 3 destroy behavior | `--no-destroy` overrides all destruction | `--no-destroy`: keep real resources regardless of run outcome (convergence or failure). No `--no-destroy` + converges: `tofu destroy` after verification. No `--no-destroy` + fails: auto-destroy (clean up failed resources). Development environments assumed non-live. |
| CLI scenario arg | File path, not name | `infrafactory run scenarios/training/web-app-paris.yaml` — explicit, no ambiguity, tab-completable. |
| CLI init | Minimal scaffold | Creates skeleton YAML with required fields + comments. Prints "next steps" suggesting commands to run. No interactive wizard for v1. |
| Holdout formats | Both criteria-only and full scenario | Criteria-only: references training scenario, adds adversarial acceptance criteria against same generated code. Full: independent scenario with stricter criteria. |
| Provider URL injection | `SCW_API_URL` env var | Harness sets `SCW_API_URL=http://localhost:8080` before any `tofu` command. Agent generates normal provider blocks. Zero coupling. |
| Iteration state | Clean slate (clean mode) or snapshot/restore (incremental mode) | **Clean mode** (default when no prior state): `mockway reset` → delete `.tfstate` → write new files → `tofu init`. **Incremental mode** (auto-detected when prior state exists): `mockway restore` (to run-start baseline) → write new files → `tofu init`. See *Incremental Deployment Model* section. |
| Generator interface | Returns files, doesn't write them | `SeedGenerator.Generate()` returns `GeneratedCode` with `map[string][]byte`. ClaudeCodeGenerator: captures stdout from `claude -p`, parses `# File:` blocks. OpenRouterGenerator: parses API response. Harness writes files. Uniform, testable. |
| Mock start command | Docker wrapper | `infrafactory mock start` runs `docker run ghcr.io/redscaresu/mockway`. Consistent with docker-compose workflow. |
| Output on re-run | Overwrite `.tf` files, preserve state | Regenerate `.tf` files in `output/{scenario}/` each run. In incremental mode, preserve `.tfstate` and `.terraform/` (only delete `.tf` files before writing new ones). In clean mode, wipe entire `output/{scenario}/`. Run store keeps iteration history separately. |
| Regression scenarios | Promoted training scenarios | Once a training scenario converges reliably, promote to `scenarios/regression/`. CI runs all regression scenarios on every change (prompt updates, policy changes). |
| Holdout discovery | Scan `scenarios/holdout/` for criteria-only | After convergence, scan holdout dir for criteria-only holdouts whose `references:` matches the training path. Full holdouts are explicit only (see below). |
| Mock credentials | Fake env vars for provider init | Harness sets dummy `SCW_ACCESS_KEY`, `SCW_SECRET_KEY`, `SCW_DEFAULT_PROJECT_ID`. Mockway accepts any token. |
| OPA in Layer 2 | `deny_state` runs after TopologyEvaluator | Layer 2 flow: `tofu apply` → TopologyEvaluator (connectivity) → OPA `deny_state` (policy against mock state). Sequential. |
| Output dir naming | From YAML `scenario:` field | `scenario: web-app-paris` → `output/web-app-paris/`. Canonical, independent of filename. |
| Full holdout trigger | Explicit only | Full holdouts run via `infrafactory run scenarios/holdout/file.yaml`. Only criteria-only holdouts auto-discover after training convergence. |
| Run store location | `.infrafactory/runs/` | Separate from output dir. `output/{scenario}/` is purely latest mutable .tf output (overwritten). `.infrafactory/runs/{scenario}/{run-id}/` keeps iteration history and immutable generated IaC snapshots under `generated/`. |
| Run history enumeration | Skip incomplete run dirs | Historical/partial run directories without `run.json` are tolerated and excluded from `/api/runs` instead of failing the whole history view. |
| Web UI dev websocket path | Direct backend websocket origin, not Vite proxy | In dev, HTTP `/api` still proxies via Vite, but the browser connects directly to backend `/api/ws` (`:4173` by default, overridable with `VITE_UI_API_ORIGIN`) to avoid proxy-reset churn. |
| Web UI websocket origin policy | Explicit localhost allowlist | Because dev UI runs cross-origin (`:5173` → `:4173`), `/api/ws` must allow local browser origins (`127.0.0.1:*`, `localhost:*`) while remaining localhost-scoped by default. |
| Policy `target` field | Passed to OPA as input | Harness passes `target` to OPA input. Policies can optionally filter by target resource type. Without target, checks all resources. |
| Data sources | Prompt constraint: avoid | Prompts tell agent not to use `data` blocks — use hardcoded IDs from mappings. If agent uses them anyway, `tofu plan` fails and feedback loop corrects it. |
| Max iterations | Configurable, default 5 | Most scenarios converge in 2-3. If not by 5, needs human attention. |
| Incremental deployment model | Single evolving scenario, regenerate all HCL | Scenario YAML grows as the project grows (add database block, add redis block). Factory regenerates all HCL each run; OpenTofu diffs against existing state and only creates/modifies what changed. Mirrors real IaC workflow. |
| Incremental mock state | Persist mockway state between runs (requires `--no-destroy`) | When using `--no-destroy`, mockway keeps its state between runs — resources from the previous apply persist, just like a real cloud account. Without `--no-destroy`, Layer 4 destruction wipes everything. `--clean` forces full reset regardless. |
| Incremental detection | Auto-detect from existing state | If mockway has resources, `.tfstate` exists in output dir, and a previous successful run exists in run store → incremental. If any is missing → clean. `--clean` flag overrides to force fresh start. |
| Feedback iteration baseline | Snapshot/restore mockway state per run | At run start, snapshot mockway's current state as baseline. Between feedback iterations within a run, restore to this baseline (not full reset). Tests incremental changes against realistic pre-existing infrastructure. |
| Incremental failure handling | Treat all failures equally | No distinction between regressions (broke something existing) and new failures (new resource misconfigured). LLM regenerates all HCL, feedback loop catches everything. Simpler, avoids dual code paths. |
| Incremental state scope | Single-scenario workflow, shared mockway instance | Incremental runs assume one scenario at a time against one mockway instance. Running different scenarios concurrently against the same mockway is not supported. Users work on one project/scenario at a time — consistent with the single evolving scenario model. |
| Incremental destruction control | `--no-destroy` flag to preserve state | Default: Layer 4 runs `tofu destroy` after convergence (existing behavior). `--no-destroy`: skip Layer 4, preserve mockway state and `.tfstate` so the next run can auto-detect incremental. Without `--no-destroy`, destruction wipes everything and the next run starts clean. Iterative development requires `--no-destroy` to build infrastructure incrementally across runs. |
| Incremental UI | Run flags on scenario page, mode badge + baseline + plan diff on Live page | Scenario page gets `--no-destroy` and `--clean` toggles plus auto-detected mode indicator. Live page shows run mode badge, collapsible baseline state panel, and raw `tofu plan` diff panel. Keeps incremental context visible without adding new pages. |

---

## Scaleway Resources in Scope (v1)

| Category | Resources | API Pattern |
|----------|-----------|-------------|
| Compute | Instances, IPs, Security Groups, Private NICs, Volumes | Zoned: `/instance/v1/zones/{zone}/servers` |
| Networking | VPC, Private Networks, VPC Routes | Regional: `/vpc/v1/regions/{region}/` + `/vpc/v2/regions/{region}/` (routes on v2 only) |
| VPC Gateways | Public Gateways, Gateway Networks | Zoned: `/vpc-gw/v2/zones/{zone}/` |
| Load Balancing | LBs, Frontends, Backends, ACLs, Routes, Certificates, IPs, Private Networks, Health Checks | Zoned + Regional: `/lb/v1/zones/{zone}/` and `/lb/v1/regions/{region}/` |
| Kubernetes | Clusters, Pools, Nodes, Versions, Upgrade, Set-Type, Kubeconfig | Regional: `/k8s/v1/regions/{region}/clusters` |
| Databases | Instances, Databases, Users, Privileges, ACLs, Settings, Read Replicas, Snapshots, Backups, Endpoints, Certificates | Regional: `/rdb/v1/regions/{region}/instances` |
| IAM | Applications, API Keys, Policies, Rules, SSH Keys, Users, Groups | Organisation-scoped: `/iam/v1alpha1/` |
| Block Storage | Volumes, Snapshots, Volume Types | Zoned: `/block/v1alpha1/zones/{zone}/` |
| IPAM | IPs (create, list, update, delete, detach, move) | Regional: `/ipam/v1/regions/{region}/` |
| DNS | Zones, Records | `/domain/v2beta1/` |
| Marketplace | Local Images (known label → UUID resolution, unknown labels rejected) | `/marketplace/v2/` |
| Container Registry | Namespaces | Regional: `/registry/v1/regions/{region}/namespaces` |
| Redis | Clusters, Versions, Node Types, ACL Rules, Endpoints, Settings, Certificates | Zoned: `/redis/v1/zones/{zone}/clusters` |
| Account (legacy) | SSH Keys (alias → IAM ssh-keys state) | `/account/v2alpha1/` |
| Storage | S3-compatible Object Storage (Mockway v2) | S3 API: `s3.{region}.scw.cloud` |

---

## Architecture

```
┌─────────────────────────────────────────────────┐
│              HUMAN LAYER                         │
│  Write scenario YAML                            │
│  Define constraints + acceptance criteria        │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│           SCENARIO LIBRARY                       │
│  scenarios/training/   — agent sees these        │
│  scenarios/holdout/    — adversarial sanity checks│
│  scenarios/regression/ — promoted, CI test suite  │
│  Structured YAML, versioned in git               │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│          SEED GENERATOR (Agent)                  │
│  Claude Code CLI (`claude -p`) on Max plan       │
│  Input: scenario YAML + constraints + feedback   │
│  Output: OpenTofu HCL files in output dir        │
│  Interface: SeedGenerator (pluggable)            │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│         VALIDATION HARNESS                       │
│         (fail-fast with skip)                    │
│                                                  │
│  Layer 1: Static (seconds)                       │
│    ┌─────────────────┐  (sequential)             │
│    │ tofu validate   │                           │
│    │ tofu plan       │                           │
│    └────────┬────────┘                           │
│             ▼                                    │
│    ┌────────────────┐  (parallel)                │
│    │ OPA policies   │                            │
│    │ (plan JSON)    │                            │
│    └────────────────┘                            │
│                                                  │
│  Layer 2: Mock Deploy (seconds-minutes)          │
│    tofu apply → Mockway                          │
│    TopologyEvaluator (connectivity graph queries)│
│    OPA deny_state (policy against mock state)    │
│                                                  │
│  Layer 3: Real Scaleway Deploy (after Layer 2)    │
│    tofu apply → real Scaleway project            │
│    Real network probes: connectivity, DNS        │
│    Only runs if Layers 1+2 pass                  │
│                                                  │
│  Layer 4: Destruction Verification               │
│    tofu destroy + mock state empty check         │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│          FEEDBACK LOOP                           │
│  Max 5 iterations (configurable)                 │
│  Structured failure JSON → all 3 phases          │
│  Re-derive from scratch → re-validate → converge │
│  Stuck detection: subset check on failure sigs   │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│          HOLDOUT VALIDATION                      │
│  Run holdout scenarios against converged code    │
│  Adversarial sanity checks (e.g. no open ports)  │
│  Fail → block, but agent gets NO holdout details │
│  Human decides: add constraint / update policy   │
└─────────────────────────────────────────────────┘
```

---

## Mock Fidelity Limitations

Mockway's referential integrity model was derived from studying the **Terraform/OpenTofu provider's behavior** and the **Scaleway API spec**, then hardened through 14+ rounds of systematic API fidelity review. This means the mock is accurate to what the provider expects, but may diverge from what Scaleway actually does.

### Known divergence risks

1. **LB delete cascade asymmetry** — the real Scaleway API cascade-deletes frontends/backends/routes/certificates when an LB is deleted. Mockway blocks deletion with 409 if frontends or backends exist (no SQL CASCADE), but cascade-deletes routes and certificates (ON DELETE CASCADE). This is acceptable because the Terraform provider always deletes children before the LB (dependency graph ordering), so this code path never executes in practice. The mock is stricter than the real API for frontends/backends.

2. **Implicit cross-service dependencies** — resources that share relationships outside the FK model (e.g., security group rules referencing other groups, DNS records pointing to LB IPs, or IAM policies scoped to specific resources) may produce 409s on the real API that mockway allows through. The mock is more permissive than the real API.

3. **Eventual consistency** — Scaleway resources may have transient states (e.g., K8s cluster `provisioning`, RDB instance `configuring`) where operations are rejected. Mockway transitions resources to their final state immediately. The mock is simpler than the real API.

4. **RDB private network refs not FK-enforced** — RDB instance endpoints store `private_network.id` inside JSON, not as a SQL FK column. Deleting a private network referenced by an RDB endpoint succeeds in mockway but would fail on real Scaleway.

5. **No field-level validation** — mockway deliberately does not validate required fields, field formats, or value constraints (e.g., `commercial_type`, `engine`, `node_type`). The Terraform provider SDK validates these before the API call reaches mockway. This is a design choice, not a gap — mockway focuses on FK references, dependency ordering, attachment constraints, and response shapes.

6. **VPC gateway network `enable_masquerade` drift** — `scaleway_vpc_gateway_network` with `enable_masquerade = true` causes perpetual plan diff. Needs investigation against the real Scaleway API.

### What this means in practice

Mockway validates **structural correctness**: the generated HCL creates resources in the right order, with valid references, correct resource IDs (not names), and tears down cleanly. It catches wrong reference types, incorrect resource IDs, cross-stack ordering problems, and unknown marketplace image labels.

It does not validate **behavioral correctness**: whether the real API accepts the exact sequence of operations the provider sends, or whether field values are within acceptable ranges.

Layer 3 (real Scaleway deploy, planned in Slices 26-29 per ADR-0010) is the only way to close this gap. Until Layer 3 is implemented, the mock catches the majority of IaC defects — sufficient for the factory's feedback loop to converge on structurally correct HCL.

---

## Incremental Deployment Model

Infrastructure grows organically. A project starts with a web server, adds a database weeks later, then Redis months after that. The factory must support this iterative development pattern, not just single-shot generation.

### How it works

The user evolves a single scenario YAML over time:

```
Week 1: scenario has compute + networking
        → infrafactory run --no-destroy scenarios/training/my-project.yaml
        → factory generates HCL → tofu apply creates web server + VPC
        → --no-destroy skips Layer 4, state persists

Week 3: user adds database: block to same scenario
        → infrafactory run --no-destroy scenarios/training/my-project.yaml
        → auto-detects incremental (mockway has state, .tfstate exists)
        → factory regenerates ALL HCL → tofu plan sees existing web server
        → tofu apply only creates the database

Week 6: user adds redis: block
        → same pattern — factory regenerates everything, tofu diffs

Final:  user wants to verify full teardown
        → infrafactory run scenarios/training/my-project.yaml  (no --no-destroy)
        → Layer 4 destroys everything, verifies clean teardown
```

The factory is stateless — it always regenerates the complete HCL from the full scenario. OpenTofu handles the incremental diff via its state file. This mirrors how real infrastructure-as-code works.

### Mock state persistence

For the mock validation to be realistic, mockway must reflect the real deployment state:

1. **Between runs**: when using `--no-destroy`, mockway keeps its state. Resources from the previous `tofu apply` persist, just like a real cloud account. No `mockway reset` between runs. Without `--no-destroy`, Layer 4 destruction wipes everything and the next run starts clean.
2. **Within a run (feedback iterations)**: at run start, the harness snapshots mockway's current state as a baseline. Between feedback iterations, it restores to this snapshot — not a full reset. This ensures each iteration tests "new HCL applied on top of existing infrastructure."
3. **Fresh start**: `infrafactory run --clean scenarios/training/web-app-paris.yaml` forces a full `mockway reset` + delete `.tfstate`, reverting to the v1 clean-slate behavior.

### Detection logic

The run loop auto-detects whether to run incrementally:

1. Check `--clean` flag → if set, always clean run (skip remaining checks)
2. Query mockway state (`GET /mock/state`) — are there existing resources?
3. Check for `.tfstate` in `output/{scenario}/` — does OpenTofu know about existing resources?
4. Check run store for a previous successful run for this scenario
5. If all three exist (mockway state + tfstate + previous run) → incremental run (snapshot baseline, skip reset)
6. If any is missing → clean run (reset mockway, delete tfstate)

### Feedback iteration flow (incremental)

```
Run start:
  1. Detect mode (incremental vs clean)
  2. If incremental: POST /mock/snapshot (save baseline)
  3. If clean: POST /mock/reset + delete .tfstate

Each iteration:
  1. If incremental: POST /mock/restore (restore to baseline)
     If clean: POST /mock/reset (empty state)
  2. Generate all HCL (3-phase pipeline)
  3. Write .tf files to output dir (incremental: delete only *.tf, preserve .tfstate + .terraform/;
     clean: wipe entire output dir)
  4. tofu init (if needed)
  5. tofu plan → Layer 1 validation
  6. tofu apply (mockway) → Layer 2 validation (topology + OPA)
  7. If Layer 3 enabled and Layers 1+2 pass:
       tofu apply (real Scaleway) → Layer 3 validation (real probes)
  8. Collect failures → feed back to next iteration

Convergence:
  9. If --no-destroy: skip destruction, preserve state for next incremental run
     If default + converges: Layer 4 destruction (tofu destroy against mock + real if Layer 3 enabled)
     If default + fails: auto-destroy real resources (if Layer 3 enabled), reset mock
  10. Holdout checks (if applicable, skipped when --no-destroy)
  11. After destruction, state is empty — next run auto-detects as clean
      After --no-destroy, state persists — next run auto-detects as incremental
```

> **Note**: mockway is a single shared instance. Incremental runs assume one scenario at a time. Running different scenarios concurrently against the same mockway is not supported.

### Scenario evolution example

```yaml
# Week 1 — just a web server
scenario: my-project
version: "1.0"
cloud: scaleway
description: Web application in Paris
resources:
  compute:
    purpose: web-server
    size: small
  networking:
    vpc: true
    private_network: true
    load_balancer:
      exposure: public
      backends:
        - port: 80
          protocol: http
constraints:
  region: fr-par
acceptance_criteria:
  - type: http_probe
    target: load_balancer
    port: 80
    expect: reachable
  - type: destruction
    expect: no_orphans
```

```yaml
# Week 3 — add a database (same file, new block)
scenario: my-project
version: "1.0"
cloud: scaleway
description: Web application with PostgreSQL database in Paris
resources:
  compute:
    purpose: web-server
    size: small
  networking:
    vpc: true
    private_network: true
    load_balancer:
      exposure: public
      backends:
        - port: 80
          protocol: http
  database:                          # ← added
    engine: postgresql               # ← added
    size: small                      # ← added
constraints:
  region: fr-par
  no_public_database: true           # ← added
acceptance_criteria:
  - type: http_probe
    target: load_balancer
    port: 80
    expect: reachable
  - type: connectivity               # ← added
    from: compute
    to: database
    port: 5432
    expect: success
  - type: connectivity               # ← added
    from: public_internet
    to: database
    port: 5432
    expect: blocked
  - type: destruction
    expect: no_orphans
```

```yaml
# Week 6 — add Redis (same file, new block)
scenario: my-project
version: "1.0"
cloud: scaleway
description: Web application with PostgreSQL and Redis in Paris
resources:
  compute:
    purpose: web-server
    size: small
  networking:
    vpc: true
    private_network: true
    load_balancer:
      exposure: public
      backends:
        - port: 80
          protocol: http
  database:
    engine: postgresql
    size: small
  redis:                               # ← added
    purpose: session-cache             # ← added
    size: small                        # ← added
constraints:
  region: fr-par
  no_public_database: true
acceptance_criteria:
  - type: http_probe
    target: load_balancer
    port: 80
    expect: reachable
  - type: connectivity
    from: compute
    to: database
    port: 5432
    expect: success
  - type: connectivity
    from: public_internet
    to: database
    port: 5432
    expect: blocked
  - type: connectivity               # ← added
    from: compute                     # ← added
    to: redis                         # ← added
    port: 6379                        # ← added
    expect: success                   # ← added
  - type: destruction
    expect: no_orphans
```

### Mockway snapshot/restore API

New endpoints required in Mockway:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/mock/snapshot` | POST | Snapshot current SQLite state. Returns snapshot ID. Only one active snapshot supported (v1). |
| `/mock/restore` | POST | Restore SQLite state to the most recent snapshot. Fails if no snapshot exists. |
| `/mock/reset` | POST | (Existing) Full reset — clears all state and any active snapshot. |

Implementation: SQLite `VACUUM INTO` for snapshot (copy DB to temp file), file swap for restore.

### What doesn't change

- **Scenario schema**: no changes. The YAML format is the same.
- **Generator interface**: still returns all files, harness writes them.
- **Validation layers**: same 4 layers, same checks.
- **Stuck detection**: same subset check on failure signatures.
- **Run store**: same structure. Adds `incremental: true/false` and optional `previous_run_id` to `RunMetadata`.

---

## Real Scaleway Deploy (Layer 3)

Layer 3 closes the gap between mockway's structural validation and real-world behavioral correctness (see *Mock Fidelity Limitations*). It runs `tofu apply` against a real Scaleway project after Layers 1+2 pass.

### Supersedes ADR-0003

ADR-0003 permanently blocked Layer 3 due to cost, credentials, and safety concerns. ADR-0010 supersedes it with the following governance:

- **Cost**: no estimation (no reliable Scaleway pricing source). User controls cost through scenario scope. Development environments assumed non-live.
- **Credentials**: user provides `SCW_ACCESS_KEY`/`SCW_SECRET_KEY` env vars with real permissions. Same mechanism as mock (dummy values), but with real keys.
- **Safety**: Layer 2 (mockway) acts as a fast pre-check gate — structural errors caught in seconds before real API calls. Auto-destroy on failure prevents orphaned resources.

### How it works

Layer 3 is an optional layer controlled by `validation.layers.sandbox_deploy.enabled: true` in `infrafactory.yaml`.

```
Iteration flow with Layer 3 enabled:

  1. Generate HCL (3-phase pipeline)
  2. Layer 1: tofu validate + tofu plan + OPA (seconds)
     → fail? → feedback loop, skip Layer 2+3
  3. Layer 2: tofu apply against mockway + topology + OPA (seconds)
     → fail? → feedback loop, skip Layer 3
  4. Layer 3: tofu apply against real Scaleway (minutes)
     → fail? → feedback loop (Layer 3 failures included in structured feedback)
     → pass? → convergence

On convergence:
  --no-destroy: keep all resources (mock + real)
  default: tofu destroy against both mock and real Scaleway

On failure (max iterations exhausted):
  --no-destroy: keep all resources
  default: auto-destroy real resources, reset mock
```

### Dual-apply architecture

The same generated HCL is applied to both mockway and real Scaleway in a single iteration. This means:

- **Mockway apply** uses `SCW_API_URL=http://127.0.0.1:8080` (existing behavior)
- **Real Scaleway apply** uses the real API endpoint (no `SCW_API_URL` override)
- Both use the same `.tf` files but separate `.tfstate` files (`terraform.tfstate` for mock, `terraform-live.tfstate` for real)
- The harness manages two state files in `output/{scenario}/`

### Project bootstrap

The generated HCL should include `scaleway_account_project` as a resource, so the factory creates and destroys the project as part of the normal IaC lifecycle. No pre-existing sandbox project needed.

### What Layer 3 validates that Layer 2 cannot

| Check | Layer 2 (mock) | Layer 3 (real) |
|-------|---------------|----------------|
| Resource creation order | FK constraints (structural) | Actual API sequencing + eventual consistency |
| Network connectivity | Graph query (topological) | Real `nc`/`curl` probe |
| DNS resolution | Not evaluatable | Real `dig`/`nslookup` |
| API cascades / 409 behavior | Provider-derived FK model | Actual Scaleway API behavior |
| Provider field requirements | Best-effort response mocking | Real API responses |
| Resource provisioning time | Immediate | Real wait times (cluster creation, etc.) |

### Acceptance criteria unlocked

With Layer 3, the `dns_resolution` acceptance criteria type becomes evaluatable (previously deferred as sandbox-only). `connectivity` and `http_probe` criteria can run real probes instead of graph queries.

---

## Implementation Contracts (Slices 22-29)

These contracts resolve ambiguities that a fresh-context implementer would otherwise have to guess. Each answer is a binding decision — implement exactly as stated.

### 1. Scenario identifier

The `scenario:` YAML field (e.g., `web-app-paris`) is the canonical identifier used in API paths, output directories, and run store. The CLI takes a file path; the API resolves the `scenario:` name from the YAML via `findScenarioPathByName()`. API consumers never use file paths.

### 2. POST /api/runs/{scenario}/start request schema

```json
{
  "clean": false,
  "no_destroy": false,
  "layer3_enabled": false
}
```

All fields optional. Missing fields use defaults shown above. These are run-level overrides, not config overrides.

**Conflict rule**: if `clean=true` and `no_destroy=true` are both set, reject with 422: `{"error": "clean and no_destroy are mutually exclusive"}`. Rationale: `--clean` wipes all state to start fresh; `--no-destroy` preserves state for the next run. These intents contradict — forcing the user to choose prevents silent surprises.

### 3. Config vs API/UI flag precedence

Config is authoritative for security-sensitive settings:

- `validation.layers.sandbox_deploy.enabled` must be `true` in `infrafactory.yaml` for Layer 3 to work. The API `layer3_enabled` field is an additional per-run opt-in — **both** config AND flag must be `true` for Layer 3 to execute. Config is the gate, API flag is the per-run choice.
- `clean` and `no_destroy` are purely run-level. No config equivalent. API/UI flags are the sole authority.

### 4. "Mockway has resources" definition

`GET /mock/state` returns a map of resource-type groups, each containing lists. "Has resources" means **at least one list across all groups contains at least one element**. An initialized but empty state (all lists empty) counts as "no resources."

### 5. previous_run_id

`previous_run_id` in `RunMetadata` is the `run_id` of the **latest run with `status: success`** for the scenario. Failed/stuck runs do not count for auto-detection. If no successful run exists, the field is empty and the auto-detection check fails (→ clean mode).

### 6. Snapshot/restore behavior

- **Snapshot** is called once at run start, before iteration 1. It is a prerequisite for incremental runs.
- **Restore** is called before **every** iteration (including iteration 1). Iteration 1 restore is idempotent — the state hasn't changed since the snapshot. This keeps the flow uniform across all iterations.
- **Snapshot failure**: fail the run hard with a clear error. Do not fall back to clean mode — the user explicitly chose (or auto-detected) incremental.
- **Restore failure**: fail the iteration hard. Do **not** fall back to reset. Falling back to reset would silently destroy the baseline state, turning an incremental run into a corrupt clean run without the user knowing.

### 7. Incremental file cleanup scope

In incremental mode, delete `*.tf` and `*.tf.json` files in `output/{scenario}/` before writing new generated files. Preserve:
- `terraform.tfstate` and `terraform.tfstate.backup`
- `terraform-live.tfstate` and `terraform-live.tfstate.backup` (Layer 3)
- `.terraform/` directory (provider plugins, module cache)

Do **not** recursively delete module directories. The factory generates flat files (no nested modules in v1).

In clean mode, wipe the entire `output/{scenario}/` directory.

### 8. Artifact contracts

| Artifact | Path | Scope |
|----------|------|-------|
| `plan.txt` | `.infrafactory/runs/{scenario}/{run_id}/iterations/{n}/plan.txt` | Per-iteration Layer 1 `tofu plan` stdout |
| `plan-live.txt` | `.infrafactory/runs/{scenario}/{run_id}/iterations/{n}/plan-live.txt` | Per-iteration Layer 3 `tofu plan` stdout (only when Layer 3 enabled) |
| `baseline_state.json` | `.infrafactory/runs/{scenario}/{run_id}/baseline_state.json` | Per-run — snapshot of mockway state taken at run start. **Incremental runs only.** Not persisted for clean runs (no baseline exists). API returns 404 for clean runs. |
| `iteration.json` | `.infrafactory/runs/{scenario}/{run_id}/iterations/{n}/iteration.json` | Per-iteration stage/failure snapshots (existing) |
| `generated/` | `.infrafactory/runs/{scenario}/{run_id}/iterations/{n}/generated/` | Per-iteration IaC snapshots (existing) |

### 9. Missing artifact endpoint behavior

Return **404** with `{"error": "..."}` JSON body. Examples:
- `GET /api/runs/{scenario}/{run_id}/plan` when `plan.txt` doesn't exist → 404.
- `GET /api/runs/{scenario}/{run_id}/baseline` when run was clean (no baseline) → 404.

Do not return 200 with empty payload or structured null. 404 is standard REST and unambiguous.

### 10. Holdout behavior with --no-destroy and Layer 3

- **`--no-destroy`**: skip holdouts entirely. Holdouts are post-convergence adversarial checks. With `--no-destroy`, the user is still iterating incrementally — running holdouts mid-iteration gives false failures. Additionally, criteria-only holdouts execute `tofu destroy` internally, which conflicts with the intent of `--no-destroy`.
- **Layer 3 enabled**: holdouts run against both mock and real state (same dual-apply pattern as the main convergence loop). Holdout destruction verification runs `tofu destroy` against both mock and real.

### 11. Layer 3 credential contract

Required env vars when Layer 3 is enabled: `SCW_ACCESS_KEY` and `SCW_SECRET_KEY`.

Validation: at run start, before any generation or apply. If either is missing:
- CLI: exit with error `"Layer 3 requires SCW_ACCESS_KEY and SCW_SECRET_KEY environment variables"`.
- API: return 422 with `{"error": "Layer 3 enabled but credentials missing: SCW_ACCESS_KEY, SCW_SECRET_KEY"}`.

### 12. Layer 3 project lifecycle

Create per run. The generated HCL includes `scaleway_account_project` as a resource. In incremental mode with `--no-destroy`, the project persists between runs (it's in `terraform-live.tfstate`). On destruction, the project is destroyed along with everything else. On the next incremental run, the existing project is reused via state — consistent with how all other resources work in the incremental model.

### 13. Real state file lifecycle

`terraform-live.tfstate` lives in `output/{scenario}/` alongside `terraform.tfstate`. Same lifecycle rules:
- **Incremental mode**: preserved between runs and between iterations (only `*.tf`/`*.tf.json` files deleted).
- **Clean mode**: deleted along with everything else in the output directory.
- **`--no-destroy`**: persists after run completes.
- **Destruction (default, no `--no-destroy`)**: after `tofu destroy` runs against real Scaleway, the state file reflects the empty state.

### 14. Destroy behavior matrix

| Run outcome | `--no-destroy` set | Default (no flag) |
|-------------|-------------------|-------------------|
| **Converges** | Skip Layer 4. Mock + real state persist. Next run auto-detects incremental. | Layer 4: `tofu destroy` mock. If Layer 3 enabled: `tofu destroy` real. Next run auto-detects clean. |
| **Fails** (budget exhausted / stuck) | Skip Layer 4. Mock + real state persist. Next run auto-detects incremental. | If Layer 3 enabled: auto-destroy real resources (prevent billing). Reset mock. Next run auto-detects clean. If Layer 3 disabled: reset mock only. |

This matrix is testable: unit tests should verify each cell with mock/real state assertions.

### 15. Real probe contract (Layer 3)

| Probe | Pass criteria | Fail criteria |
|-------|--------------|---------------|
| `connectivity` | TCP connect to `{target_host}:{port}` succeeds within timeout | Timeout or connection refused |
| `http_probe` | HTTP GET to `http://{target_host}:{port}/` returns 2xx or 3xx | Timeout, connection refused, or 4xx/5xx |
| `dns_resolution` | DNS lookup returns at least one A or AAAA record | NXDOMAIN or timeout |

Config keys (in `infrafactory.yaml`):

```yaml
validation:
  real_probes:
    timeout_seconds: 5        # Per-attempt timeout
    retries: 6                # Number of attempts before declaring failure
    retry_delay_seconds: 5    # Delay between retry attempts
```

**Layer 3 probe mode**: when Layer 3 is enabled, real probes **replace** graph queries for `connectivity` and `http_probe` — do not run both. Real probes are strictly more authoritative than graph queries. Layer 2 graph queries still run as part of Layer 2 validation (before Layer 3), so structural issues are caught first. Layer 3 probes validate behavioral correctness that graph queries cannot.

**Fallback**: when Layer 3 is off, `connectivity` and `http_probe` use Layer 2 graph queries (existing behavior). `dns_resolution` remains auto-pass informational output.

**`dns_resolution` stabilization**: DNS propagation is inherently asynchronous. There is no separate stabilization window — use the standard retry mechanism (`validation.real_probes.retries` × `validation.real_probes.retry_delay_seconds`). With defaults (6 retries × 5s delay), the probe has up to ~30s total timeout budget plus retry delays (6 attempts × 5s timeout, with 5 pauses between attempts). If DNS hasn't propagated within this window, the probe fails. Operators can tune retry/delay config for environments with slower propagation.

**Probe config scope**: config keys are global (apply to all probes equally). Per-criterion overrides are not supported in v1. If needed later, add optional `timeout_seconds`/`retries` fields to individual acceptance criteria entries.

### 16. Opt-in gating for Layer 3 E2E tests (Slice 28)

- **Build tag**: `go test -run TestLayer3IncrementalE2E -tags layer3 ./...` — tests are gated by build tag, not just env var.
- **Env vars**: `INFRAFACTORY_LAYER3_E2E=1` must be set (double gate with build tag). Also requires `SCW_ACCESS_KEY` and `SCW_SECRET_KEY`.
- **Cost guardrail**: test uses minimal resources (single small compute instance per stage).
- **Cleanup verification**: after test, assert both `GET /mock/state` returns empty AND `tofu show -json` on `terraform-live.tfstate` shows no resources.

### 17. UI readiness endpoint contracts

**`GET /api/scenarios/{scenario}/run-mode`**:
```json
{
  "mode": "incremental",
  "reason": "Prior state detected: mockway has resources, .tfstate exists, previous successful run found"
}
```
`mode`: enum `"incremental"` | `"clean"`. `reason`: human-readable string explaining the detection result.

**`GET /api/scenarios/{scenario}/layer3-status`**:
```json
{
  "enabled": true,
  "ready": true,
  "missing": []
}
```
`enabled`: value of `validation.layers.sandbox_deploy.enabled` from config. `ready`: `enabled` AND all credentials present. `missing`: list of missing env var names (empty when ready or when disabled).

### 18. Run-start when Layer 3 requested but not ready

**Block hard.** If a run requests `layer3_enabled=true` but Layer 3 cannot execute, return 422 and do **not** silently degrade to mock-only. This avoids false confidence that real validation occurred.

- If config gate is off: `{"error": "Layer 3 is disabled in config (validation.layers.sandbox_deploy.enabled=false)"}`
- If config gate is on but credentials missing: `{"error": "Layer 3 enabled in config but credentials missing: SCW_ACCESS_KEY, SCW_SECRET_KEY"}`

The `/layer3-status` endpoint lets the UI check readiness before offering the start button.

### 19. RunMetadata versioning for new fields

The new fields (`Incremental`, `PreviousRunID`, `Layer3Enabled`) were added as optional (`omitempty`) to the existing `"infrafactory.run.metadata.v1"` schema. No version bump was needed because the fields are additive and backward-compatible — Go's `json.Unmarshal` silently ignores missing fields, treating them as zero values (`false`/`""`). Backward-read rule: when reading a record that lacks these fields, `Incremental` defaults to `false`, `PreviousRunID` to `""`, `Layer3Enabled` to `false`. Never fail on missing fields — degrade gracefully. `RunMetadataSchemaLegacy` (`"infrafactory.run.metadata.legacy"`) remains readable unchanged — the same backward-read defaults apply.

### 20. Failure taxonomy for new failure sources

The existing `failure_class` tagging in `FeedbackJSON` must include entries for new failure sources:

| `failure_class` | Source | Description |
|-----------------|--------|-------------|
| `snapshot_failed` | Slice 23 | Mockway `POST /mock/snapshot` returned an error |
| `restore_failed` | Slice 23 | Mockway `POST /mock/restore` returned an error |
| `layer3_apply_failed` | Slice 26 | `tofu apply` against real Scaleway failed |
| `layer3_destroy_failed` | Slice 26 | `tofu destroy` against real Scaleway failed |
| `layer3_credential_missing` | Slice 26 | Required env vars absent at run start |
| `real_probe_failed` | Slice 27 | Real network probe (connectivity/http/dns) failed |

These classes are terminal (snapshot/restore/credential) or feedbackable (apply/destroy/probe). Terminal classes stop the run immediately. Feedbackable classes are included in structured failure JSON for the next iteration, same as existing Layer 1/2 failures.

### 21. Concurrency invariant

**One active run per mockway instance** is the operational invariant. The existing `sync.Mutex` in `uiRunStarter` enforces single-run for API/UI-triggered runs (returns 409 if busy). Separate CLI processes are not globally lock-coordinated in v1; concurrent CLI runs against one mockway instance are unsupported and may corrupt shared incremental state.

Testable contract: if `POST /api/runs/{scenario}/start` is called while another run is active, return `409 Conflict` with `{"error": "run already in progress"}` (defined as `ErrRunBusy` in `handlers_run_executor.go`, emitted by `startRunHandler` in `handlers_runs.go`). This applies regardless of whether the in-progress run is for the same or different scenario.

### 22. Legacy run compatibility

When reading `RunMetadata` from runs created before Slices 22-29:
- Missing `incremental` field → treat as `false`.
- Missing `previous_run_id` field → treat as `""`.
- Missing `baseline_state.json` → API returns 404 (same as clean runs).
- Missing `plan.txt` in iterations → API returns 404.
- Missing `plan-live.txt` → API returns 404 (Layer 3 was not available).

The UI must handle 404 responses for all new artifact endpoints gracefully — show "not available" or hide the panel, never crash or show a blank error.

Missing new fields/artifacts must not change run ordering or filter behavior in `/api/runs`. Legacy runs without `incremental` or `previous_run_id` sort and filter identically to before — these fields are display-only metadata, not sort/filter keys.

---

## Scenario YAML Schema

**Validation**: hand-written JSON Schema (`scenario.schema.json`). Validated at parse time in the Go loader via `santhosh-tekuri/jsonschema/v6`. Enables VS Code autocompletion when referenced in editor YAML settings.

### Acceptance Criteria Check Types (v1)

| Type | Purpose | Layer | How it executes |
|------|---------|-------|-----------------|
| `connectivity` | Test network reachability between resources | Mock (graph query) + Sandbox (real probe) | **Mock**: TopologyEvaluator queries resource graph — do source and target share a private network path? Does target have a public endpoint? **Sandbox**: actual `nc`/`curl` probe. |
| `http_probe` | Test HTTP endpoint responds on port | Mock (graph query) + Sandbox (real probe) | **Mock**: Does LB exist with frontend on specified port, backends configured, and a public IP? **Sandbox**: actual HTTP request. |
| `destruction` | Verify `tofu destroy` leaves no orphans | Mock + Sandbox | After `tofu destroy`, query `GET /mock/state` — all resource lists must be empty. |
| `policy` | OPA/Rego policy checks (encryption, public access, etc.) | Static (plan JSON) + Mock (deployed state) | Layer 1: OPA evaluates plan JSON. Layer 2: OPA evaluates mock state JSON. |
| `dns_resolution` | Verify DNS records resolve correctly | Sandbox only | Real `dig`/`nslookup` against configured DNS. Not evaluatable against mock. |

### Intent-driven example (developer-friendly)

```yaml
scenario: web-app-paris
version: "1.0"
cloud: scaleway
description: >
  A web application with a managed PostgreSQL database
  in Paris, accessible via load balancer, with private
  networking between app and database.

resources:
  compute:
    purpose: web-server
    size: small
    count: 2
  networking:
    vpc: true
    private_network: true
    load_balancer:
      exposure: public
      backends:
        - port: 80
          protocol: http
  database:
    engine: postgresql
    size: small
    high_availability: false

constraints:
  region: fr-par
  no_public_database: true
  encryption_at_rest: true

acceptance_criteria:
  - type: http_probe
    target: load_balancer
    port: 80
    expect: reachable
  - type: connectivity
    from: public_internet
    to: compute
    expect: blocked
    description: "Instances must not be directly reachable from the internet"
  - type: connectivity
    from: public_internet
    to: database
    port: 5432
    expect: blocked
  - type: connectivity
    from: compute
    to: database
    port: 5432
    expect: success
    description: "App servers must reach the database via private network"
  - type: policy
    check: encryption_at_rest
    expect: pass
  - type: policy
    check: no_public_endpoints
    target: database
    expect: pass
  - type: dns_resolution
    domain: "{{scenario_name}}.example.com"
    expect: resolves
  - type: destruction
    expect: no_orphans
```

### Prescriptive override example (platform engineer)

```yaml
scenario: web-app-paris-pinned
version: "1.0"
cloud: scaleway
description: >
  Same as web-app-paris but with exact Scaleway resource
  types pinned for cost control.

resources:
  compute:
    purpose: web-server
    size: small
    count: 2
    override:
      offer: DEV1-S
      image: ubuntu_jammy
  networking:
    vpc: true
    private_network: true
    load_balancer:
      exposure: public
      backends:
        - port: 80
          protocol: http
  database:
    engine: postgresql
    size: small
    high_availability: false
    override:
      node_type: DB-DEV-S
      engine_version: "15"

constraints:
  region: fr-par
  zone: fr-par-1
  no_public_database: true
  encryption_at_rest: true

acceptance_criteria:
  - type: http_probe
    target: load_balancer
    port: 80
    expect: reachable
  - type: connectivity
    from: public_internet
    to: database
    port: 5432
    expect: blocked
  - type: connectivity
    from: compute
    to: database
    port: 5432
    expect: success
  - type: destruction
    expect: no_orphans
```

### Holdout scenarios

Holdouts are adversarial sanity checks that run **after** the training scenario converges. The agent never sees holdout failures — this prevents gaming (e.g. opening all ports to satisfy "web server responds on port 80").

Two formats are supported:

**Criteria-only holdout** (references existing generated code):

```yaml
scenario: no-open-ports
version: "1.0"
cloud: scaleway
type: holdout
references: scenarios/training/web-app-paris.yaml
description: >
  Verify the web-app-paris generated code doesn't open
  unnecessary ports just to satisfy the LB check.

acceptance_criteria:
  - type: connectivity
    from: public_internet
    to: compute
    expect: blocked
    description: "Instances must not be directly reachable — only via LB"
  - type: policy
    check: no_public_endpoints
    target: compute
    expect: pass
```

**Full holdout scenario** (generates independently with stricter criteria):

A full scenario YAML (same format as training) placed in `scenarios/holdout/`. Run explicitly via `infrafactory run scenarios/holdout/file.yaml`. The agent generates code independently and gets the normal feedback loop (same as training). The `scenarios/holdout/` directory is an organizational convention for scenarios with stricter criteria — `infrafactory run` behaves identically regardless of directory.

**Auto-discovery** (criteria-only holdouts): after `infrafactory run` converges a training scenario, it scans `scenarios/holdout/` for criteria-only holdouts whose `references:` field matches the training scenario path. Only those run automatically. Full holdout scenarios are never auto-discovered — they must be run explicitly.

**Criteria-only holdout execution flow**: after training convergence, mock state is empty (destruction already ran). The harness redeploys the converged code:
1. Take converged `.tf` files from `output/{training-scenario}/`
2. `POST /mock/reset` → delete `.tfstate` → `tofu init` (same clean slate as iteration start)
3. `tofu apply` to Mockway (fresh state)
4. Run holdout acceptance criteria (TopologyEvaluator + OPA `deny_state`)
5. `tofu destroy` cleanup

This is essentially Layers 2-4 of the validation harness, but using the holdout's acceptance criteria instead of the training scenario's.

### Regression scenarios

Once a training scenario converges reliably across multiple runs, promote it to `scenarios/regression/`. Same YAML format as training. CI runs all regression scenarios on every change (prompt updates, policy changes, new OPA rules) to catch regressions. Think of it as the test suite for the factory itself.

### Scenario Field Reference

The scenario YAML is validated at parse time against `scenario.schema.json` (JSON Schema draft 2020-12) using `santhosh-tekuri/jsonschema/v6`. VS Code autocompletion works when the schema is referenced in editor YAML settings.

**Top-level fields**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `scenario` | string | Yes | Unique identifier. Must match `^[a-z][a-z0-9-]*[a-z0-9]$`. Used for output dir naming. |
| `version` | string | Yes | Schema version. Only `"1.0"` in v1. |
| `cloud` | string | Yes | Target cloud. Only `"scaleway"` in v1. |
| `description` | string | Yes | Human-readable intent description. Passed to agent. |
| `type` | string | No | Set to `"holdout"` for holdout scenarios. Omit for training/regression. |
| `references` | string | No | Training scenario path (criteria-only holdouts only). Requires `type: holdout`. |
| `resources` | object | Conditional | Required unless `type: holdout` with `references` (criteria-only holdout). |
| `constraints` | object | No | Constraint key-value pairs mapped to OPA policies. |
| `acceptance_criteria` | array | Yes | At least 1 criterion. All must pass for convergence. |

**`resources.compute`**:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `purpose` | string | Yes | — | Logical role (e.g., `web-server`). Used for naming. |
| `size` | string | Yes | — | `small` \| `medium` \| `large` \| `xlarge`. Resolved via `mappings.yaml`. |
| `count` | integer | No | `1` | Number of instances. |
| `override.offer` | string | No | — | Exact Scaleway commercial type (e.g., `DEV1-S`). Overrides size mapping. |
| `override.image` | string | No | — | Image label (e.g., `ubuntu_jammy`) or UUID. |

**`resources.networking`**:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `vpc` | boolean | No | `false` | Create a VPC. |
| `private_network` | boolean | No | `false` | Create a private network inside the VPC. |
| `load_balancer.exposure` | string | If LB | — | `public` or `private`. |
| `load_balancer.backends[].port` | integer | If LB | — | Backend port (1–65535). |
| `load_balancer.backends[].protocol` | string | If LB | — | `http`, `https`, or `tcp`. |

**`resources.database`**:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `engine` | string | Yes | — | `postgresql` or `mysql`. |
| `size` | string | Yes | — | `small` \| `medium` \| `large` \| `xlarge`. Resolved via `mappings.yaml`. |
| `high_availability` | boolean | No | `false` | Enable HA standby node. |
| `override.node_type` | string | No | — | Exact Scaleway node type (e.g., `DB-DEV-S`). |
| `override.engine_version` | string | No | — | Engine version (e.g., `"15"`). |

**`resources.kubernetes`**:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `size` | string | Yes | — | `small` \| `medium` \| `large` \| `xlarge`. Resolved via `mappings.yaml`. |
| `override.node_type` | string | No | — | Exact Scaleway node type. |
| `override.node_count` | integer | No | — | Exact node count. |

**`resources.iam`**:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `purpose` | string | Yes | — | Logical role (e.g., `deploy-bot`). |
| `application` | boolean | No | `true` | Create an IAM application. |
| `api_key` | boolean | No | `true` | Create an API key for the application. |
| `policy` | boolean | No | `true` | Create a policy for the application. |

**`resources.registry`**:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `purpose` | string | Yes | — | Logical role (e.g., `ci-artifacts`). |
| `is_public` | boolean | No | `false` | Whether the registry namespace is public. |

**`resources.redis`**:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `purpose` | string | Yes | — | Logical role (e.g., `session-store`). |
| `size` | string | Yes | — | `small` \| `medium` \| `large` \| `xlarge`. Resolved via `mappings.yaml` (RED1-MICRO, RED1-S, RED1-M, RED1-L). |
| `override.node_type` | string | No | — | Exact Redis node type. |

**`constraints`** — extensible key-value map. Known keys:

| Key | Type | Description |
|-----|------|-------------|
| `region` | string | Required Scaleway region. Enforced by `region_restriction.rego`. |
| `zone` | string | Required Scaleway zone. Enforced by `region_restriction.rego`. |
| `no_public_database` | boolean | Enforced by `no_public_database.rego`. |
| `encryption_at_rest` | boolean | Enforced by `encryption_at_rest.rego`. |
| `no_public_endpoints` | boolean | Enforced by `no_public_endpoints.rego`. |

Additional constraint keys can be added — they map to OPA policies via `constraint_policies` in `infrafactory.yaml`.

**`acceptance_criteria[]`** — each criterion has a `type` that determines required fields:

| Type | Required Fields | `expect` Values | Layer |
|------|----------------|----------------|-------|
| `connectivity` | `from`, `to` | `success`, `blocked` | Mock (graph) + Sandbox (probe) |
| `http_probe` | `target`, `port` | `reachable`, `unreachable` | Mock (graph) + Sandbox (probe) |
| `destruction` | — | `no_orphans` | Mock + Sandbox |
| `policy` | `check` | `pass`, `fail` | Static (plan) + Mock (state) |
| `dns_resolution` | `domain` | `resolves`, `not_resolves` | Sandbox only |

Optional fields on all types: `description` (human-readable), `port` (connectivity only — scopes to a specific port), `target` (policy only — scopes to a resource type, passed to OPA as `input.target`).

---

## Project Structure

Mockway lives in a **separate repo** ([`mockway`](https://github.com/redscaresu/mockway)).

```
infrafactory/
├── cmd/
│   └── infrafactory/             # CLI entrypoint
│       └── main.go
├── internal/
│   ├── cli/                      # Cobra commands
│   │   ├── root.go
│   │   ├── init.go
│   │   ├── generate.go
│   │   ├── validate.go
│   │   ├── test.go
│   │   └── run.go
│   ├── scenario/                 # Scenario parsing + validation
│   │   ├── schema.go
│   │   ├── loader.go
│   │   └── mapping.go
│   ├── generator/                # Seed generator interface + impls
│   │   ├── generator.go
│   │   └── claudecode.go
│   ├── harness/                  # Validation harness
│   │   ├── harness.go
│   │   ├── layer.go
│   │   ├── static.go
│   │   ├── mockdeploy.go
│   │   ├── sandbox.go
│   │   └── destroy.go
│   ├── topology/                 # TopologyEvaluator — graph queries on mock state
│   │   └── evaluator.go
│   ├── feedback/                 # Feedback loop + JSON structures
│   │   └── feedback.go
│   ├── runstore/                 # Run store interface + impls
│   │   └── store.go
│   └── config/                   # CLI config parsing
│       └── config.go
├── prompts/                      # Agent prompt templates (each handles {{.FeedbackJSON}})
│   ├── phase1_plan_architecture.md
│   ├── phase2_generate_hcl.md
│   └── phase3_self_review.md
├── scenarios/                    # Scenario YAML files
│   ├── training/
│   ├── holdout/
│   └── regression/
├── policies/                     # OPA/Rego policies
│   ├── common/
│   ├── scaleway/
│   └── custom/
├── testdata/                      # Test fixtures
│   ├── scenarios/                 # Valid + invalid scenario YAML
│   ├── plans/                     # tofu plan JSON fixtures
│   ├── generated/                 # Pre-written OpenTofu for mock agent
│   └── policies/                  # Test OPA policies
├── scenario.schema.json            # JSON Schema for scenario YAML validation
├── mappings.yaml
├── infrafactory.yaml
├── docker-compose.yml
├── goreleaser.yml
├── go.mod
├── go.sum
├── output/                        # Generated OpenTofu files (per scenario, overwritten)
├── .infrafactory/                 # Runtime data (gitignored)
│   └── runs/                      # Run history per scenario
├── .gitignore
├── CONCEPT.md
└── README.md
```

---

## CLI UX

**Framework**: Cobra

**Commands**:
- `infrafactory init` — scaffold a skeleton scenario YAML with comments + print next steps
- `infrafactory generate <path>` — invoke agent, produce OpenTofu
- `infrafactory validate <path>` — static validation only (Layer 1)
- `infrafactory test <path>` — full validation harness (all layers) against existing OpenTofu
- `infrafactory run <path>` — generate + validate loop until convergence, then run holdouts
- `infrafactory ui` — serve the browser UI for scenarios, live runs, diagnostics, and per-run IaC history
- `infrafactory mock start` — start Mockway via `docker run ghcr.io/redscaresu/mockway`

`<path>` is always a scenario YAML file path (e.g. `scenarios/training/web-app-paris.yaml`), not a name lookup. The `validate` and `test` commands parse the scenario to derive the output dir (`output/{scenario}/`) and look for existing `.tf` files there. They require that `generate` or `run` has been executed first.

**Global flags**:
- `--verbose` / `-v` — stream raw tool output
- `--dry-run` — show resolved scenario + which layers would run, no execution
- `--max-iterations N` — override default 5
- `--config PATH` — custom config file (default: `./infrafactory.yaml`)
- `--output-dir PATH` — where generated OpenTofu goes (default: `./output/{scenario}/`, where `{scenario}` is the YAML `scenario:` field value)

**Example output** (default structured progress):

```
InfraFactory v0.1.0 — web-app-paris

Iteration 1/5
  Generating OpenTofu...              done (12s)
  Validating...
    tofu validate                     PASS
    tofu plan                         PASS
    policy/no_public_database         FAIL  scaleway_rdb_instance.main has no private_network
    policy/encryption_at_rest         PASS
  Skipping mock deploy (policy failures)

Iteration 2/5
  Generating OpenTofu (with feedback)... done (15s)
  Validating...
    tofu validate                     PASS
    tofu plan                         PASS
    policy/no_public_database         PASS
    policy/encryption_at_rest         PASS
  Mock deploy...
    tofu apply → Mockway              PASS (6 resources created)
    connectivity: internet→db:5432    PASS (blocked)
    connectivity: compute→db:5432     PASS (reachable)
    http_probe: lb:80                 PASS
    policy_state/no_public_database   PASS
  Destruction...
    tofu destroy                      PASS (6 resources destroyed)
    orphan check                      PASS (0 orphans)

CONVERGED in 2 iterations

Output: output/web-app-paris/
```

---

## Config Reference (`infrafactory.yaml`)

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `version` | string | Yes | — | Config version. `"1.0"`. |
| `agent.type` | string | Yes | — | Agent implementation. `claude-code` or `openrouter`. |
| `agent.max_iterations` | integer | No | `5` | Maximum generate→validate iterations before stopping. |
| `agent.phase_delay_seconds` | integer | No | `0` | Pause (seconds) between `claude -p` calls. Increase if rate limited. |
| `agent.phases` | []string | No | `[plan_architecture, generate_hcl, self_review]` | Phase names. Must match prompt template filenames in `paths.prompts`. |
| `mockway.url` | string | Yes | — | Mockway HTTP URL (e.g., `http://localhost:8080`). |
| `mockway.auto_reset` | boolean | No | `true` | Whether to `POST /mock/reset` at iteration start. |
| `scaleway.credentials_source` | string | No | `env` | Where to read Scaleway credentials. `env` reads `SCW_ACCESS_KEY` etc. from environment. |
| `validation.layers.static.enabled` | boolean | No | `true` | Enable Layer 1 (tofu validate/plan + OPA). |
| `validation.layers.static.policy_paths` | []string | No | `[]` | Directories containing OPA `.rego` files. |
| `validation.layers.mock_deploy.enabled` | boolean | No | `true` | Enable Layer 2 (tofu apply → Mockway). |
| `validation.layers.sandbox_deploy.enabled` | boolean | No | `false` | Enable Layer 3 (real Scaleway deploy). Requires `SCW_ACCESS_KEY` and `SCW_SECRET_KEY` env vars. See Implementation Contract #3, #11, #18. |
| `validation.layers.destruction.enabled` | boolean | No | `true` | Enable Layer 4 (tofu destroy + orphan check). |
| `constraint_policies` | map[string]string | No | `{}` | Maps constraint names to `.rego` file paths (relative to `paths.policies`). |
| `paths.scenarios` | string | No | `./scenarios` | Root directory for scenario YAML files. |
| `paths.mappings` | string | No | `./mappings.yaml` | Path to size mappings file. |
| `paths.output` | string | No | `./output` | Root directory for generated OpenTofu files. |
| `paths.policies` | string | No | `./policies` | Root directory for OPA policy files. |
| `paths.prompts` | string | No | `./prompts` | Directory containing prompt templates. |

### Mappings Resolution (`mappings.yaml`)

The `mappings.yaml` file maps intent-driven T-shirt sizes to concrete Scaleway offerings:

```yaml
scaleway:
  <category>:      # compute | database | kubernetes
    <size>:         # small | medium | large | xlarge
      <field>: <value>
```

**Resolution algorithm** (in `internal/scenario/mapping.go`):

1. Read the scenario's resource category and `size` field (e.g., `resources.compute.size: small`)
2. Look up `scaleway.<category>.<size>` in `mappings.yaml` → returns a map of fields (e.g., `{offer: DEV1-S, vcpus: 2, ram_gb: 2}`)
3. If the scenario has an `override` block, merge override fields on top — **override wins** on conflicts
4. Pass the resolved fields to the agent prompt as `{{.ResolvedMappings}}`

**Example**: scenario has `compute.size: small` with `override.image: ubuntu_jammy`:
- Mapping lookup: `scaleway.compute.small` → `{offer: DEV1-S, vcpus: 2, ram_gb: 2}`
- Override merge: `{offer: DEV1-S, vcpus: 2, ram_gb: 2, image: ubuntu_jammy}`
- Result passed to agent

No region-specific or zone-specific mapping logic — the same mappings apply globally. Region/zone are constraints, not mapping selectors.

---

## Validation Harness Detail

### Provider URL injection

The harness sets environment variables before running any `tofu` command:

```
SCW_API_URL=http://localhost:8080           # Mockway URL (from config)
SCW_ACCESS_KEY=SCWXXXXXXXXXXXXXXXXX         # Fake — Mockway accepts any token
SCW_SECRET_KEY=00000000-0000-0000-0000-000000000000
SCW_DEFAULT_PROJECT_ID=00000000-0000-0000-0000-000000000000
```

The agent generates normal Scaleway provider blocks — no special configuration needed. The Scaleway OpenTofu provider reads these env vars and routes all API calls through Mockway.

### Clean slate per iteration

Each iteration starts fresh. **At the start of each iteration** (before Layer 1):
1. `tofu destroy` (if `.tfstate` exists — uses previous iteration's .tf files for provider config)
2. `POST /mock/reset` (wipe Mockway state)
3. Delete `.tfstate` files (keep `.terraform/providers/` cached to avoid re-downloading the Scaleway provider each iteration)
4. Harness writes `GeneratedCode` files to `output/{scenario}/` (overwriting previous iteration's .tf files)
5. `tofu init` (fast — provider already cached)

This must happen before Layer 1 because `tofu plan` requires `tofu init` and contacts Mockway to refresh state. Without clean slate first, `tofu plan` could see stale resources from a previous failed iteration and produce incorrect plans.

This matches the "re-derive from scratch" philosophy — since the agent regenerates all .tf files each iteration, applying on top of stale state would cause conflicts.

### Chaining: fail-fast with skip

- If `tofu validate` fails → skip everything
- If `tofu plan` fails → skip everything
- Within Layer 1: OPA policies run in parallel against plan JSON
- If Layer 1 has hard failures → skip Layer 2
- Layer 2: `tofu apply` → Mockway, then TopologyEvaluator (connectivity graph queries), then OPA `deny_state` rules against mock state JSON
- Layer 3 (real Scaleway deploy) is opt-in, gated by `validation.layers.sandbox_deploy.enabled` config + `layer3_enabled` per-run flag. Only runs if Layers 1+2 pass. See *Real Scaleway Deploy (Layer 3)* section and Implementation Contracts #3, #18.
- Layer 4 (destruction) runs if anything was deployed (skipped when `--no-destroy` is set)
- `dns_resolution` acceptance criteria are **auto-pass** when sandbox deploy is disabled (they require real DNS). Auto-pass checks appear in output as informational — they don't block convergence

### TopologyEvaluator

The TopologyEvaluator is a harness component that reasons about infrastructure connectivity by querying Mockway's admin state API. It does **not** make network probes — it walks the resource graph.

**How connectivity checks resolve against mock state:**

| Check | Graph query |
|-------|------------|
| `from: compute, to: database, expect: success` | Do any servers have a `private_nic` whose `private_network_id` matches a private network the RDB instance is also attached to? |
| `from: public_internet, to: database, expect: blocked` | Does the RDB instance have only private endpoints (no endpoint without `private_network`)? |
| `from: public_internet, to: compute, expect: blocked` | Do any servers have a `public_ip` assigned? If not → blocked. |
| `http_probe: load_balancer:80, expect: reachable` | Does an LB exist with a public IP, a frontend on port 80, and at least one backend? |
| `destruction: expect: no_orphans` | After `tofu destroy`, is `GET /mock/state` empty across all services? |

The evaluator fetches `GET /mock/state`, builds an in-memory graph (nodes = resources, edges = FK references), and answers each criterion by walking the graph. This is structural reasoning, not simulation.

### Feedback JSON Structure

**Example: iteration with static failures (Layer 2 skipped)**:

```json
{
  "scenario": "web-app-paris",
  "iteration": 1,
  "converged": false,
  "layers": {
    "static": {
      "status": "partial",
      "checks": [
        {"name": "tofu_validate", "status": "pass"},
        {"name": "tofu_plan", "status": "pass"},
        {"name": "policy/no_public_database", "status": "fail",
         "detail": "scaleway_rdb_instance.main has no private_network configured",
         "resource": "scaleway_rdb_instance.main", "file": "database.tf:8"},
        {"name": "policy/encryption_at_rest", "status": "pass"}
      ]
    },
    "mock_deploy": {
      "status": "skipped",
      "reason": "policy failures in static layer"
    }
  }
}
```

**Example: iteration with mock deploy results (Layer 2 ran)**:

```json
{
  "scenario": "web-app-paris",
  "iteration": 2,
  "converged": false,
  "layers": {
    "static": {
      "status": "pass",
      "checks": [
        {"name": "tofu_validate", "status": "pass"},
        {"name": "tofu_plan", "status": "pass"},
        {"name": "policy/no_public_database", "status": "pass"},
        {"name": "policy/encryption_at_rest", "status": "pass"}
      ]
    },
    "mock_deploy": {
      "status": "partial",
      "checks": [
        {"name": "tofu_apply", "status": "pass", "detail": "6 resources created"},
        {"name": "connectivity/compute→database", "status": "pass"},
        {"name": "connectivity/internet→database", "status": "pass"},
        {"name": "http_probe/lb:80", "status": "fail",
         "detail": "LB has no frontend on port 80",
         "resource": "scaleway_lb.main"},
        {"name": "policy_state/no_public_database", "status": "pass"}
      ]
    },
    "destruction": {
      "status": "pass",
      "checks": [
        {"name": "tofu_destroy", "status": "pass", "detail": "6 resources destroyed"},
        {"name": "orphan_check", "status": "pass", "detail": "0 orphans"}
      ]
    }
  }
}
```

---

## Harness Orchestration (Main Loop)

The `infrafactory run` command drives the full generate→validate→feedback loop. Pseudocode for the main loop in `internal/harness/harness.go`:

```go
func (h *Harness) Run(ctx context.Context, scenario Scenario) (*RunResult, error) {
    runID, _ := h.runStore.CreateRun(ctx, scenario.Name)
    var prevFailures []FailureSignature
    var feedback *Feedback // nil on first iteration

    for i := 1; i <= h.config.MaxIterations; i++ {
        // 1. Generate code (3-phase agent pipeline)
        generated, err := h.generator.Generate(ctx, scenario, feedback)
        if err != nil {
            return nil, fmt.Errorf("iteration %d: generate failed: %w", i, err)
        }

        // 2. Clean slate
        h.cleanSlate(ctx, scenario) // destroy → reset mock → delete tfstate

        // 3. Write generated files to output dir
        outputDir := filepath.Join(h.config.OutputDir, scenario.Name)
        for name, content := range generated.Files {
            os.WriteFile(filepath.Join(outputDir, name), content, 0644)
        }

        // 4. Run tofu init
        h.runTofu(ctx, outputDir, "init")

        // 5. Validate through layers (fail-fast)
        iteration := h.validate(ctx, scenario, outputDir)
        iteration.Number = i
        iteration.Files = generated.Files

        // 6. Store iteration result
        h.runStore.AppendIteration(ctx, runID, iteration)

        // 7. Check convergence
        if iteration.Converged {
            // Run criteria-only holdouts
            h.runHoldouts(ctx, scenario, outputDir)
            return &RunResult{Converged: true, Iterations: i, RunID: runID}, nil
        }

        // 8. Stuck detection
        currentFailures := extractFailureSignatures(iteration)
        if isSubset(currentFailures, prevFailures) {
            return &RunResult{Converged: false, Stuck: true, Iterations: i, RunID: runID}, nil
        }
        prevFailures = currentFailures

        // 9. Build feedback for next iteration
        feedback = &Feedback{Scenario: scenario.Name, Iteration: iteration}

        // 10. Phase delay (rate limit mitigation)
        if h.config.PhaseDelay > 0 {
            time.Sleep(h.config.PhaseDelay)
        }
    }

    return &RunResult{Converged: false, Iterations: h.config.MaxIterations, RunID: runID}, nil
}
```

The `validate` method implements the layer chain described in Validation Harness Detail. Each layer returns checks; the harness aggregates them into an `Iteration`. The `cleanSlate` method runs: `tofu destroy` (if state exists) → `POST /mock/reset` → delete `.tfstate` → keep `.terraform/providers/` cached.

---

## Feedback Loop

**Max iterations**: 5 (configurable via `--max-iterations` or `infrafactory.yaml`).

**Flow**: generate → validate → if failures, feed structured JSON back to agent → re-generate → re-validate → repeat until converged or stuck.

**Converged**: all acceptance criteria pass across all validation layers.

### Stuck Detection (subset check)

After each iteration, compute the set of failure signatures:

```go
type FailureSignature struct {
    CheckName string // e.g. "policy/no_public_database"
    Resource  string // e.g. "scaleway_rdb_instance.main" (empty if not resource-specific)
}
```

**Rules:**
- If iteration N's failure set is a **subset of** iteration N-1's set → **stuck** (nothing got fixed). Bail out.
- If the set shrank (some failures resolved) → **progress**. Continue.
- If a new failure appeared that wasn't in N-1 → **not stuck** (agent is trying different approaches). Continue.

**On stuck**: exit with a clear message listing the unresolved failures. Human attention needed.

**On max iterations reached**: exit with current state. Report which checks pass and which still fail.

---

## OPA Policy Design

**Approach**: Hybrid — pre-written policy library for common constraints, novel constraints caught by mock deploy acceptance criteria.

**Policy input**: Both plan JSON (Layer 1) AND mock state (Layer 2).

**Policy library structure**:

```
policies/
├── common/                          # Cloud-agnostic
│   └── naming.rego                  # resource naming conventions
├── scaleway/                        # Scaleway-specific
│   ├── no_public_database.rego      # RDB must use private networks
│   ├── encryption_at_rest.rego      # Storage/DB encryption enabled
│   ├── no_public_endpoints.rego     # No public IPs on compute
│   ├── region_restriction.rego      # Resources in allowed regions only
│   └── vpc_required.rego            # All resources attached to VPC
└── custom/                          # User-added policies
```

**Constraint-to-policy mapping** (auto-discovered via `infrafactory.yaml`):

```yaml
constraint_policies:
  no_public_database: scaleway/no_public_database.rego
  encryption_at_rest: scaleway/encryption_at_rest.rego
  no_public_endpoints: scaleway/no_public_endpoints.rego
  region: scaleway/region_restriction.rego
  zone: scaleway/region_restriction.rego
```

**Example policy** (`policies/scaleway/no_public_database.rego`):

```rego
package scaleway.no_public_database

import rego.v1

deny contains msg if {
    resource := input.planned_values.root_module.resources[_]
    resource.type == "scaleway_rdb_instance"
    not resource.values.private_network
    msg := sprintf(
        "%s has no private_network — public access allowed",
        [resource.address]
    )
}

deny_state contains msg if {
    instance := input.state.rdb.instances[_]
    endpoint := instance.endpoints[_]
    not endpoint.private_network
    msg := sprintf(
        "RDB %s has public endpoint in deployed state",
        [instance.id]
    )
}
```

---

## Seed Generator (Phased Pipeline)

```go
type SeedGenerator interface {
    Generate(ctx context.Context, scenario Scenario, feedback *Feedback) (*GeneratedCode, error)
}

type GeneratedCode struct {
    Files map[string][]byte // filename → content (e.g. "main.tf" → bytes)
}
```

`Generate()` returns files as data — the harness writes them to `output/{scenario}/`. This keeps the interface uniform across implementations and easy to test (a `MockGenerator` for E2E tests just returns hardcoded files).

**Implementations**:
- `ClaudeCodeGenerator` — runs 3 sequential `claude -p` calls, captures stdout, parses JSON (phase 1) and `# File:` blocks (phases 2-3) into `GeneratedCode`. See [Shell Integration: Claude Code CLI Pipeline](#claude-code-cli-pipeline) for parsing details.
- Future: `OpenRouterGenerator` — calls OpenRouter API, parses code blocks from response, returns them

**Three phases per iteration** (3 `claude -p` calls):

| Phase | Prompt File | Input | Output |
|-------|------------|-------|--------|
| 1. Plan Architecture | `prompts/phase1_plan_architecture.md` | Scenario YAML + mappings | JSON architecture plan |
| 2. Generate HCL | `prompts/phase2_generate_hcl.md` | Architecture plan + constraints + overrides | OpenTofu .tf files |
| 3. Self Review | `prompts/phase3_self_review.md` | Generated files + acceptance criteria | Fixes applied to .tf files |

On feedback iterations (2+), all 3 phases receive the failure JSON as additional context via `{{.FeedbackJSON}}` in the prompt templates. The agent re-derives from scratch each iteration — no patching previous output. This keeps the pipeline consistent and avoids accumulating patches on patches.

Prompts stored as markdown with Go template interpolation. Template variables per phase:

| Variable | Phase 1 | Phase 2 | Phase 3 | Source |
|----------|---------|---------|---------|--------|
| `{{.ScenarioYAML}}` | Yes | Yes | Yes | Raw scenario YAML file content |
| `{{.Constraints}}` | Yes | Yes | Yes | Formatted constraint key-value pairs |
| `{{.ResolvedMappings}}` | Yes | — | — | Mappings after size resolution + override merge |
| `{{.Overrides}}` | Yes | — | — | Override block from scenario (conditional) |
| `{{.ArchitecturePlan}}` | — | Yes | — | Phase 1 stdout (JSON) |
| `{{.AcceptanceCriteria}}` | — | Yes | Yes | Formatted acceptance criteria list |
| `{{.GeneratedFiles}}` | — | — | Yes | Concatenated phase 2 output (all `# File:` blocks) |
| `{{.FeedbackJSON}}` | Yes* | Yes* | Yes* | Previous iteration's failure JSON (nil on iteration 1) |

\* Only present on iterations 2+. Wrapped in `{{if .FeedbackJSON}}` conditional blocks in each template.

**Rate limit mitigation**: configurable `agent.phase_delay_seconds` (default: 0) adds a pause between `claude -p` calls. Users hitting Max plan rate limits can set this to 5-10s.

---

## Shell Integration

The harness invokes three external tools. All `tofu` and `claude` commands run with the working directory set to `output/{scenario}/`.

### OpenTofu Commands

Environment variables set before every `tofu` command:

```
SCW_API_URL=http://localhost:8080           # From config: mockway.url
SCW_ACCESS_KEY=SCWXXXXXXXXXXXXXXXXX         # Fake — Mockway accepts any token
SCW_SECRET_KEY=00000000-0000-0000-0000-000000000000
SCW_DEFAULT_PROJECT_ID=00000000-0000-0000-0000-000000000000
```

| Command | Purpose | Output Handling |
|---------|---------|-----------------|
| `tofu init` | Initialize providers (cached in `.terraform/`) | Check exit code. Stderr on failure → feedback JSON. |
| `tofu validate` | HCL syntax + provider validation | Check exit code. Stderr on failure → feedback JSON. |
| `tofu plan -out=tfplan` | Generate execution plan | Check exit code. Stderr on failure → feedback JSON. |
| `tofu show -json tfplan` | Convert plan to JSON for OPA | Capture stdout → parse as JSON → pass to OPA as `input`. |
| `tofu apply -auto-approve` | Apply to Mockway | Check exit code. Stdout for resource count. Stderr on failure → feedback JSON. |
| `tofu destroy -auto-approve` | Tear down resources | Check exit code. Must succeed for destruction verification. |

**Error handling**: capture both stdout and stderr via separate pipes. Parse stderr for error messages. On non-zero exit code, extract the error detail and include it in the feedback JSON as a check failure with `"status": "fail"` and `"detail": "<stderr excerpt>"`.

**Provider cache**: `.terraform/providers/` is preserved between iterations (only `.tfstate` is deleted). This avoids re-downloading the Scaleway provider on each iteration.

### Claude Code CLI Pipeline

Each iteration runs 3 sequential `claude -p` calls. All output is captured from stdout.

```go
// Pseudocode for a single phase
func runPhase(prompt string, workDir string) (string, error) {
    cmd := exec.Command("claude", "-p", prompt)
    cmd.Dir = workDir
    out, err := cmd.Output()
    return string(out), err
}
```

**Phase 1 (Plan Architecture)**:
- Input: rendered `phase1_plan_architecture.md` template → passed to `claude -p` as the prompt argument
- Output: JSON architecture plan on stdout
- Parsing: find JSON object in stdout (handle potential preamble text), unmarshal to verify structure

**Phase 2 (Generate HCL)**:
- Input: rendered `phase2_generate_hcl.md` (with `{{.ArchitecturePlan}}` = phase 1 stdout)
- Output: `.tf` files on stdout, each prefixed with `# File: <filename>.tf`
- Parsing: split stdout on `# File: ` markers, extract filename and content into `map[string][]byte`

**Phase 3 (Self Review)**:
- Input: rendered `phase3_self_review.md` (with `{{.GeneratedFiles}}` = concatenated phase 2 output)
- Output: either `"NO ISSUES FOUND"` or corrected files with `# File:` headers
- Parsing: if stdout contains `"NO ISSUES FOUND"`, keep phase 2 files unchanged. Otherwise, parse `# File:` blocks and merge — corrected files overwrite, uncorrected files kept as-is from phase 2.

**File block parser** (used by phases 2 and 3):

```go
// parseFileBlocks splits stdout on "# File: " markers and extracts filename→content pairs.
// Strips markdown code fences (```hcl, ```) that Claude may wrap around file content.
func parseFileBlocks(stdout string) map[string][]byte {
    files := make(map[string][]byte)
    blocks := strings.Split(stdout, "# File: ")
    for _, block := range blocks[1:] { // skip preamble before first marker
        newline := strings.IndexByte(block, '\n')
        filename := strings.TrimSpace(block[:newline])
        content := block[newline+1:]
        content = stripCodeFences(content)
        files[filename] = []byte(content)
    }
    return files
}

// stripCodeFences removes leading ```hcl/```terraform/``` and trailing ``` markers.
// Also strips trailing blank lines that may appear after content or before fences.
func stripCodeFences(s string) string {
    lines := strings.Split(s, "\n")
    // Strip leading fence (e.g., "```hcl", "```terraform", "```")
    if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
        lines = lines[1:]
    }
    // Strip trailing blank lines and fences
    for len(lines) > 0 {
        trimmed := strings.TrimSpace(lines[len(lines)-1])
        if trimmed == "" || trimmed == "```" {
            lines = lines[:len(lines)-1]
        } else {
            break
        }
    }
    return strings.Join(lines, "\n")
}
```

### OPA Policy Evaluation

Uses the OPA Go SDK (`github.com/open-policy-agent/opa/rego`) for in-process evaluation. No external `opa` binary required.

**Layer 1 (static — against plan JSON)**:

```go
input := map[string]any{
    "planned_values": planJSON["planned_values"],
    "constraints":    scenario.Constraints,
    "target":         criterion.Target, // optional — scopes policy to a resource type (e.g., "database")
}
query := rego.New(
    rego.Query("data.scaleway.no_public_database.deny"),
    rego.Load(policyPaths, nil),
    rego.Input(input),
)
rs, _ := query.Eval(ctx)
// rs[0].Expressions[0].Value is a set of deny messages
```

**Layer 2 (mock state — against deployed state)**:

```go
// Fetch mock state from Mockway admin API
resp, _ := http.Get(mockwayURL + "/mock/state")
var mockState map[string]any
json.NewDecoder(resp.Body).Decode(&mockState)

input := map[string]any{
    "state":       mockState,
    "constraints": scenario.Constraints,
    "target":      criterion.Target, // optional — scopes policy to a resource type
}
query := rego.New(
    rego.Query("data.scaleway.no_public_database.deny_state"),
    rego.Load(policyPaths, nil),
    rego.Input(input),
)
```

**Policy discovery**: the harness loads all `.rego` files from `validation.layers.static.policy_paths`. For constraint-specific policies, it uses the `constraint_policies` map in config to determine which policies to evaluate based on the scenario's active constraints. Policies not referenced by any active constraint are still loaded (they may have unconditional rules).

**Collecting results**: each policy's `deny` (or `deny_state`) set is a collection of string messages. Empty set = pass. Non-empty set = fail. Messages are included in the feedback JSON's `checks` array with `"status": "fail"` and `"detail": "<deny message>"`.

---

## Run Store

Package: `internal/runstore/`

**v1**: Flat files in `.infrafactory/runs/{scenario}/{run-id}/` (e.g., `1.json`, `2.json` per iteration). Separate from `output/` so overwriting .tf files doesn't touch history.
**Future**: CXDB when continuous reconciliation demands it.

```go
type RunStore interface {
    CreateRun(ctx context.Context, scenario string) (RunID, error)
    AppendIteration(ctx context.Context, runID RunID, iteration Iteration) error
    GetIterations(ctx context.Context, runID RunID) ([]Iteration, error)
    GetLatestIteration(ctx context.Context, runID RunID) (*Iteration, error)
}
```

---

## Go Type Definitions

Core domain types used across packages. These are the key structs that an implementer needs to define.

```go
// RunID uniquely identifies a single factory run.
type RunID string

// Scenario is the parsed representation of a scenario YAML file.
// Package: internal/scenario/
type Scenario struct {
    Name               string                       `yaml:"scenario"`
    Version            string                       `yaml:"version"`
    Cloud              string                       `yaml:"cloud"`
    Description        string                       `yaml:"description"`
    Type               string                       `yaml:"type,omitempty"`       // "holdout" or empty
    References         string                       `yaml:"references,omitempty"` // criteria-only holdout
    Resources          *Resources                   `yaml:"resources,omitempty"`
    Constraints        map[string]any               `yaml:"constraints,omitempty"`
    AcceptanceCriteria []AcceptanceCriterion         `yaml:"acceptance_criteria"`
}

type Resources struct {
    Compute    *ComputeResource    `yaml:"compute,omitempty"`
    Networking *NetworkingResource `yaml:"networking,omitempty"`
    Database   *DatabaseResource   `yaml:"database,omitempty"`
    Kubernetes *KubernetesResource `yaml:"kubernetes,omitempty"`
    IAM        *IAMResource        `yaml:"iam,omitempty"`
    Registry   *RegistryResource   `yaml:"registry,omitempty"`
    Redis      *RedisResource      `yaml:"redis,omitempty"`
}

type ComputeResource struct {
    Purpose  string            `yaml:"purpose"`
    Size     string            `yaml:"size"`
    Count    int               `yaml:"count,omitempty"`
    Override map[string]string `yaml:"override,omitempty"`
}

type NetworkingResource struct {
    VPC            bool          `yaml:"vpc,omitempty"`
    PrivateNetwork bool          `yaml:"private_network,omitempty"`
    LoadBalancer   *LoadBalancer `yaml:"load_balancer,omitempty"`
}

type LoadBalancer struct {
    Exposure string    `yaml:"exposure"`
    Backends []Backend `yaml:"backends"`
}

type Backend struct {
    Port     int    `yaml:"port"`
    Protocol string `yaml:"protocol"`
}

type DatabaseResource struct {
    Engine           string            `yaml:"engine"`
    Size             string            `yaml:"size"`
    HighAvailability bool              `yaml:"high_availability,omitempty"`
    Override         map[string]string `yaml:"override,omitempty"`
}

type KubernetesResource struct {
    Size     string            `yaml:"size"`
    Override map[string]string `yaml:"override,omitempty"`
}

type IAMResource struct {
    Purpose     string `yaml:"purpose"`
    Application *bool  `yaml:"application,omitempty"` // default true
    APIKey      *bool  `yaml:"api_key,omitempty"`     // default true
    Policy      *bool  `yaml:"policy,omitempty"`      // default true
}

type RegistryResource struct {
    Purpose  string `yaml:"purpose"`
    IsPublic bool   `yaml:"is_public,omitempty"`
}

type RedisResource struct {
    Purpose  string            `yaml:"purpose"`
    Size     string            `yaml:"size"`
    Override map[string]string `yaml:"override,omitempty"`
}

type AcceptanceCriterion struct {
    Type        string `yaml:"type"`
    Expect      string `yaml:"expect"`
    Description string `yaml:"description,omitempty"`
    From        string `yaml:"from,omitempty"`        // connectivity
    To          string `yaml:"to,omitempty"`          // connectivity
    Port        int    `yaml:"port,omitempty"`        // connectivity, http_probe
    Target      string `yaml:"target,omitempty"`      // http_probe, policy
    Check       string `yaml:"check,omitempty"`       // policy
    Domain      string `yaml:"domain,omitempty"`      // dns_resolution
}

// Iteration captures the result of a single generate→validate cycle.
// Package: internal/feedback/
type Iteration struct {
    Number    int               `json:"iteration"`
    Converged bool              `json:"converged"`
    Layers    map[string]*Layer `json:"layers"`
    Files     map[string][]byte `json:"-"` // generated .tf files (not serialized to feedback JSON)
}

type Layer struct {
    Status string  `json:"status"` // "pass", "partial", "fail", "skipped"
    Reason string  `json:"reason,omitempty"`
    Checks []Check `json:"checks,omitempty"`
}

type Check struct {
    Name     string `json:"name"`
    Status   string `json:"status"` // "pass", "fail", "skipped"
    Detail   string `json:"detail,omitempty"`
    Resource string `json:"resource,omitempty"`
    File     string `json:"file,omitempty"`
}

// Feedback is the JSON structure passed back to the agent on iterations 2+.
// It wraps the scenario name and the last iteration's results.
type Feedback struct {
    Scenario string    `json:"scenario"`
    Iteration
}

// FailureSignature is used for stuck detection (subset comparison between iterations).
type FailureSignature struct {
    CheckName string
    Resource  string
}
```

---

## Mockway (Separate Repo)

**Repo**: [github.com/redscaresu/mockway](https://github.com/redscaresu/mockway)

**Binary**: `mockway --port 8080` (default: in-memory SQLite, use `--db ./mockway.db` for file-based debugging)

**Build order**: Mockway is built and published first as a standalone tool. InfraFactory imports it as a dependency / uses it via Docker.

**Main API** (port 8080): Single HTTP server, path-based routing. Auth: accepts any `X-Auth-Token`.

| Service | Path Prefix | Key Endpoints |
|---------|-------------|---------------|
| Instance | `/instance/v1/zones/{zone}/` | servers, ips, security_groups, private_nics, volumes, products/servers catalog, user_data, server actions |
| VPC | `/vpc/v1/regions/{region}/` + `/vpc/v2/regions/{region}/` | vpcs, private-networks, routes (routes on v2 only) |
| VPC Gateways | `/vpc-gw/v2/zones/{zone}/` | gateways, gateway-networks |
| Load Balancer | `/lb/v1/zones/{zone}/` + `/lb/v1/regions/{region}/` | lbs, frontends, backends, private_networks, acls, routes, certificates, health checks |
| Kubernetes | `/k8s/v1/regions/{region}/` | clusters, pools, nodes, versions, upgrade, set-type, kubeconfig |
| RDB | `/rdb/v1/regions/{region}/` | instances, databases, users, privileges, acls, settings, read-replicas, snapshots, backups, endpoints, certificates, logs |
| IAM | `/iam/v1alpha1/` | applications, api-keys, policies, rules, ssh-keys, users, groups |
| Container Registry | `/registry/v1/regions/{region}/` | namespaces |
| Redis | `/redis/v1/zones/{zone}/` | clusters, versions, node-types, acl-rules, endpoints, settings, certificates |
| Block Storage | `/block/v1alpha1/zones/{zone}/` | volumes, snapshots, volume-types |
| IPAM | `/ipam/v1/regions/{region}/` | ips (create, list, update, delete, detach, move) |
| Domain/DNS | `/domain/v2beta1/` | dns-zones, dns-records |
| Marketplace | `/marketplace/v2/` | local-images (known label → zone-specific UUID resolution, unknown labels rejected) |
| Account (legacy) | `/account/v2alpha1/` | ssh-keys (alias → IAM ssh-keys state) |

**Scope**: 13 services + 1 legacy alias, 42+ resource types, ~250 handler methods + 5 admin endpoints + 1 catch-all (UnimplementedHandler). Hand-written handlers derived from studying the Terraform SDK and Scaleway API spec. API fidelity verified through systematic codex review loops (14 bug patterns documented). No S3 in v1.

### Referential Integrity

Mockway enforces the same referential integrity as the real Scaleway API:

- **On create**: validate that referenced resources exist via SQLite FK constraints. Creating a `private_nic` with a non-existent `server_id` → 404 Not Found. Creating an LB route with frontend/backend from different LBs → 400 Bad Request. Creating an LB with an already-attached `ip_id` → rejected. Unknown marketplace image labels → empty list (catches typos like `ubuntu_jammyy`).
- **On sub-resource operations**: validate parent exists before proceeding. Listing databases for a non-existent RDB instance → 404. Setting ACLs on a non-existent instance → 404. Getting a NIC via `/servers/{server_id}/private_nics/{nic_id}` validates the NIC belongs to that server.
- **On delete**: reject if dependents still exist (409 Conflict). Delete a VPC when private networks are still attached → 409. Delete an LB when frontends or backends exist → 409. Delete an LB IP while attached to an LB → rejected.
- **On delete (cascade/detach exceptions)**: server deletion detaches IPs and security group references (`ON DELETE SET NULL`) and cascades private NICs (`ON DELETE CASCADE`). LB deletion cascades `lb_private_networks` attachment records, routes, and certificates, and detaches LB IPs. Frontends and backends are NOT cascaded — they block with 409. K8s cluster deletion cascades pools. RDB instance deletion cascades databases, users, privileges, ACLs, read replicas, snapshots, and backups.
- **On update**: `patchMerge` helper preserves existing fields, skips nil values, deep-merges nested maps one level. All Update functions sync extracted SQL FK columns.
- **`POST /mock/reset`**: wipes all state (including marketplace cache). Disables FK checks, deletes all rows, re-enables FKs.
- **`POST /mock/snapshot`** / **`POST /mock/restore`**: snapshot and restore full state for iteration baselines.
- **`GET /mock/state/{service}`**: per-service state queries for targeted inspection.

### Key Resource Relationships

```
VPC
 ├── Private Network
 │    ├── Instance Private NIC → Instance Server
 │    ├── RDB Instance (private endpoint — JSON ref, not FK)
 │    ├── LB Private Network attachment
 │    └── VPC Gateway Network → VPC Public Gateway
 └── VPC Route

Instance Server
 ├── Instance IP (public, optional — ON DELETE SET NULL)
 ├── Instance Private NIC → Private Network (ON DELETE CASCADE)
 ├── Instance Volume (embedded in server JSON — no FK, standalone table)
 └── Instance Security Group (ON DELETE SET NULL)

Block Volume (standalone — zone-scoped)
 └── Block Snapshot (volume_id FK)

Load Balancer
 ├── LB IP (separate table, detached on LB delete)
 ├── LB Frontend → LB ACL (ON DELETE CASCADE)
 ├── LB Backend
 ├── LB Route → Frontend + Backend (same-LB validated)
 ├── LB Certificate (ON DELETE CASCADE)
 └── LB Private Network attachment (cascaded on LB delete)

K8s Cluster (ON DELETE CASCADE → pools)
 ├── K8s Node Pool → synthesised Nodes
 └── Private Network (optional)

RDB Instance (ON DELETE CASCADE → databases, users, privileges, ACLs, read replicas, snapshots, backups)
 ├── RDB Database
 ├── RDB User
 ├── RDB Privilege
 ├── RDB ACL
 ├── RDB Read Replica → RDB Endpoint
 ├── RDB Snapshot
 └── RDB Backup

IAM Application
 ├── IAM API Key (access_key is the PK, not UUID; application_id optional, user_id validated)
 └── IAM Policy → IAM Rules (ON DELETE CASCADE)

IAM Group → IAM Group Members (ON DELETE CASCADE)

IAM SSH Key (standalone — no parent dependency)
 └── Also accessible via Account legacy routes (same state)

DNS Zone → DNS Records (cascaded on zone delete)

Container Registry Namespace (standalone — region-scoped)

Redis Cluster (standalone — zone-scoped)

IPAM IP (standalone — region-scoped)
```

### SQLite Schema

Per-type tables with JSON blob for full resource data, extracted FK columns for integrity:

```sql
-- VPC
CREATE TABLE vpcs (id TEXT PRIMARY KEY, region TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE private_networks (id TEXT PRIMARY KEY, vpc_id TEXT NOT NULL REFERENCES vpcs(id), region TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE vpc_routes (id TEXT PRIMARY KEY, vpc_id TEXT NOT NULL REFERENCES vpcs(id), region TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE vpc_public_gateways (id TEXT PRIMARY KEY, zone TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE vpc_gateway_networks (id TEXT PRIMARY KEY, gateway_id TEXT NOT NULL REFERENCES vpc_public_gateways(id), private_network_id TEXT NOT NULL REFERENCES private_networks(id), data JSON NOT NULL);

-- Instance
CREATE TABLE instance_security_groups (id TEXT PRIMARY KEY, zone TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE instance_servers (id TEXT PRIMARY KEY, zone TEXT NOT NULL, security_group_id TEXT REFERENCES instance_security_groups(id) ON DELETE SET NULL, data JSON NOT NULL);
CREATE TABLE instance_ips (id TEXT PRIMARY KEY, server_id TEXT REFERENCES instance_servers(id) ON DELETE SET NULL, zone TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE instance_private_nics (id TEXT PRIMARY KEY, server_id TEXT NOT NULL REFERENCES instance_servers(id) ON DELETE CASCADE, private_network_id TEXT NOT NULL REFERENCES private_networks(id), zone TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE instance_volumes (id TEXT PRIMARY KEY, zone TEXT NOT NULL, data JSON NOT NULL);

-- Load Balancer
CREATE TABLE lb_ips (id TEXT PRIMARY KEY, zone TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE lbs (id TEXT PRIMARY KEY, zone TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE lb_frontends (id TEXT PRIMARY KEY, lb_id TEXT NOT NULL REFERENCES lbs(id), data JSON NOT NULL);
CREATE TABLE lb_backends (id TEXT PRIMARY KEY, lb_id TEXT NOT NULL REFERENCES lbs(id), data JSON NOT NULL);
CREATE TABLE lb_private_networks (lb_id TEXT NOT NULL REFERENCES lbs(id), private_network_id TEXT NOT NULL REFERENCES private_networks(id), data JSON NOT NULL, PRIMARY KEY (lb_id, private_network_id));
CREATE TABLE lb_acls (id TEXT PRIMARY KEY, frontend_id TEXT NOT NULL REFERENCES lb_frontends(id) ON DELETE CASCADE, data JSON NOT NULL);
CREATE TABLE lb_routes (id TEXT PRIMARY KEY, lb_id TEXT NOT NULL REFERENCES lbs(id) ON DELETE CASCADE, data JSON NOT NULL);
CREATE TABLE lb_certificates (id TEXT PRIMARY KEY, lb_id TEXT NOT NULL REFERENCES lbs(id) ON DELETE CASCADE, data JSON NOT NULL);

-- Kubernetes
CREATE TABLE k8s_clusters (id TEXT PRIMARY KEY, region TEXT NOT NULL, private_network_id TEXT REFERENCES private_networks(id), data JSON NOT NULL);
CREATE TABLE k8s_pools (id TEXT PRIMARY KEY, cluster_id TEXT NOT NULL REFERENCES k8s_clusters(id) ON DELETE CASCADE, region TEXT NOT NULL, data JSON NOT NULL);

-- RDB
CREATE TABLE rdb_instances (id TEXT PRIMARY KEY, region TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE rdb_databases (instance_id TEXT NOT NULL REFERENCES rdb_instances(id) ON DELETE CASCADE, name TEXT NOT NULL, data JSON NOT NULL, PRIMARY KEY (instance_id, name));
CREATE TABLE rdb_users (instance_id TEXT NOT NULL REFERENCES rdb_instances(id) ON DELETE CASCADE, name TEXT NOT NULL, data JSON NOT NULL, PRIMARY KEY (instance_id, name));
CREATE TABLE rdb_privileges (instance_id TEXT NOT NULL REFERENCES rdb_instances(id) ON DELETE CASCADE, user_name TEXT NOT NULL, database_name TEXT NOT NULL, data JSON NOT NULL, PRIMARY KEY (instance_id, user_name, database_name));
CREATE TABLE rdb_acls (instance_id TEXT PRIMARY KEY REFERENCES rdb_instances(id) ON DELETE CASCADE, data JSON NOT NULL);
CREATE TABLE rdb_read_replicas (id TEXT PRIMARY KEY, instance_id TEXT NOT NULL REFERENCES rdb_instances(id) ON DELETE CASCADE, region TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE rdb_snapshots (id TEXT PRIMARY KEY, instance_id TEXT NOT NULL REFERENCES rdb_instances(id) ON DELETE CASCADE, region TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE rdb_backups (id TEXT PRIMARY KEY, instance_id TEXT NOT NULL REFERENCES rdb_instances(id) ON DELETE CASCADE, region TEXT NOT NULL, data JSON NOT NULL);

-- IAM (organisation-scoped — no zone/region column)
CREATE TABLE iam_applications (id TEXT PRIMARY KEY, data JSON NOT NULL);
CREATE TABLE iam_api_keys (access_key TEXT PRIMARY KEY, application_id TEXT REFERENCES iam_applications(id), data JSON NOT NULL);
CREATE TABLE iam_policies (id TEXT PRIMARY KEY, application_id TEXT REFERENCES iam_applications(id), data JSON NOT NULL);
CREATE TABLE iam_rules (id TEXT PRIMARY KEY, policy_id TEXT NOT NULL REFERENCES iam_policies(id) ON DELETE CASCADE, data JSON NOT NULL);
CREATE TABLE iam_ssh_keys (id TEXT PRIMARY KEY, data JSON NOT NULL);
CREATE TABLE iam_users (id TEXT PRIMARY KEY, data JSON NOT NULL);
CREATE TABLE iam_groups (id TEXT PRIMARY KEY, data JSON NOT NULL);
CREATE TABLE iam_group_members (group_id TEXT NOT NULL REFERENCES iam_groups(id) ON DELETE CASCADE, user_id TEXT NOT NULL REFERENCES iam_users(id) ON DELETE CASCADE, PRIMARY KEY (group_id, user_id));

-- Block Storage
CREATE TABLE block_volumes (id TEXT PRIMARY KEY, zone TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE block_snapshots (id TEXT PRIMARY KEY, zone TEXT NOT NULL, volume_id TEXT REFERENCES block_volumes(id), data JSON NOT NULL);

-- DNS
CREATE TABLE dns_zones (dns_zone TEXT PRIMARY KEY, domain TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE domain_records (id TEXT PRIMARY KEY, dns_zone TEXT NOT NULL, data JSON NOT NULL);

-- Other
CREATE TABLE registry_namespaces (id TEXT PRIMARY KEY, region TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE redis_clusters (id TEXT PRIMARY KEY, zone TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE ipam_ips (id TEXT PRIMARY KEY, region TEXT NOT NULL, data JSON NOT NULL);
CREATE TABLE marketplace_labels (label TEXT PRIMARY KEY);
CREATE TABLE schema_versions (version INTEGER PRIMARY KEY);
```

SQLite enforces FKs natively (`PRAGMA foreign_keys = ON`). The JSON `data` column holds the full API response shape — flexible, no migrations when Scaleway adds optional fields.

### Admin State API

The admin state schema is a **versioned contract** between Mockway and InfraFactory.

**Endpoints**:
- `POST /mock/reset` — wipe all state (including marketplace cache)
- `GET /mock/state` — full resource graph as JSON
- `GET /mock/state/{service}` — single service state (valid: `instance`, `vpc`, `lb`, `k8s`, `rdb`, `iam`, `registry`, `redis`, `block`, `ipam`, `domain`; unknown → 404)
- `POST /mock/snapshot` — snapshot current state for later restore
- `POST /mock/restore` — restore to last snapshot (404 if no snapshot exists)

**Schema** (`GET /mock/state`):

```json
{
  "instance": {
    "servers": [
      {"id": "uuid", "zone": "fr-par-1", "name": "web-1", "commercial_type": "DEV1-S",
       "public_ip": null, "private_nics": ["nic-uuid-1"]}
    ],
    "ips": [
      {"id": "uuid", "address": "51.15.x.x", "server_id": null}
    ],
    "private_nics": [
      {"id": "nic-uuid-1", "server_id": "uuid", "private_network_id": "pn-uuid"}
    ],
    "security_groups": [
      {"id": "uuid", "zone": "fr-par-1", "inbound_default_policy": "drop"}
    ],
    "volumes": []
  },
  "vpc": {
    "vpcs": [
      {"id": "uuid", "region": "fr-par", "name": "main"}
    ],
    "private_networks": [
      {"id": "pn-uuid", "vpc_id": "uuid", "region": "fr-par", "name": "app-network"}
    ],
    "routes": [],
    "gateways": [],
    "gateway_networks": []
  },
  "lb": {
    "lbs": [
      {"id": "uuid", "zone": "fr-par-1", "name": "web-lb",
       "ip": [{"id": "uuid", "ip_address": "51.15.x.x", "lb_id": "uuid"}]}
    ],
    "ips": [
      {"id": "uuid", "ip_address": "51.15.x.x", "lb_id": "uuid"}
    ],
    "frontends": [
      {"id": "uuid", "lb_id": "uuid", "name": "http", "inbound_port": 80,
       "backend_id": "be-uuid"}
    ],
    "backends": [
      {"id": "be-uuid", "lb_id": "uuid", "name": "web-servers",
       "forward_port": 80, "server_ips": ["10.0.0.1", "10.0.0.2"]}
    ],
    "acls": [],
    "routes": [],
    "certificates": [],
    "private_networks": [
      {"lb_id": "uuid", "private_network_id": "pn-uuid"}
    ]
  },
  "k8s": {
    "clusters": [
      {"id": "uuid", "region": "fr-par", "name": "kapsule-1", "status": "ready",
       "private_network_id": "pn-uuid"}
    ],
    "pools": [
      {"id": "uuid", "cluster_id": "uuid", "name": "default",
       "node_type": "DEV1-M", "size": 3}
    ]
  },
  "rdb": {
    "instances": [
      {"id": "uuid", "region": "fr-par", "name": "app-db", "engine": "PostgreSQL-15",
       "node_type": "DB-DEV-S",
       "endpoints": [
         {"ip": "10.0.0.5", "port": 5432, "private_network": {"id": "pn-uuid"}}
       ]}
    ],
    "databases": [
      {"instance_id": "uuid", "name": "appdb"}
    ],
    "users": [
      {"instance_id": "uuid", "name": "appuser"}
    ],
    "privileges": [],
    "read_replicas": [],
    "snapshots": [],
    "backups": []
  },
  "iam": {
    "applications": [
      {"id": "uuid", "name": "my-app", "description": "CI/CD application"}
    ],
    "api_keys": [
      {"access_key": "SCWxxxxxxxxxxxxxxxxx", "application_id": "uuid",
       "description": "deploy key"}
    ],
    "policies": [
      {"id": "uuid", "name": "full-access", "application_id": "uuid"}
    ],
    "ssh_keys": [
      {"id": "uuid", "name": "my-laptop",
       "public_key": "ssh-ed25519 AAAA..."}
    ],
    "users": [],
    "groups": []
  },
  "registry": {
    "namespaces": [
      {"id": "uuid", "region": "fr-par", "name": "my-registry",
       "is_public": false, "organization_id": "org-uuid"}
    ]
  },
  "redis": {
    "clusters": [
      {"id": "uuid", "zone": "fr-par-1", "name": "my-cache",
       "version": "7.0.12", "node_type": "RED1-MICRO",
       "cluster_size": 1}
    ]
  },
  "block": {
    "volumes": [],
    "snapshots": []
  },
  "ipam": {
    "ips": []
  },
  "domain": {
    "dns_zones": [],
    "records": []
  }
}
```

This schema mirrors the Scaleway API response shapes. The TopologyEvaluator in InfraFactory consumes this to build the connectivity graph.

**Mockway v2** (future): S3 on second port (Azurite-inspired), deeper resource simulation (K8s cluster lifecycle, LB health checks), eventual consistency simulation.

**Distribution**: GoReleaser → Go binaries + Docker (`ghcr.io/redscaresu/mockway`) + Homebrew (`brew install redscaresu/tap/mockway`)

---

## Continuous Reconciliation (Target Architecture, Not v1)

**v1 (Model A)**: User updates scenario YAML → re-runs `infrafactory run` → agent regenerates → harness validates.

**Target (Model B)**: InfraFactory monitors deployed infrastructure and reconciles drift.

```
Scheduled Reconciler → Drift Detection (tofu plan against live state)
  → No drift: sleep
  → Drift found: Classify → Expected: accept / Unexpected: re-run factory
```

Requires (future): state tracking via ContextStore (CXDB), scheduled `tofu plan`, drift classification, auto-remediation, alerting.

---

## Distribution

| Channel | What | How |
|---------|------|-----|
| GitHub Releases | Compiled Go binaries (linux/darwin, amd64/arm64) | GoReleaser on tag push |
| Docker (GHCR) | `ghcr.io/redscaresu/mockway` (mock), `ghcr.io/redscaresu/infrafactory` (CLI) | GoReleaser |
| Homebrew | `brew install redscaresu/tap/infrafactory`, `brew install redscaresu/tap/mockway` | GoReleaser → tap repo |
| `go install` | `go install github.com/redscaresu/infrafactory/cmd/infrafactory@latest` | Standard Go |

---

## Testing Strategy

### Unit Tests (no external deps)
- Scenario YAML parsing and JSON Schema validation
- Size mapping resolution
- Feedback JSON serialization/deserialization
- Harness layer chaining logic
- OPA policy evaluation against fixture plan JSON
- Config file parsing
- Prompt template rendering

### Integration Tests (require Docker: Mockway)
- `tofu apply` against Mockway → verify resource state via admin API
- Full Layer 1 + Layer 2 with real tools
- Acceptance criteria checks against mock state

### E2E Tests (full pipeline)
- **Deterministic E2E**: mock agent returns pre-written OpenTofu → full validate → converge
- **Live E2E** (slow, CI schedule): actually calls `claude -p` for a simple scenario

---

## Toolchain

| Component | Tool | Cost |
|-----------|------|------|
| IaC | OpenTofu | Free (MPL-2.0) |
| Scaleway Provider | `scaleway/scaleway` (OpenTofu registry) | Free |
| Policy Engine | OPA / Rego | Free |
| Mock Server | [Mockway](https://github.com/redscaresu/mockway) | Free (ours) |
| Agent (initial) | Claude Code CLI (`claude -p`) | Covered by Max plan |
| Agent (future) | OpenRouter API | Pay-per-token |
| CLI Framework | Cobra | Free |
| CI/CD | GitHub Actions | Free tier |

---

## Go Dependencies

```
github.com/spf13/cobra                   # CLI framework
github.com/open-policy-agent/opa         # OPA Go SDK (rego package — in-process policy evaluation)
github.com/santhosh-tekuri/jsonschema/v6 # JSON Schema validation for scenario YAML
gopkg.in/yaml.v3                         # YAML parsing (scenarios, mappings, config)
```

Standard library packages used extensively: `os/exec` (tofu + claude invocation), `encoding/json`, `text/template` (prompt rendering), `net/http` (Mockway admin API client), `path/filepath`, `os`.

No other external dependencies required for v1. UUID generation is only needed in Mockway (not InfraFactory).

---

## What's NOT in v1

- Layer 3: Sandbox Deploy (real Scaleway — requires real credentials, sandbox project, network probes, cost awareness)
- Multi-cloud support (AWS, Azure, GCP)
- Continuous reconciliation / drift detection (Model B)
- CXDB integration (flat files first, ContextStore interface ready)
- StrongDM ID / agentic auth
- Production promotion / graduated deployment
- Gene transfusion from exemplars
- Pyramid summaries
- GUI / web interface
- LLM-as-Judge satisfaction testing
- Scenario mixins / composition
- Full Attractor graph runner
- Cost estimation / budget checks (no reliable Scaleway pricing data source)
- S3 mock (Mockway v2)
- tfsec / checkov (poor Scaleway support — add when they gain rules)
- Infracost (no Scaleway pricing support)

These are designed-for but deferred, with interfaces ready for future integration.
