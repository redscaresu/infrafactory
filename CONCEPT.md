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
| Mock scope | 7 services + 1 legacy alias (Instance, VPC, LB, K8s, RDB, IAM, Marketplace + Account legacy) | Breadth over depth. 19 resource types, ~91 handler methods + 3 admin endpoints + 1 catch-all. S3 deferred to Mockway v2. |
| Mock generation | Codegen-first (`oapi-codegen`) with hand-written fallback | Scaleway specs include custom `x-one-of`; pre-process to standard `oneOf` and use codegen when viable, otherwise hand-write types/routes from spec references. |
| Mock HTTP router | chi | Lightweight, stdlib-compatible, path-parameter routing. No framework lock-in. |
| Mock testing | Unit + integration in Mockway repo | Unit: SQLite store, FK validation, handler logic. Integration: HTTP tests against running server (create/read/delete/list, FK rejection). |
| Mock state model | SQLite with FK constraints + referential integrity | On create: validate referenced resources exist (404 if not). On delete: reject if dependents exist (409), cascade or detach where Scaleway does (e.g. server delete detaches IPs, cascades NICs). Matches real Scaleway API behaviour. |
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
| CLI scenario arg | File path, not name | `infrafactory run scenarios/training/web-app-paris.yaml` — explicit, no ambiguity, tab-completable. |
| CLI init | Minimal scaffold | Creates skeleton YAML with required fields + comments. Prints "next steps" suggesting commands to run. No interactive wizard for v1. |
| Holdout formats | Both criteria-only and full scenario | Criteria-only: references training scenario, adds adversarial acceptance criteria against same generated code. Full: independent scenario with stricter criteria. |
| Provider URL injection | `SCW_API_URL` env var | Harness sets `SCW_API_URL=http://localhost:8080` before any `tofu` command. Agent generates normal provider blocks. Zero coupling. |
| Iteration state | Clean slate each iteration | At iteration start (before Layer 1): `tofu destroy` (if state exists, uses old .tf files) → `mockway reset` → delete `.tfstate` (keep `.terraform/providers/` cached) → write new files → `tofu init`. Must precede `tofu plan` which needs init + clean state. |
| Generator interface | Returns files, doesn't write them | `SeedGenerator.Generate()` returns `GeneratedCode` with `map[string][]byte`. ClaudeCodeGenerator: captures stdout from `claude -p`, parses `# File:` blocks. OpenRouterGenerator: parses API response. Harness writes files. Uniform, testable. |
| Mock start command | Docker wrapper | `infrafactory mock start` runs `docker run ghcr.io/redscaresu/mockway`. Consistent with docker-compose workflow. |
| Output on re-run | Overwrite | Wipe `output/{scenario}/` and start fresh. Run store keeps iteration history separately. `.tf` files are always the latest. |
| Regression scenarios | Promoted training scenarios | Once a training scenario converges reliably, promote to `scenarios/regression/`. CI runs all regression scenarios on every change (prompt updates, policy changes). |
| Holdout discovery | Scan `scenarios/holdout/` for criteria-only | After convergence, scan holdout dir for criteria-only holdouts whose `references:` matches the training path. Full holdouts are explicit only (see below). |
| Mock credentials | Fake env vars for provider init | Harness sets dummy `SCW_ACCESS_KEY`, `SCW_SECRET_KEY`, `SCW_DEFAULT_PROJECT_ID`. Mockway accepts any token. |
| OPA in Layer 2 | `deny_state` runs after TopologyEvaluator | Layer 2 flow: `tofu apply` → TopologyEvaluator (connectivity) → OPA `deny_state` (policy against mock state). Sequential. |
| Output dir naming | From YAML `scenario:` field | `scenario: web-app-paris` → `output/web-app-paris/`. Canonical, independent of filename. |
| Full holdout trigger | Explicit only | Full holdouts run via `infrafactory run scenarios/holdout/file.yaml`. Only criteria-only holdouts auto-discover after training convergence. |
| Run store location | `.infrafactory/runs/` | Separate from output dir. `output/{scenario}/` is purely .tf files (overwritten). `.infrafactory/runs/{scenario}/{run-id}/` keeps iteration history. |
| Policy `target` field | Passed to OPA as input | Harness passes `target` to OPA input. Policies can optionally filter by target resource type. Without target, checks all resources. |
| Data sources | Prompt constraint: avoid | Prompts tell agent not to use `data` blocks — use hardcoded IDs from mappings. If agent uses them anyway, `tofu plan` fails and feedback loop corrects it. |
| Max iterations | Configurable, default 5 | Most scenarios converge in 2-3. If not by 5, needs human attention. |

