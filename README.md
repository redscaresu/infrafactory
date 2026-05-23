# InfraFactory

Scenario-driven infrastructure generation and validation for Scaleway with OpenTofu.

## Problem It Solves

Teams often face the same infrastructure pain points:
- Infrastructure intent is documented in prose, but implementation is in hand-written IaC.
- Validation is inconsistent and manual (or only static linting).
- Failed iterations are hard to diagnose and repeat.

InfraFactory addresses this by making infrastructure delivery scenario-driven and deterministic:
1. Define intent in scenario YAML.
2. Validate contracts up front (config + schema).
3. Generate and validate infrastructure through layered checks.
4. Persist artifacts and structured failures for repeatable iteration.

## How It Works

You write a scenario YAML describing what you want (compute, database, load balancer, etc.).
InfraFactory generates, validates, and optionally deploys the infrastructure through a multi-layer
pipeline. If anything fails, it feeds the structured errors back to the LLM and retries automatically.

### Step 1: Scenario Definition

Write a YAML file declaring your infrastructure intent:
- **Resources**: compute, networking, database, kubernetes, redis, registry, IAM
- **Constraints**: region, zone, encryption, no public database, etc.
- **Acceptance criteria**: what must be true for the infrastructure to be considered correct

### Step 2: Three-Phase LLM Generation

Each iteration runs three sequential LLM phases:

```
  Phase 1: Plan Architecture
    Scenario YAML + size mappings -> JSON architecture plan
    (maps "size: small" to concrete Scaleway types like DEV1-S)

  Phase 2: Generate HCL
    Architecture plan + acceptance criteria -> OpenTofu .tf files
    (includes 18 Scaleway-specific pitfall rules to avoid common mistakes)

  Phase 3: Self-Review
    Generated HCL -> 10-point checklist review -> corrected files or "NO ISSUES FOUND"
```

### Step 3: Four-Layer Validation

Every iteration runs the generated code through up to four validation layers.
Each layer catches different classes of problems. Layers gate each other — a
failure in an earlier layer prevents later layers from running.

```
  Layer 1: Static Validation (seconds)
  ├── tofu init         — provider plugin setup
  ├── tofu validate     — HCL syntax and type checking
  ├── tofu plan         — resource graph and dependency resolution
  ├── tofu show -json   — structured plan output for policy checks
  └── OPA plan policies — evaluated against plan JSON using "deny" rules
      ├── no_public_database    — databases must have private_network blocks
      ├── no_public_endpoints   — no public IPs on internal resources
      ├── vpc_required          — VPC must be present
      ├── region_restriction    — resources in allowed regions/zones only
      ├── encryption_at_rest    — storage encryption required
      └── naming                — resource names follow conventions

  Layer 2: Mock Deploy via Mockway (seconds)
  ├── POST /mock/reset          — clear mock state (or snapshot/restore for incremental)
  ├── tofu apply -auto-approve  — deploy against mock Scaleway API
  ├── GET /mock/state           — pull deployed resource state
  ├── DeriveTopology            — compute connectivity/http_probe maps from raw resources
  │   (walks LB frontends/backends/IPs, server NICs, RDB/Redis endpoints, private networks)
  ├── Topology checks           — verify derived connectivity and reachability
  │   ├── connectivity          — can compute reach database on port 5432? (shared private network)
  │   ├── http_probe            — is the load balancer reachable on port 80? (LB+frontend+backend+IP)
  │   └── dns_resolution        — does the domain resolve? (informational until Layer 3)
  ├── OPA state policies        — evaluated against mock state using "deny_state" rules
  │   └── e.g. no_public_database checks deployed state for public endpoints
  └── Referential integrity     — mock enforces FK constraints (delete returns 409 with dependents)

  Layer 3: Real Scaleway Deploy (optional, minutes)
  ├── tofu plan -state=terraform-live.tfstate   — capture plan-live.txt artifact
  ├── tofu apply -state=terraform-live.tfstate  — deploy to real Scaleway API
  ├── Real network probes (replace topology checks from Layer 2)
  │   ├── connectivity    — TCP connect to actual host:port with retry
  │   ├── http_probe      — HTTP GET to real endpoint, expect 2xx/3xx
  │   └── dns_resolution  — DNS A/AAAA lookup with propagation retry
  ├── Self-managed project lifecycle
  │   └── scaleway_account_project resource in HCL — created and destroyed automatically
  └── Auto-destroy on failure — prevents orphaned billable resources

  Layer 4: Destroy Verification (seconds)
  ├── tofu destroy -auto-approve  — tear down all resources
  ├── Orphan check                — verify mock state has zero remaining resources
  └── If Layer 3 enabled: tofu destroy -state=terraform-live.tfstate
```

### Step 4: Retry Loop

If any validation layer fails, InfraFactory:
1. Records the structured failure (layer, stage, check, detail)
2. Checks stop conditions (stuck detection, repair budget exhausted)
3. Feeds the failure JSON into the next iteration's LLM prompt
4. Re-runs all three generation phases with the failure context
5. Repeats until all criteria pass or the budget is exhausted (default: 5 retries)

### Step 5: Holdout Verification

After training convergence, criteria-only holdout scenarios run as a final gate.
These are adversarial checks the LLM never sees during training — they verify the
generated code works for edge cases the scenario author cares about.

### Step 6: Incremental Evolution (optional)

With `--no-destroy`, the infrastructure state persists between runs. You can evolve
a scenario over time (e.g., add Redis to an existing web app + database stack) and
InfraFactory regenerates all HCL while OpenTofu handles the incremental diff.

## New Here

If you are onboarding to this repo, use this order:
1. Run `go run ./cmd/infrafactory --help` once to see the command contract.
2. Read `internal/cli/root.go` to see command entrypoints.
3. Read `internal/cli/runtime.go` to understand shared runtime setup and dependency injection.
4. Read one command end-to-end (`internal/cli/generate_command.go`), then compare with `validate`, `test`, and `run`.
5. Read package contracts in this order: `internal/config`, `internal/scenario`, `internal/generator`, `internal/harness`, `internal/feedback`, `internal/runstore`.

