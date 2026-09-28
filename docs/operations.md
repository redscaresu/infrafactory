# Operations

Task-specific detail that `AGENTS.md` points to. Read a section when its task comes up; nothing
here needs reading at the start of a session.

## Layer 3: real Scaleway

Layer 3 spends real money against `api.scaleway.com`. Read ADR-0023 before changing anything on
this path; it defines what a passing Layer 3 result means.

**The run owns its project** (ADR-0025). infrafactory creates the run's project through the
Account API before the apply and hands it to the provider as the default, so every resource
lands in it. Generated HCL must not declare `scaleway_account_project` or set `project_id`; the
Layer 3 HCL gate refuses both.

**Credentials.** Authenticate as the `infrafactory-layer3` IAM application, whose key lives in
`~/.config/infrafactory/layer3.env` (mode `0600`; rotated 2026-09-07):

```bash
set -a; . ~/.config/infrafactory/layer3.env; set +a
```

Do not fall back to the default `scw` profile: on a developer machine that is a personal key,
here the organization owner's. The sandbox strips `SCW_PROFILE`/`SCW_CONFIG_PATH`, which also
forces the default profile, so "just use the default profile" silently means "run as whoever
that is". The application's policy set is recorded in ADR-0023's credential amendments.

**`openclaw-prod` is protected by software, not by the API**, since `InstancesFullAccess` was
granted. The guards are the deny-by-default allowlist, the run-project guard
(`AssertProjectDeletable`), the orphan sweep, and project-per-run. Treat each as load-bearing;
widening any is a blast-radius decision (ADR-0023).

**Configuration:**
1. `SCW_DEFAULT_ORGANIZATION_ID` — required; preflight fails closed without it (`layer3.env` sets it).
2. `validation.layers.sandbox_deploy.enabled: true`.
3. `validation.layers.sandbox_deploy.allow_resource_types` — deny-by-default; empty or absent
   denies everything. Checked after generation and before apply, so a denied type costs nothing.
   The list lives in three places (`internal/config/config.go`, `infrafactory.yaml`,
   `docs/layer3-coverage.md`) and tests keep them in step.
4. `scaleway.fallback_project_id` — a dedicated disposable project for anything that resolves the
   provider default. Refused if set to the organization id.

**Operational notes:**
- The sandbox environment is sealed, not merely overridden (`harness.SandboxStripEnv`): an
  inherited `SCW_API_URL` cannot retarget a real apply at mockway.
- `infrafactory reap <scenario>` destroys what an interrupted run left behind, gated by
  `AssertProjectDeletable` and verified by a real-API sweep. A reap that cannot prove the account
  clean fails.
- Layer 3 is opt-in, off by default, and never wired into a scheduled CI job.
- Real-vs-mock deltas: `docs/layer3-real-vs-mock-deltas.md`. Real Scaleway can return a create
  error after the resource exists; apply retries once (`sandboxApplyAttempts`), never on a
  cancelled context.
- A `run` writes auto-learned pitfalls into `pitfalls/*.yaml` **in the checkout it runs from**. Run
  generated and real-cloud runs from a throwaway worktree, not the lead's working copy, and never
  `git add -A` after one: a learned entry once replaced a curated safety pitfall and nearly merged
  inside an unrelated PR.
- `infrafactory pitfalls check-avoid aws --resource R --attribute A --from DIR` is the only way a
  `source: avoid` rule leaves the corpus: it replays the shape cut from `DIR` on fakeaws, with no
  LLM, and retires the rule only on a clean apply and plan (ADR-0034). It writes
  `pitfalls/avoid-checks/`; commit exactly what it wrote, from a throwaway worktree.
- A failed run auto-destroys and then sweeps. If the sweep cannot confirm the account clean, the
  failure names `infrafactory reap <scenario>`; act on it.

## Sibling mocks

Three first-party HTTP-level mocks + one third-party backend live alongside infrafactory:

- **mockway** (`../mockway`, github.com/redscaresu/mockway) — Scaleway mock; runs on `:8080`. Apache-2.0, public.
- **fakegcp** (`../fakegcp`, github.com/redscaresu/fakegcp) — GCP mock; runs on `:8081`. Memorystore + Cloud SQL + GKE + IAM + Storage + DNS + Pub/Sub + Secret Manager + Cloud Run + Cloud KMS (added 2026-05-31 in fakegcp@c7999b5).
- **fakeaws** (`../fakeaws`, github.com/redscaresu/fakeaws) — AWS mock; runs on `:8082`. Ships 10 services across 5 wire formats (IAM, S3, EC2, RDS, DynamoDB, EKS, SQS, Route53, Secrets Manager, KMS).
- **fakegenesys** (`../fakegenesys`, github.com/redscaresu/fakegenesys) — Genesys Cloud CCaaS mock; runs on `:8083`. Ships 15 resources across identity (user, group, location, auth_role, oauth_client), routing (queue + members + skill + wrapupcode + language + utilization singleton), and architect (datatable + rows + user_prompt + flow with multipart + lock/publish state machine + responsemanagement_response + idp_generic singleton). Spec-driven fidelity via `specs/genesys-openapi.json`. Added 2026-06-06 (arc S108-S115); hardened in arc S123-S127 + sibling rollout S128-S130 (v0.2.0).
- **SeaweedFS** (`chrislusf/seaweedfs` container) — third-party S3 backend for `aws_s3_bucket` reads (`terraform-provider-aws` needs the full management surface; fakeaws's stripped S3 handler isn't enough). Runs on `:9090` via `docker-compose.mocks.yml`. Anonymous-mode `ListAllMyBuckets` returns empty even when buckets exist — use HEAD-by-name as the assertion path. Empirical evaluation log in `CONCEPT.md` § "Third-Party Mock Integration" (rejects Adobe S3Mock + Garage + LocalStack + MinIO).
- **s3router** (`cmd/s3router/`, S80) — in-repo reverse-proxy shim that listens on `:9091` and fans S3 traffic across SeaweedFS and fakeaws. `?publicAccessBlock` → fakeaws (SeaweedFS uniquely 501s on that subresource); everything else → SeaweedFS; `PUT/DELETE /<bucket>` fans out to both. `infrafactory.yaml` `s3.url` points at the shim, not SeaweedFS directly. Add a subresource to `fakeawsSubresources` in `main.go` only when a new SeaweedFS 501 surfaces. ADR-0015 § "S80 — S3 backend router" carries the rationale.

**AWS needs fakeaws STS up for Layer 1 and `validate`, not only for apply.** The generated provider
block has no `skip_*` (ADR-0039), so the provider calls STS `GetCallerIdentity` when it configures,
which `tofu plan` does. cloudEnv routes STS to fakeaws's `/sts`; with fakeaws down, Layer 1 fails at
plan naming the fakeaws address.

All four sibling repos are independent public OSS repos on origin/main; cross-repo work cascades (`AGENTS.md` § Operational caveats). The s3router is part of the infrafactory repo, not a sibling.

When extending a sibling mock, mirror the per-bundle PR rule in `../fakeaws/concepts.md` — handler + tests + examples + scenario anchors + coverage_matrix.yaml + `LandedServices` flip all in one slice. The `TestFullCoverageAudit` + `TestRegressionSeedAuditManifestMatchesHandlers` audits in each mock repo enforce this.

**Provider smoke-harness pattern (canonical)**: every sibling fake uses the same `examples/provider_smoke_test.go` pattern — `examples/{working,misconfigured,updates}/<svc>/` directories auto-discovered, real provider binary run against the fake (no real-cloud credentials needed; the provider IS the wire-format validator). Gating is per-repo env var (`MOCKWAY_ENABLE_E2E` / `FAKEGCP_ENABLE_E2E` / `INFRAFACTORY_ENABLE_E2E` / `FAKEGENESYS_ENABLE_E2E`). When spawning a new sibling, copy this harness as-is — it's the cheapest correctness gate we have. Detail in each sibling's `AGENTS.md` § "Provider smoke harness".

**Contract-coverage convention (canonical)**: every sibling fake ships a `handlers/contract_audit_test.go` that enforces the `CRITICAL[<id>]:` / `MUST[<id>]:` docstring → `TestContract_<id>` test pairing. A wire-shape invariant the consuming provider depends on must NOT live as a comment alone — drift becomes a failed `go test`, not a missed code review. New sibling fakes inherit the file as part of the day-one OSS checklist (see `feedback_oss_mature_day_one.md` item 14). Reference impl: `../fakegenesys/handlers/contract_audit_test.go`.

### Fidelity strategies

How each sibling fake decides what wire shape a handler SHOULD return (the smoke harness above is the correctness *gate*, not the discovery *source*). Quick reference for fresh agents:

| Fake | Strategy | Source of truth | Cost per new handler |
|---|---|---|---|
| **mockway** | Spec-driven | `specs/` tree (downloaded Scaleway OpenAPI YAML) | Low — spec gives shape upfront |
| **fakegcp** | Hybrid | GCP discovery docs (referenced ad-hoc, no `specs/` tree) | Medium — semi-discovered per handler |
| **fakeaws** | Reactive | `terraform-provider-aws` source + `TF_LOG=DEBUG` capture | High — ~1-2hr per service before handler is confident |
| **fakegenesys** | Spec-driven (mirrors mockway) | `specs/genesys-openapi.json` (filtered Genesys Swagger 2.0, ~200KB) | Low |