---

## Scaleway Resources in Scope (v1)

| Category | Resources | API Pattern |
|----------|-----------|-------------|
| Compute | Instances, IPs, Security Groups, Private NICs, Volumes | Zoned: `/instance/v1/zones/{zone}/servers` |
| Networking | VPC, Private Networks | Regional: `/vpc/v1/regions/{region}/` |
| Load Balancing | Load Balancers, Frontends, Backends | Zoned: `/lb/v1/zones/{zone}/lbs` |
| Kubernetes | Kapsule clusters + node pools | Regional: `/k8s/v1/regions/{region}/clusters` |
| Databases | Managed PostgreSQL, MySQL | Regional: `/rdb/v1/regions/{region}/instances` |
| IAM | Applications, API Keys, Policies, SSH Keys | Organisation-scoped: `/iam/v1alpha1/` |
| Marketplace | Local Images (image label → UUID resolution) | `/marketplace/v2/` |
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
│  Layer 3: Sandbox Deploy (NOT IN V1 — stub only) │
│    tofu apply → real Scaleway sandbox project    │
│    Real network probes: connectivity, DNS        │
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
scaleway_infra_factory/
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
| `scaleway.sandbox_project_id` | string | No | `""` | Scaleway project ID for Layer 3 sandbox deploy. Empty disables Layer 3. |
| `validation.layers.static.enabled` | boolean | No | `true` | Enable Layer 1 (tofu validate/plan + OPA). |
| `validation.layers.static.policy_paths` | []string | No | `[]` | Directories containing OPA `.rego` files. |
| `validation.layers.mock_deploy.enabled` | boolean | No | `true` | Enable Layer 2 (tofu apply → Mockway). |
| `validation.layers.sandbox_deploy.enabled` | boolean | No | `false` | Enable Layer 3. **Not implemented in v1** — always set to `false`. |
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
- Layer 3 (sandbox) is opt-in, gated by config. **Not implemented in v1** — returns skip. Interface exists for future implementation
- Layer 4 (destruction) runs if anything was deployed
- `dns_resolution` acceptance criteria are **skipped** when sandbox deploy is disabled (they require real DNS). Skipped checks appear in feedback JSON with `"status": "skipped"` — they don't block convergence

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
| Instance | `/instance/v1/zones/{zone}/` | servers, ips, security_groups, private_nics, volumes, products/servers catalog, user_data stubs, server action |
| VPC | `/vpc/v1/regions/{region}/` | vpcs, private-networks |
| Load Balancer | `/lb/v1/zones/{zone}/` | lbs, frontends, backends, private_networks |
| Kubernetes | `/k8s/v1/regions/{region}/` | clusters, pools |
| RDB | `/rdb/v1/regions/{region}/` | instances, databases, users |
| IAM | `/iam/v1alpha1/` | applications, api-keys, policies, ssh-keys, rules |
| Marketplace | `/marketplace/v2/` | local-images (image label → zone-specific UUID resolution) |
| Account (legacy) | `/account/v2alpha1/` | ssh-keys (alias → IAM ssh-keys state) |

**Scope**: 7 services + 1 legacy alias, 19 resource types, ~91 handler methods + 3 admin endpoints + 1 catch-all (UnimplementedHandler). Codegen-first (`oapi-codegen`) with hand-written fallback when spec quirks (for example `x-one-of`) block usable generation. No S3 in v1.

### Referential Integrity