First 10-minute code walk:
1. `go test ./internal/cli -run TestGenerateCommandWritesFilesDeterministically`
2. `go test ./internal/scenario -run TestLoadWithSchemaPaths`
3. `go test ./internal/harness -run TestStaticHarness`

This gives one quick pass across command orchestration, input contracts, and harness execution.

## Mental Model

Think in three layers:
1. Contracts: config/scenario parsing and validation up front.
2. Execution primitives: generator/harness packages return deterministic typed results.
3. Orchestration: CLI commands compose primitives and map errors/output to a stable CLI contract.

Single-command lifecycle (`generate`, simplified):
1. `internal/cli` builds runtime (config + dependencies + loaders).
2. Scenario is loaded and validated (`internal/scenario` + `scenario.schema.json`).
3. Generator returns files as data (`internal/generator`), not filesystem side effects.
4. CLI writes files deterministically and renders command output.

## Architecture

```
                          +------------------+
                          |   User / CI      |
                          +--------+---------+
                                   |
                    +--------------+--------------+
                    |                             |
              +-----v-------+            +--------v--------+
              |  CLI        |            |  Web UI         |
              |  infrafactory            |  :4173          |
              |  run/test/  |            |  SvelteKit      |
              |  generate/  |            |  (embedded)     |
              |  ui/mock    |            |  /scenarios     |
              +-----+-------+            |  /runs /live    |
                    |                    |  /compare       |
                    |                    |  /pitfalls      |
                    |                    +--------+--------+
                    |                             |
                    +---->  REST API  <-----------+
                       /api/runs   /api/scenarios
                       /api/runs/{s}/compare
                       /api/scenarios/validate
                       /api/pitfalls (GET/PUT)
                       /api/output  /api/ws  /api/diagnostics
                                   |
         +-------------------------+-------------------------+
         |                         |                         |
   +-----v-------+         +-------v------+         +--------v------+
   |  Config     |         |  Scenario    |         |  RunStore     |
   |  loader     |         |  loader +    |         |  .infrafactory/
   |  infrafactory         |  schema      |         |  runs/        |
   |  .yaml      |         |  validation  |         |  artifacts    |
   +-----+-------+         +-------+------+         +---------------+
         |                         |
         +------------+------------+
                      |
               +------v-------+
               |  Generator   |     3-phase LLM pipeline (cloud-aware):
               |  (Claude /   |     1. Plan Architecture
               |  OpenRouter) |     2. Generate HCL
               +------+-------+     3. Self-Review
                      |             prompts/{scaleway,gcp}/phase{1,2,3}*
                      |             pitfalls/{scaleway,gcp}.yaml
                      v
         +---------------------------+
         |    Validation Layers      |
         |                           |
         |  Layer 1: Static          |  tofu init/validate/plan
         |    + OPA policy checks    |  + show -json
         |    (deny rules)           |  + naming, encryption_at_rest,
         |                           |    no_public_*, vpc_required,
         |                           |    region_restriction
         |                           |
         |  Layer 2: Mock Deploy     |  tofu apply -> mockway / fakegcp
         |    + topology derivation  |  + cloud auto-dispatch
         |    + state policy checks  |  + acceptance criteria evaluator
         |    (deny_state rules)     |
         |                           |
         |  Layer 3: Real Deploy     |  tofu plan/apply -> Scaleway
         |    (optional, opt-in)     |  + real network probes
         |                           |  + auto-destroy on failure
         |  Layer 4: Destroy         |  tofu destroy
         |    + orphan verification  |  + holdout checks
         +------------+--------------+
                      |
         +-------------+-------------+-------------------+
         |                           |                   |
   +-----v------+             +------v-------+    +------v------+
   |  mockway   |             |  fakegcp     |    |  Scaleway   |
   |  (Scaleway |             |  (GCP mock)  |    |  (real API) |
   |   mock)    |             |  port :????  |    |  Layer 3    |
   |  :8080     |             |  SQLite      |    |  only       |
   |  SQLite    |             |  ../fakegcp/ |    |             |
   |  ../mockway|             |              |    |             |
   +------------+             +--------------+    +-------------+

    Feedback Loop (on failure):
    structured failures -> FeedbackJSON -> next iteration's LLM prompt

    Auto-Pitfall Learning:
    target_reached  -> ExtractLearnedPitfall -> AppendPitfall
    repair_budget_  -> DetectOscillation     -> AppendPitfall (cross-
    exhausted/stuck                              cloud filtered by
                                                 sc.Cloud)
```

### Multi-Cloud Mock Backends

InfraFactory targets two cloud providers today, each with its own SQLite-backed mock binary started from source by the test harness (see `internal/e2e/helpers.go`):

```
   cloud=scaleway   --->   mockway   (../mockway, :8080)
   cloud=gcp        --->   fakegcp   (../fakegcp, :8081)
```

Both mocks expose the same admin endpoints — `/mock/state`, `/mock/reset`, `/mock/snapshot`, `/mock/restore` — so `internal/cli/mockStateClient` works against either backend. Topology derivation auto-detects which cloud emitted a `/mock/state` payload (top-level `compute` key → GCP; `instance` → Scaleway) and dispatches to per-cloud rules.

#### Running both mocks at the same time

Check out `mockway` and `fakegcp` as siblings of `infrafactory`:

```
~/dev/
├── infrafactory/
├── mockway/
└── fakegcp/
```

Then from the `infrafactory` repo root:

```bash
make mocks-up        # starts mockway on :8080 AND fakegcp on :8081
make mocks-status    # prints running pids
make mocks-logs      # tails 20 lines of each mock's log
make mocks-down      # stops both
```