**Recommendation for new fakes**: prefer spec-driven if the target cloud publishes an OpenAPI / Swagger / discovery doc. For sibling extensions: opportunistic upgrade — existing handlers stable, new ones can adopt spec-driven if a spec is available.

Detail in each sibling's `AGENTS.md` § "Fidelity strategy".

## The planning chain

Work is planned top-down, and the user approves each level before the next is made:

1. **HLD** — `/hld <title>` opens a pane on the most capable model (Fable), where the user writes
   `docs/hld/YYYY-MM-DD-<slug>.md` with it; a reviewer attacks the draft before it is `agreed`.
2. **Epics** — `/plan-hld <hld>`: a swarm, led by Fable, splits the HLD into epics that each
   have a **Done when** that could fail, and skeptics and codex check for gaps and overlaps.
3. **Stories** — `/plan-epic <epic>`: a swarm, led by Opus, splits each epic into one-PR stories.
4. **Build** — `scripts/swarm.sh story <slug>`: `ready` stories built in parallel panes.

Every agent at every level runs in its own herdr pane. The session that runs the chain supervises,
decides model and effort by role, reviews, and merges; it does not do the agents' work itself.

## Model and effort

Every agent's model and effort come from its role, set in one place: `policy()` in
`scripts/swarm.sh` (`scripts/swarm.sh policy <role>` prints it). The principle is to spend on
judgment and save on reading:

- **Design is Fable.** The HLD is co-written at `high`, because the user waits on every turn, and
  attacked at `xhigh` before it is agreed. Splitting an HLD into epics is the one call that shapes
  all the work below it, so it runs on Fable at `xhigh`.
- **Judgment is Opus.** An epic's decomposition into stories decides everything after it, so it
  runs at `xhigh`. Skeptics and the critic run at `high`, because a skeptic is the only gate a
  story passes before it is built.
- **Reading and running is Sonnet.** Surveys read and cite at `high`, since they feed the lead;
  verification that runs tests and reports runs at `medium`, as do docs and chores.
- **Building code is Opus at `high`**, and at `xhigh` for a story marked `risk: high` (Layer 3,
  teardown, safety, hygiene).
- **Codex is the cross-model check**, read-only, because a different model family shares fewer
  blind spots with the one that wrote the plan.
- **Below the design level, Fable is escalation only**: a story that failed twice, or an epic whose
  contradictions no one can reconcile.

A story's `kind` (and `risk`) chooses its role; `kind: lead` (real cloud, credentials) and
`kind: operator` (a human step) are refused by the swarm.

## Scoping an epic

An epic (`docs/epics/<slug>.md`: goal, **Done when**, out of scope, constraints) becomes stories
with `/plan-epic <slug>` (`.claude/commands/plan-epic.md`). It runs as a herdr swarm, every agent
in its own pane: three surveys (where it lands, what constrains it, what overlaps it), one lead
decomposition into one-PR stories with `kind`, `touches` and `depends_on`, a skeptic per story
(capped at five, the rest logged) and a codex pass, then a critic. Agents write their answers to
`.swarm/<epic>/`. The user sees refuted stories and contradictions first and approves before any
story file is written. About ten agents per run, so scope deliberately.

## Parallel agents (herdr)

A wave builds several `ready` stories at once, one agent per herdr pane, each in its own git
worktree; up to four panes a tab. The lead dispatches, reviews and merges; agents never merge.

**Pick the wave.** Only `ready` stories whose `touches` do not overlap. Work that edits a shared
file (`AGENTS.md`, `infrafactory.yaml`, the schema, the hygiene scripts) is done by the lead, alone.

**Start each story** from the lead's pane:

```bash
scripts/swarm.sh story <slug>      # worktree on story/<slug>, a pane, the agent, its brief
scripts/swarm.sh wait <slug>       # in the background: returns when it settles
```

The brief is the story file plus the standing rules (never merge, no real cloud, codex loop, reply
with the PR URL when CI is green). `herdr agent read <slug> --source recent-unwrapped` shows what an
agent is doing.

**Merge one at a time.** After each merge, merge `main` into every other open wave branch before
trusting its CI; a branch that conflicts gets no CI at all, which looks like a hang.

**Keep with the lead:** real-cloud runs (a restart mid-apply leaves resources behind),
credentials, and anything that changes permissions.

**Clean up.** After each merge, `git worktree remove ../infrafactory-wt/<slug>` (an error if it is
already gone), then `git worktree prune`. `scripts/swarm.sh close build` closes the wave's tabs.

## Demo recording

`asciinema` (CLI demo via `./docs/demo/record.sh`) → `.cast` → `agg` → `.gif`; Playwright (UI demo
via `make demo-ui`) → `.webm` → `gifski` → `.gif`. Build-time only; not advertised in user docs.