Mockway enforces the same referential integrity as the real Scaleway API:

- **On create**: validate that referenced resources exist. Creating a `private_nic` with a non-existent `server_id` → 404 Not Found. Creating an RDB instance with a `private_network_id` that doesn't exist → 404 Not Found. Creating an IAM API key with a non-existent `application_id` → 404 Not Found.
- **On delete**: reject if dependents still exist. Delete a VPC when private networks are still attached → 409 Conflict. Delete a private network when NICs are attached → 409 Conflict. Delete an IAM application with attached API keys or policies → 409 Conflict.
- **On delete (cascade/detach)**: some FKs use `ON DELETE SET NULL` (server deletion detaches IPs and security group references) or `ON DELETE CASCADE` (server deletion cascades to private NICs). This matches the destroy ordering the Terraform provider expects.
- **`POST /mock/reset`**: wipes all state. Disables FK checks (`PRAGMA foreign_keys = OFF`), deletes all rows, re-enables FKs.

### Key Resource Relationships

```
VPC
 └── Private Network
      ├── Instance Private NIC → Instance Server
      ├── RDB Instance (private endpoint)
      └── LB Private Network attachment

Instance Server
 ├── Instance IP (public, optional — ON DELETE SET NULL)
 ├── Instance Private NIC → Private Network (ON DELETE CASCADE)
 └── Instance Security Group (ON DELETE SET NULL)

Load Balancer
 ├── LB IP (public, auto-generated on LB create — stored inline in LB data JSON, no separate table)
 ├── LB Frontend (inbound port)
 ├── LB Backend (forward port, server IPs)
 └── LB Private Network attachment

K8s Cluster
 ├── K8s Node Pool
 └── Private Network (optional)

IAM Application
 ├── IAM API Key (access_key is the PK, not UUID; application_id optional)
 └── IAM Policy (optional application_id FK)

IAM SSH Key (standalone — no parent dependency)
 └── Also accessible via Account legacy routes (same state)
```

### SQLite Schema

Per-type tables with JSON blob for full resource data, extracted FK columns for integrity:

```sql
-- VPC
CREATE TABLE vpcs (
    id TEXT PRIMARY KEY,
    region TEXT NOT NULL,
    data JSON NOT NULL
);

CREATE TABLE private_networks (
    id TEXT PRIMARY KEY,
    vpc_id TEXT NOT NULL REFERENCES vpcs(id),
    region TEXT NOT NULL,
    data JSON NOT NULL
);

-- Instance (security_groups first — referenced by servers)
CREATE TABLE instance_security_groups (
    id TEXT PRIMARY KEY,
    zone TEXT NOT NULL,
    data JSON NOT NULL
);

CREATE TABLE instance_servers (
    id TEXT PRIMARY KEY,
    zone TEXT NOT NULL,
    security_group_id TEXT REFERENCES instance_security_groups(id) ON DELETE SET NULL,
    data JSON NOT NULL
);

CREATE TABLE instance_ips (
    id TEXT PRIMARY KEY,
    server_id TEXT REFERENCES instance_servers(id) ON DELETE SET NULL,
    zone TEXT NOT NULL,
    data JSON NOT NULL
);

CREATE TABLE instance_private_nics (
    id TEXT PRIMARY KEY,
    server_id TEXT NOT NULL REFERENCES instance_servers(id) ON DELETE CASCADE,
    private_network_id TEXT NOT NULL REFERENCES private_networks(id),
    zone TEXT NOT NULL,
    data JSON NOT NULL
);

-- Load Balancer
CREATE TABLE lbs (
    id TEXT PRIMARY KEY,
    zone TEXT NOT NULL,
    data JSON NOT NULL
);

CREATE TABLE lb_frontends (
    id TEXT PRIMARY KEY,
    lb_id TEXT NOT NULL REFERENCES lbs(id),
    data JSON NOT NULL
);

CREATE TABLE lb_backends (
    id TEXT PRIMARY KEY,
    lb_id TEXT NOT NULL REFERENCES lbs(id),
    data JSON NOT NULL
);

CREATE TABLE lb_private_networks (
    lb_id TEXT NOT NULL REFERENCES lbs(id),
    private_network_id TEXT NOT NULL REFERENCES private_networks(id),
    data JSON NOT NULL,
    PRIMARY KEY (lb_id, private_network_id)
);

-- Kubernetes
CREATE TABLE k8s_clusters (
    id TEXT PRIMARY KEY,
    region TEXT NOT NULL,
    private_network_id TEXT REFERENCES private_networks(id),
    data JSON NOT NULL
);

CREATE TABLE k8s_pools (
    id TEXT PRIMARY KEY,
    cluster_id TEXT NOT NULL REFERENCES k8s_clusters(id),
    region TEXT NOT NULL,
    data JSON NOT NULL
);

-- RDB
CREATE TABLE rdb_instances (
    id TEXT PRIMARY KEY,
    region TEXT NOT NULL,
    data JSON NOT NULL
);

CREATE TABLE rdb_databases (
    instance_id TEXT NOT NULL REFERENCES rdb_instances(id),
    name TEXT NOT NULL,
    data JSON NOT NULL,
    PRIMARY KEY (instance_id, name)
);

CREATE TABLE rdb_users (
    instance_id TEXT NOT NULL REFERENCES rdb_instances(id),
    name TEXT NOT NULL,
    data JSON NOT NULL,
    PRIMARY KEY (instance_id, name)
);

-- IAM (organisation-scoped — no zone/region column)
CREATE TABLE iam_applications (
    id TEXT PRIMARY KEY,
    data JSON NOT NULL
);

CREATE TABLE iam_api_keys (
    access_key TEXT PRIMARY KEY,
    application_id TEXT REFERENCES iam_applications(id),
    data JSON NOT NULL
);

CREATE TABLE iam_policies (
    id TEXT PRIMARY KEY,
    application_id TEXT REFERENCES iam_applications(id),
    data JSON NOT NULL
);

CREATE TABLE iam_ssh_keys (
    id TEXT PRIMARY KEY,
    data JSON NOT NULL
);
```

SQLite enforces FKs natively (`PRAGMA foreign_keys = ON`). The JSON `data` column holds the full API response shape — flexible, no migrations when Scaleway adds optional fields.

### Admin State API

The admin state schema is a **versioned contract** between Mockway and InfraFactory.

**Endpoints**:
- `POST /mock/reset` — wipe all state
- `GET /mock/state` — full resource graph as JSON
- `GET /mock/state/{service}` — single service state (valid: `instance`, `vpc`, `lb`, `k8s`, `rdb`, `iam`; unknown → 404)

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
    ]
  },
  "vpc": {
    "vpcs": [
      {"id": "uuid", "region": "fr-par", "name": "main"}
    ],
    "private_networks": [
      {"id": "pn-uuid", "vpc_id": "uuid", "region": "fr-par", "name": "app-network"}
    ]
  },
  "lb": {
    "lbs": [
      {"id": "uuid", "zone": "fr-par-1", "name": "web-lb",
       "ip": [{"id": "uuid", "ip_address": "51.15.x.x", "lb_id": "uuid"}]}
    ],
    "frontends": [
      {"id": "uuid", "lb_id": "uuid", "name": "http", "inbound_port": 80,
       "backend_id": "be-uuid"}
    ],
    "backends": [
      {"id": "be-uuid", "lb_id": "uuid", "name": "web-servers",
       "forward_port": 80, "server_ips": ["10.0.0.1", "10.0.0.2"]}
    ],
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
    ]
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
    ]
  }
}
```

This schema mirrors the Scaleway API response shapes. The TopologyEvaluator in InfraFactory consumes this to build the connectivity graph.

**Mockway v2** (future): S3 on second port (Azurite-inspired), deeper resource simulation (K8s cluster lifecycle, LB health checks).

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
| `go install` | `go install github.com/redscaresu/scaleway_infra_factory/cmd/infrafactory@latest` | Standard Go |

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