Individual control:

```bash
make mockway-up      # mockway only
make fakegcp-up      # fakegcp only
make mockway-down
make fakegcp-down
```

Pidfiles and logs land in `/tmp/infrafactory-mocks/` (override with `MOCKS_RUN_DIR=...`). Ports default to `MOCKWAY_PORT=8080` / `FAKEGCP_PORT=8081` and are reflected in `infrafactory.yaml` as `mockway.url` and `fakegcp.url`.

The runtime's `cloudMockStateRouter` (see `internal/cli/mockway_client.go`) reads each scenario's `cloud:` field after `LoadScenario` and dispatches every Layer-2 admin call (`State` / `Reset` / `Snapshot` / `Restore`) to the matching backend automatically — a single `infrafactory run` invocation can iterate over Scaleway scenarios and GCP scenarios back-to-back without restarting either mock. If `fakegcp.url` is empty, GCP scenarios fall back to mockway (which will 4xx but keeps the runtime constructible).

Sample `infrafactory.yaml`:

```yaml
mockway:
  url: http://127.0.0.1:8080
  auto_reset: true

fakegcp:
  url: http://127.0.0.1:8081
  auto_reset: true
```

End-to-end check after `make mocks-up`:

```bash
infrafactory run scenarios/training/web-app-paris.yaml      # cloud: scaleway -> mockway
infrafactory run scenarios/training/gcp-vm-network.yaml     # cloud: gcp      -> fakegcp
```

### Mockway API Coverage (Scaleway)

Mockway is a companion SQLite-backed mock server that simulates Scaleway APIs for deterministic, offline testing. It covers:

| Service | Scaleway API | CRUD | Notes |
|---|---|---|---|
| Compute | Instance (servers, IPs, NICs, security groups, volumes) | Full | Server type catalog includes DEV1-S/M/L, GP1-XS/S/M/L/XL. Marketplace local images for ubuntu/debian/centos. |
| Networking | VPC, Private Network, Public Gateway | Full | Zone/region scoping. |
| Load Balancer | LB, Frontend, Backend, LB IP, LB Private Network | Full | Multi-backend support. Delete returns 409 when dependents exist (except LB private networks which cascade). |
| Database | RDB Instance, User, Database, ACL, Privilege, Certificate, Endpoint | Full | PostgreSQL + MySQL engines. Private network endpoints with IPAM. Delete returns 409 when dependents exist. |
| Kubernetes | K8s Cluster, Node Pool | Full | Cluster delete returns 409 when pools exist. DELETE returns resource with `"status": "deleting"` (required by SDK). |
| IAM | Application, API Key, Policy, Rule | Full | Application defaults applied. |
| Container Registry | Registry Namespace | Full | Public/private namespaces. |
| Redis | Redis Cluster | Full | Password, ACL rules, endpoints, settings. Required default fields populated. |
| Block Storage | Volume, Snapshot | Basic | Minimal CRUD. |

Key behaviors:
- Referential integrity: DELETE returns 409 Conflict when dependents exist (K8s pools, LB frontends/backends, RDB users/databases).
- All handlers use UUID IDs and RFC3339 timestamps.
- State inspection: `GET /mock/state` returns all resources; `POST /mock/reset` clears state.

### OPA Plan Policy Checks (Static Layer)

OPA policies are evaluated at two points using a rule naming convention:
- **`deny` rules**: evaluated against `tofu show -json tfplan` output during static validation (layer 1). Fires before any deploy action.
- **`deny_state` rules**: evaluated against the mockway state snapshot (`GET /mock/state`) during mock deploy validation (layer 2). Fires after apply.

A single `.rego` file can contain both rule types. For example, `no_public_database.rego` has a `deny` rule that checks the plan for missing `private_network` blocks and a `deny_state` rule that checks deployed state for public endpoints.

Bundled policy files:

Scaleway:
- `policies/scaleway/no_public_database.rego` (plan + state)
- `policies/scaleway/no_public_endpoints.rego` (plan)
- `policies/scaleway/vpc_required.rego` (plan)
- `policies/scaleway/region_restriction.rego` (plan)
- `policies/scaleway/encryption_at_rest.rego` (plan)

GCP (Slice 36):
- `policies/gcp/no_public_sql.rego` (plan + state)
- `policies/gcp/vpc_required.rego` (plan + state)
- `policies/gcp/region_restriction.rego` (plan + state)
- `policies/gcp/encryption.rego` (plan + state)

Common:
- `policies/common/naming.rego` (plan)

Per-cloud routing for criteria-driven Layer 2 evaluation lives in `internal/cli/test_command.go::cloudConstraintPolicies` — a `cloud: gcp` scenario with `check: encryption_at_rest` resolves to `policies/gcp/encryption.rego`, not the Scaleway file. The Scaleway-shaped flat `constraint_policies:` config map remains the fallback for unmapped checks.

Custom policies can be added to `policies/custom/` — any `.rego` file under `paths.policies` is automatically picked up.

This is separate from holdouts:
- OPA checks are part of the training validation stages within each run iteration.
- Holdouts are criteria-only scenarios executed after training convergence as a final gating phase.

## CLI Commands

| Command | Purpose |
|---------|---------|
| `infrafactory init --path <file>` | Scaffold a new scenario YAML |
| `infrafactory generate <scenario>` | Run 3-phase LLM generation only |
| `infrafactory validate <scenario>` | Run Layer 1 static checks only |
| `infrafactory test <scenario>` | Run Layers 1-4 (no retry loop) |
| `infrafactory run <scenario>` | Full pipeline with retry loop + holdouts |
| `infrafactory mock start/stop/status/logs` | Manage mockway container |
| `infrafactory ui` | Serve the web dashboard |

Key flags for `run`:
- `--clean` — force fresh start (wipe state)
- `--no-destroy` — keep resources for incremental follow-up
- `--repair-iterations-max N` — override retry limit (default: 5)

## Acceptance Criteria

| Type | Layer 2 (mock) | Layer 3 (real) |
|------|---------------|----------------|
| `connectivity` | Topology graph query | TCP connect with retry |
| `http_probe` | Topology graph query | HTTP GET, expect 2xx/3xx |
| `dns_resolution` | Auto-pass (informational) | DNS A/AAAA lookup with retry |
| `policy` | OPA rules on plan + state | Same |
| `destruction` | Orphan check after destroy | Same + real destroy |

## Repository Layout

- `cmd/infrafactory/`: CLI entrypoint
- `internal/cli`: command tree and command-level wiring
- `internal/config`: runtime config model and loader (`infrafactory.yaml`)
- `internal/scenario`: scenario parsing and schema validation
- `internal/generator`: generator contracts, prompt rendering, output parser
- `internal/harness`: static/deploy/destroy orchestration primitives
- `internal/feedback`: failure models and stuck-detection helpers
- `internal/runstore`: `.infrafactory/runs` persistence implementation
- `scenario.schema.json`: scenario contract
- `infrafactory.yaml`: runtime config contract
- `policies/`: OPA policy files
- `prompts/`: LLM phase prompt templates (plan_architecture, generate_hcl, self_review)
- `mappings.yaml`: T-shirt size → Scaleway offering mappings (compute, database, kubernetes, redis)
- `scripts/`: quality gate helpers (`check_all.sh`, `check_doc_hygiene.sh`, `check_benchmarks.sh`, `full_flow.sh`)
- `scenarios/`: training/holdout/regression fixtures

Package ownership guide:
- `internal/cli`: args/flags, runtime wiring, command orchestration, output contract.
- `internal/config`: `infrafactory.yaml` defaults + typed validation errors.
- `internal/scenario`: scenario decode + schema validation + typed model.
- `internal/generator`: generator interfaces/errors, prompt rendering, output parsing.
- `internal/harness`: static/mock/destroy workflows and stage-level failures.
- `internal/feedback`: failure-signature modeling and stuck-detection utilities.
- `internal/runstore`: persisted run metadata and iteration artifacts.

## Requirements

- Go `1.24.6+`
- OpenTofu (`tofu`) available in `PATH`
- Docker + Docker Compose plugin (`docker compose`)
- `make`
- `curl` (used by smoke/dependency readiness helpers)
- Optional for deploy-layer integration: Mockway running locally

## Quick Start

### Prerequisites

- Go 1.24.6+
- OpenTofu (`tofu`) in PATH
- Docker + Docker Compose
- Claude CLI (`claude`) or `OPENROUTER_API_KEY`

### CLI Workflow

```bash
# 1. Build
make build

# 2. Start mockway (mock Scaleway API)
make deps-up

# 3. Create a scenario
./bin/infrafactory init --path scenarios/training/my-app.yaml
# Edit the YAML with your resources and criteria

# 4. Run the full pipeline (generate + validate + deploy + destroy + retry)
./bin/infrafactory run scenarios/training/my-app.yaml

# 5. Check artifacts
ls .infrafactory/runs/my-app/

# 6. Cleanup
make deps-down
```

### Web UI Workflow

```bash
make deps-up
make run
# Builds everything and starts the UI at http://127.0.0.1:4173
```

The UI provides:
- **Scenario browser** — browse, edit, and save scenario YAML
- **Run controls** — start runs with `--clean` / `--no-destroy` / Layer 3 toggles
- **Live page** — real-time iteration progress with elapsed timer, stage indicators, retry reasons
- **Run history** — per-run IaC viewer with iteration snapshots, diffs, and download
- **Diagnostics** — backend readiness checks

### Web UI Development

```bash
# Terminal 1: Go API server (no embedded assets)
go run -tags noui ./cmd/infrafactory ui --addr 127.0.0.1:4173

# Terminal 2: Svelte dev server with hot reload
make ui-install && make ui-dev
# Open http://127.0.0.1:5173
```

### Run Artifacts

Each run persists to `.infrafactory/runs/<scenario>/<run-id>/`:

| File | Contents |
|------|----------|
| `run.json` | Run metadata, status, terminal reason |
| `app.log` | Structured JSON application logs |
| `plan.txt` | Layer 1 tofu plan output |
| `baseline_state.json` | Mockway state snapshot before run (incremental) |
| `generated/` | Immutable IaC snapshot for this run |
| `iterations/<n>/iteration.json` | Per-iteration stages and failures |
| `iterations/<n>/generated/` | Per-iteration IaC snapshot |
| `iterations/<n>/plan-live.txt` | Layer 3 tofu plan output (when enabled) |

### Testing

```bash
make test          # Go unit + UI unit + Playwright e2e
make test-unit     # Go tests only
make ui-test       # Frontend unit tests only
make ui-test-e2e   # Playwright e2e tests only
make smoke         # Opt-in real-tool smoke tests
```

### Logging

Structured app logs are emitted as JSON lines with deterministic fields:
- `level`, `command`, `event`
- optional: `status`, `run_id`, `iteration`, `stage`, `check`, `detail`

Current sinks:
- `stderr` for all commands.
- run-scoped artifact file for `run`: `.infrafactory/runs/<scenario>/<run-id>/app.log`.

Secret-like detail tokens are redacted in log details (`token`, `api_key`, `secret`, `password`, `prompt`).

### Run Feedback Payload (MVP)

`run` passes structured failure feedback into next-iteration generation (`FeedbackJSON`) with:
- `layer`, `stage`, `check`, `command`, `detail`
- optional: `policy`, `resource`
- `failure_class`: `iac_validation`, `transport_runtime`, `orchestration_control`, `snapshot_failed`, `restore_failed`, `layer3_apply_failed`, `layer3_destroy_failed`, `probe_failed`, or `layer3_preflight_failed`

Terminal control markers are intentionally excluded from iterative repair feedback entries.

### Provider Schema Prompt Injection

`generate` and `run` lazily extract the provider schema once per command runtime and inject it into phases 2 and 3. This works for any cloud provider — Scaleway (`scaleway/scaleway`) and GCP (`hashicorp/google`, planned Slice 36):
- **Extraction**: `tofu init` + `tofu providers schema -json` in an isolated temp directory; cached for the runtime lifetime.
- **Timing**: on first generate call (not during generic runtime bootstrap), so `validate`/`test`/`mock` commands avoid the overhead.
- **Filtering**: phase 1 output identifies which resource types are needed; `schema_filter.go` extracts only those types (plus companion sub-resources) from the full provider schema. This keeps prompt size bounded.
- **Injection**: phases 2 and 3 both receive the filtered schema as an "Authoritative Reference" section. The prompt instructs the LLM to verify every attribute name and block type against the schema before using it.
- **Failure mode**: extraction failures are non-fatal; generation proceeds without schema injection. Look for `provider_schema skipped` in logs.

### Multi-Cloud Architecture (Planned)

InfraFactory is designed to be cloud-agnostic. Four extension points are per-cloud:

| Extension point | Scaleway (current) | GCP (Slice 36) |
|----------------|-------------------|----------------|
| Prompt templates | `prompts/scaleway/` | `prompts/gcp/` |
| Pitfalls | `pitfalls/scaleway.yaml` | `pitfalls/gcp.yaml` |
| Topology derivation | Scaleway resources in `topology_derive.go` | GCP resources (dispatch by cloud) |
| Mock server | mockway (`:8080`) | fakegcp (`:8080`) |
| OPA policies | `policies/scaleway/` | `policies/gcp/` |
| Provider schema | `scaleway/scaleway` | `hashicorp/google` |

Adding a new cloud provider requires: prompt templates, pitfalls file, topology derivation rules, mock server, OPA policies, and training scenarios. The scenario's `cloud` field drives all dispatch.

### Provider Pitfalls

Provider-specific pitfalls are loaded from `pitfalls/{cloud}.yaml` at runtime based on the scenario's `cloud` field. Currently 16 Scaleway pitfalls covering K8s versioning, instance types, LB wiring, RDB configuration, Redis passwords, and DNS records.

Pitfalls are injected into phases 2 and 3 via `{{.Pitfalls}}` — no code changes needed to add new ones. Each pitfall has a `source` field: `static` (manually written) or `learned` (discovered from run feedback).

To add a new pitfall, edit `pitfalls/scaleway.yaml` (or create `pitfalls/gcp.yaml` for GCP, etc.).

Phase 1 also enforces exact size mapping usage to prevent the LLM from inventing Scaleway types that don't exist in the mock or real API.

### Size Mappings and Overrides

`mappings.yaml` maps T-shirt sizes to concrete Scaleway offerings:

| Resource | Sizes | Example Mapping |
|---|---|---|
| Compute | small/medium/large/xlarge | small → `DEV1-S`, large → `GP1-S`, xlarge → `GP1-M` |
| Database | small/medium/large/xlarge | small → `DB-DEV-S`, large → `DB-GP-XS` |
| Kubernetes | small/medium/large/xlarge | small → `DEV1-M` (1 node), medium → `GP1-XS` (3 nodes) |
| Redis | small/medium/large/xlarge | small → `RED1-MICRO`, xlarge → `RED1-L` |

Scenario-level overrides (e.g. `override: { node_type: DB-GP-XS, engine_version: "15" }`) take priority over size mappings and are passed to phase 1 as prescriptive instructions.

## Usage

### Exit Codes and Error Contract

CLI exit codes:
- `0`: success (`cli.ExitCodeSuccess`)
- `1`: runtime failure (`cli.ExitCodeRuntime`)
- `2`: usage/argument/flag contract failure (`cli.ExitCodeUsage`)

Error contract:
- Usage errors are surfaced as `*cli.CLIError` with code `usage` and map to exit code `2`.
- Runtime failures map to exit code `1`, with normalized error codes:
  - `config_invalid`
  - `scenario_malformed`
  - `scenario_invalid`
  - `dependency_unavailable`
  - `command_failed`
- Output mode contract is strict: `--output` must be `human` or `json`.
- Machine output schema version is `infrafactory.output.v1`.

### `infrafactory.yaml` Quick Reference

| Key | Required | Default | Purpose |
|---|---|---|---|
| `version` | yes | none | Config schema version (`"1.0"`). |
| `agent.type` | yes | none | Generator backend type (`claude-code` or `openrouter`). |
| `agent.repair_iterations_max` | no | `5` | Maximum failure-triggered retries in `run`. |
| `agent.phases` | no | `[plan_architecture, generate_hcl, self_review]` | Ordered generation phases (canonical sequence). |
| `agent.phase_delay_seconds` | no | `0` | Delay between generator phases (rate-limit mitigation). |
| `agent.claude.command` | no | `claude` | Executable used for `claude-code` transport. |
| `agent.claude.phase_timeout_seconds` | no | `300` | Hard timeout per Claude phase call; prevents indefinite hangs. |
| `agent.openrouter.model` | conditional | none | Required when `agent.type=openrouter`. |
| `agent.openrouter.base_url` | no | `https://openrouter.ai/api/v1` | OpenRouter API base URL. |
| `agent.openrouter.timeout_seconds` | no | `60` | OpenRouter request timeout per phase. |
| `agent.openrouter.max_retries` | no | `2` | OpenRouter retry count for transient failures. |
| `mockway.url` | yes | none | Mockway base URL used by deploy/destroy layers. |
| `mockway.auto_reset` | no | `true` | Whether mock reset is expected before deploy checks. |
| `validation.layers.*.enabled` | no | varies | Enables/disables layer execution paths. `sandbox_deploy` defaults to `false`; set to `true` for Layer 3 real Scaleway deploy (ADR-0010). |
| `validation.real_probes.timeout_seconds` | no | `5` | Per-attempt timeout for Layer 3 TCP/HTTP/DNS probes. |
| `validation.real_probes.retries` | no | `6` | Retry budget for Layer 3 probes to tolerate startup and propagation delays. |
| `validation.real_probes.retry_delay_seconds` | no | `5` | Delay between Layer 3 probe attempts. |
| `paths.output` | no | `./output` | Generated IaC output root. |
| `paths.policies` | no | `./policies` | Policy root used by harness validation. |

Canonical config example: `infrafactory.yaml` in repo root.

### Scenario Authoring Quick Reference

Required top-level keys:
- `scenario`, `version`, `cloud`, `description`, `acceptance_criteria`

Available resource types (under `resources:`):
- `compute`: `purpose`, `size` (small/medium/large/xlarge), `count`, optional `override` (offer, image)
- `networking`: `vpc`, `private_network`, optional `load_balancer` (exposure: public/private, backends: port + protocol: http/https/tcp)
- `database`: `engine` (postgresql/mysql), `size`, `high_availability`, optional `override` (node_type, engine_version)
- `kubernetes`: `size`, optional `override` (node_type, node_count)
- `redis`: `purpose`, `size`, optional `override` (node_type)
- `registry`: `purpose`, `is_public`
- `iam`: `purpose`, `application`, `api_key`, `policy` (all default to true when omitted)

Common criteria patterns:
- `policy`: `type: policy`, `check: <constraint_name>`, `expect: pass|fail`
- `connectivity`: `from`, `to`, optional `port`, `expect: success|blocked`
- `http_probe`: `target`, `port`, `expect: reachable|unreachable`
- `destruction`: `expect: no_orphans`

Holdout-only routing fields:
- `type: holdout`
- `references: <training-scenario-path>`

Layer 3 note:
- `dns_resolution` requires Layer 3 to be enabled. Without Layer 3, it auto-passes with an informational support-matrix stage.

### Training Scenarios

12 training scenarios covering the full parameter space:

| Scenario | Resources | Key Coverage |
|---|---|---|
| `web-app-paris` | compute, networking (LB), database | PostgreSQL, small, public LB, policy checks |
| `k8s-cluster-paris` | kubernetes, networking | K8s small, VPC + private network |
| `iam-policies-paris` | iam | IAM application + API key + policy |
| `registry-paris` | registry | Private container registry |
| `redis-paris` | redis, networking | Redis small cache |
| `full-stack-paris` | all 7 resource types | Composite multi-service |
| `mysql-ha-paris` | compute, networking, database | MySQL engine, medium DB, HA=true |
| `compute-lb-multi-paris` | compute, networking (multi-backend LB) | Large compute (count=3), HTTP + TCP backends |
| `k8s-medium-override-paris` | kubernetes, networking | Medium K8s, node_type/node_count overrides |
| `private-lb-db-paris` | compute, networking (private LB), database | Private LB, large PostgreSQL with overrides |
| `public-registry-iam-paris` | registry, iam | Public registry, IAM with policy=false |
| `redis-xlarge-session-paris` | compute, redis, networking | XLarge Redis with node_type override, xlarge compute |

### Basic setup and verification

```bash
go mod tidy
make test-all
```

### Developer Experience commands (`Makefile`)

Dependency lifecycle:

```bash
make deps-up
make deps-ps
make deps-logs
make deps-down
make deps-recreate
make deps-clean
```

Testing:

```bash
make test-unit
make test-all
make bench-check
```

CI behavior:
- Pull requests run `go test ./...`.
- Pushes to `main` run `go test ./...` and build/upload Linux binaries for `amd64` and `arm64`.

Real-tool smoke (opt-in):

```bash
make smoke-validate
MOCKWAY_URL=http://127.0.0.1:8080 make smoke-mockway
make smoke
make smoke-mockway-local MOCKWAY_BIN=/path/to/mockway
make smoke-mockway-manual
```

Transport adapter smoke tests (opt-in):

```bash
INFRAFACTORY_ENABLE_CLAUDE_TRANSPORT_SMOKE=1 \
go test ./internal/generator -run TestClaudeSeedGeneratorRealCommandSmoke

INFRAFACTORY_ENABLE_OPENROUTER_TRANSPORT_SMOKE=1 \
OPENROUTER_API_KEY=... \
OPENROUTER_MODEL=anthropic/claude-3.5-sonnet \
go test ./internal/generator -run TestOpenRouterSeedGeneratorRealHTTPOptInSmoke
```

Notes:
- `smoke-validate` runs `TestValidateCommandRealToolSmoke` with `INFRAFACTORY_ENABLE_REALTOOL_SMOKE=1`.
- `smoke-mockway` starts dependencies (`make deps-up`), waits for Mockway readiness, then runs `TestTestCommandRealToolMockwaySmoke` with `INFRAFACTORY_ENABLE_REALTOOL_MOCKWAY=1`.
- `smoke-mockway-local` runs the same smoke test against a locally installed `mockway` binary and auto-stops it after the test.
- `smoke-mockway-manual` runs the explicit fallback sequence (`docker run` + healthcheck + smoke test).
- Default test paths remain hermetic; smoke tests require external tools/services.
- Benchmark regression checks are env-gated and optional by default (`INFRAFACTORY_ENABLE_BENCHMARKS=1`).

Smoke test path options:
- Compose-managed dependency path: `make smoke-mockway`
- Local binary path (no Docker image required): `make smoke-mockway-local MOCKWAY_BIN=/path/to/mockway`
- Manual Docker fallback path: `make smoke-mockway-manual`

Troubleshooting:
- If Docker image pull fails with `denied`, use the local binary path (`smoke-mockway-local`) until image publishing is available.
- If you see `connection refused` to `localhost:8080`, use `127.0.0.1` explicitly and ensure Mockway is healthy:
  `curl -sSf http://127.0.0.1:8080/mock/state >/dev/null`.
- `smoke-mockway-local` may print one or more "waiting for mockway binary..." lines during startup; that is expected.
- If `generate` or `run` fails with `prompt render failed`, ensure `paths.prompts` points to a directory containing `phase1_plan_architecture.md`, `phase2_generate_hcl.md`, and `phase3_self_review.md`.
- If `generate` appears stuck on Claude transport, lower `agent.claude.phase_timeout_seconds` to fail faster and surface timeout errors while debugging.
- If `validate` or `test` fails with a generic `exit status 1`, rerun and inspect the surfaced `stderr:` tail in the failure detail; command stderr is now included directly in stage failure output.
- If you want iterative LLM correction from failures, use `run` (not just `generate` + `test`): `run` feeds prior iteration failures back into generation via `FeedbackJSON`.
- If you need to verify what failure feedback the model actually received, run with `INFRAFACTORY_CAPTURE_LLM_RAW=1` and inspect both `llm_prompt_<phase>.json` and `llm_raw_<phase>.json` for the same iteration.
- If generated `.tf` files contain markdown fences/tables, rerun `generate`; parser hardening strips fenced payloads and drops common markdown artifacts before file writes.
- `self_review` now applies partial corrections by merging returned `# File:` blocks into the existing generated file set; files omitted in self-review output are retained.
- `self_review` "no changes" detection now requires the exact canonical phrase `NO ISSUES FOUND` (case-insensitive, trimmed). Fuzzy substring matching (e.g. "looks good", "code is correct") has been removed to prevent false suppression of corrections. Unparseable self-review prose (no file blocks, not canonical phrase) falls through as a no-op, retaining phase-2 files.
- If `run` stops with `stuck`, compare failure detail strings across iteration artifacts (`iterations/<n>/iteration.json`): stuck signatures now include `check`, `resource`, and `detail`.
- If you see a `provider_schema skipped` log entry, schema extraction failed and generation continued without schema injection; verify `tofu` availability/network access if you expected schema-enriched prompts.
- If Claude output omits Scaleway provider wiring, `generate` now auto-injects `required_providers.scaleway` and `provider "scaleway"` into `providers.tf` before writing files.
- If `agent.type=openrouter` fails with `dependency_unavailable`, export `OPENROUTER_API_KEY` in the execution environment.
- If transport smoke tests fail, verify provider prerequisites:
  - claude transport smoke: `claude` command is installed and authenticated.
  - openrouter transport smoke: `OPENROUTER_API_KEY` and `OPENROUTER_MODEL` are set.

### Testing Matrix

| Goal | Command | External deps |
|---|---|---|
| Hermetic full test suite | `go test ./...` | none |
| Full local quality gate | `bash scripts/check_all.sh` | none |
| Unit-focused internal work | `make test-unit` | none |
| Repo-wide checks | `make test-all` | none |
| Benchmark guardrails (opt-in) | `make bench-check` | none |
| Real-tool static smoke | `make smoke-validate` | `tofu` |
| Real-tool mock deploy smoke | `make smoke-mockway` | `tofu`, Docker/Mockway |
| Real-tool mock smoke (local bin) | `make smoke-mockway-local MOCKWAY_BIN=/path/to/mockway` | `tofu`, local `mockway` |

Output contract regression guardrail:
- Golden snapshots for human/json output rendering are stored in:
  - `internal/cli/testdata/golden/output_contract/`
  - `internal/cli/testdata/golden/commands/`
- Refresh snapshots intentionally with `UPDATE_GOLDEN=1 go test ./internal/cli -run TestOutputContractGoldenSnapshots`.
- Refresh command-level snapshots with `UPDATE_GOLDEN=1 go test ./internal/cli -run TestCommandOutputGoldenSnapshots`.

Benchmark regression guardrail:
- Run `make bench-check` to execute benchmark thresholds in `scripts/check_benchmarks.sh`.
- Override thresholds with env vars:
  - `INFRAFACTORY_BENCH_MAX_NS_OUTPUT_JSON`
  - `INFRAFACTORY_BENCH_MAX_NS_OUTPUT_HUMAN`
  - `INFRAFACTORY_BENCH_MAX_NS_RUNSTORE_RW`

### Practical example 1: Inspect available CLI commands and flags

```bash
go run ./cmd/infrafactory --help
```

Command tree currently exposed:
- `init [--path <scenario-path>]`
- `generate <scenario-path>`
- `validate <scenario-path>`
- `test <scenario-path> [--no-destroy]`
- `run <scenario-path> [--repair-iterations-max N] [--clean] [--no-destroy]`
- `mock start`
- `mock stop`
- `mock status`
- `mock logs`
- `ui [--addr <host:port>]`

Global flags:
- `--config` (default `./infrafactory.yaml`)
- `--output` (`human` or `json`)

### Practical example 2: Initialize a scenario scaffold

```bash
go run ./cmd/infrafactory init --path scenarios/training/new-scenario.yaml
```

This writes a minimal schema-valid scaffold and prints deterministic next-step commands.

### Practical example 3: Run command adapters with explicit scenario path

```bash
go run ./cmd/infrafactory generate scenarios/training/web-app-paris.yaml --config infrafactory.yaml --output human
go run ./cmd/infrafactory validate scenarios/training/web-app-paris.yaml --config infrafactory.yaml --output json
go run ./cmd/infrafactory test scenarios/training/web-app-paris.yaml --config infrafactory.yaml --output human
go run ./cmd/infrafactory run scenarios/training/web-app-paris.yaml --config infrafactory.yaml --repair-iterations-max 3 --output json
```

Incremental run behavior:
- `--clean` forces a fresh run and ignores any prior state signals.
- `--no-destroy` skips post-convergence destroy and holdout execution so mockway state and `terraform.tfstate` remain available for the next run.
- Without either flag, `run` auto-detects incremental mode only when mockway already has resources, `output/<scenario>/terraform.tfstate` exists, and the run store contains a previous successful run for the same scenario.

Incremental operator workflow:
1. Start from a baseline scenario such as `scenarios/training/incremental-project-paris.yaml`.
2. Run the first pass with `--no-destroy` so the mock account state and `terraform.tfstate` remain available:
   `go run ./cmd/infrafactory run scenarios/training/incremental-project-paris.yaml --config infrafactory.yaml --repair-iterations-max 3 --no-destroy`
3. Edit the same scenario file to add the next resource slice, then rerun with `--no-destroy`.
4. InfraFactory will report `run/mode: pass (incremental ...)` once the prior successful run, state file, and mockway resources all exist.
5. Use `--clean` when you want to discard the preserved baseline and force a fresh apply from the current scenario definition.
6. A normal run without `--no-destroy` still performs the final destroy; the next run will fall back to clean mode because the preserved baseline is gone.

### Practical example 4: Start Mockway via CLI wrapper

```bash
go run ./cmd/infrafactory mock start --config infrafactory.yaml
go run ./cmd/infrafactory mock status --config infrafactory.yaml
go run ./cmd/infrafactory mock logs --config infrafactory.yaml
go run ./cmd/infrafactory mock stop --config infrafactory.yaml
```

### Practical example 5: Run package-focused checks while developing

```bash
go test ./internal/config
go test ./internal/scenario
go test ./internal/generator
go test ./internal/harness
go test ./internal/feedback
go test ./internal/runstore
```

### Practical example 6: Run optional layer-2 integration smoke test

```bash
INFRAFACTORY_ENABLE_INTEGRATION=1 \
INFRAFACTORY_MOCKWAY_URL=http://127.0.0.1:8080 \
go test ./internal/harness -run TestLayer2IntegrationSmoke
```

### Practical example 7: Run optional CLI real-tool smoke tests directly

```bash
INFRAFACTORY_ENABLE_REALTOOL_SMOKE=1 \
go test ./internal/cli -run TestValidateCommandRealToolSmoke

INFRAFACTORY_ENABLE_REALTOOL_MOCKWAY=1 \
INFRAFACTORY_MOCKWAY_URL=http://127.0.0.1:8080 \
go test ./internal/cli -run TestTestCommandRealToolMockwaySmoke
```

Manual fallback sequence (equivalent to `make smoke-mockway-manual`):

```bash
docker run --rm -d --name infrafactory-mockway -p 8080:8080 ghcr.io/redscaresu/mockway
curl -sSf http://127.0.0.1:8080/mock/state >/dev/null
INFRAFACTORY_ENABLE_REALTOOL_MOCKWAY=1 INFRAFACTORY_MOCKWAY_URL=http://127.0.0.1:8080 go test ./internal/cli -run TestTestCommandRealToolMockwaySmoke
```

### Practical example 8: Inspect persisted run artifacts

```text
.infrafactory/runs/<scenario>/<run-id>/
```

You will find `run.json` metadata, per-iteration artifacts (for example `iterations/1/iteration.json`), and `app.log` structured command/run logs in that directory tree.

### Practical example 9: One-command full flow helper

```bash
./scripts/full_flow.sh
```

Optional overrides:
```bash
CAPTURE_LLM_RAW=1 REPAIR_MAX=3 OUTPUT_MODE=human ./scripts/full_flow.sh
```

This helper starts mock, runs `run`, prints key artifact paths, and stops mock automatically.

## Local Quality Checks

```bash
bash scripts/check_all.sh
```

## Documentation Index

- Architecture: `docs/architecture.md`
- Full concept log: `CONCEPT.md`
- Decisions (ADRs): `docs/decisions/`
- Contributor guide: `CONTRIBUTING.md`
- Agent workflow: `AGENTS.md`
- Session bootstrap: `SESSION_START.md`
- Ticket backlog: `BACKLOG.md`
- Current execution stub: `CURRENT_TICKET.md`
- Rolling status: `STATUS.md`
- Execution prompt: `docs/process/EXECUTION_PROMPT.md`

### Architecture Decision Records (ADRs)

| ADR | Title | Status |
|---|---|---|
| 0001 | Foundations — base stack and execution model | Accepted |
| 0002 | CLI Command Contract — frozen args/flags/exit-code contract | Accepted |
| 0003 | Permanent Sandbox/Live Deploy Block — governance non-goal | Accepted |
| 0004 | Generator Transport Contract — claude/openrouter selection and phase semantics | Accepted |
| 0005 | Dual Iteration Controls | Superseded by ADR-0006 |
| 0006 | Run Failure-Only Retry Control — single `repair_iterations_max` knob, stop on first success | Accepted |
| 0007 | Scenario Schema Resource Expansion — kubernetes, iam, registry, redis resource definitions | Accepted |

### Doc Hygiene Automation

`bash scripts/check_doc_hygiene.sh --staged` runs automatically as part of `check_all.sh` and enforces contributor governance:
- Code or config changes (`cmd/`, `internal/`, `prompts/`, `policies/`, `scenarios/`, `go.mod`, `scenario.schema.json`) require a `STATUS.md` update.
- CLI contract or schema changes (`cmd/infrafactory/`, `internal/cli/`, `scenario.schema.json`, `infrafactory.yaml`) require an ADR update in `docs/decisions/`.
- New ADR files require an update to `docs/decisions/README.md` index.

This ensures documentation stays in sync with code changes without manual enforcement.

## Agent Kickoff

For autonomous ticket execution in a fresh agent session, use:

```text
Use docs/process/EXECUTION_PROMPT.md exactly. Start now.
```

## License

Apache License 2.0. See `LICENSE`.
