# Arc: Layer 3 — first real Scaleway apply

Status: planned (2026-08-22)
Owner: next-session claude (designed for autonomous execution)
Follows: `layer3-production-plan.md` (S30, 2026-02) — built the Layer 3 harness; never ran it against Scaleway.
Shape: goal-named variable-length arc per AGENTS.md "Planning a New Arc" (5 slices, ~9–13 hr + one operator-gated execution slice).

## Big picture

Layer 3 has been **code-complete since S30 and has never made a single call to `api.scaleway.com`.** Slices 26–30 built the whole apparatus — `tofu init → plan → apply` against `terraform-live.tfstate`, sandbox destroy, real TCP/HTTP/DNS probes, credential preflight, `plan-live.txt` artifact capture, auto-destroy on failed runs, UI toggle, `layer3-status` endpoint. All of it was validated **against mockway standing in for Scaleway**. Both real-tool tests (`internal/cli/realtool_smoke_test.go:285` and `:369`) are env-gated and skip by default; there is no evidence in `docs/status/ARCHIVE.md` that either has ever been run with real credentials.

The goal of this arc is narrow and concrete: **drive one scenario (`block-paris`) through apply → verify → destroy → provably-zero-orphans against real Scaleway**, behind guardrails that make a false-green or a billing leak structurally impossible.

It is deliberately *not* "turn Layer 3 on for the 16 Scaleway scenarios". Getting one cheap resource through the full lifecycle honestly is worth more than 16 scenarios that might all be silently hitting a mock.

### Verified during planning — these are NOT gaps

Cheap to re-derive, expensive to re-investigate. Recorded so a fresh session doesn't redo the work.

| Checked | Finding |
|---|---|
| `tofu plan/apply -state=<file>` on OpenTofu 1.11.5 | **Works.** Ran it against a `null_resource`; `terraform-live.tfstate` written as expected. The legacy `-state` flag is deprecated in docs but functional. Not a blocker. |
| Same-HCL dual-apply for Scaleway | **Sound.** `internal/cli/generate_command.go:70` emits a bare `provider "scaleway" {}`; the endpoint arrives purely via `SCW_API_URL`. Unset it and the provider talks to real Scaleway. This is why Layer 3 is Scaleway-only — GCP and AWS bake per-service endpoints into the HCL provider block (`buildGoogleProviderBlock:189`, `buildAwsProviderBlock:523`), so their HCL is not dual-apply-safe. |
| Auto-destroy on failed runs | **Implemented.** `internal/cli/run_command.go:688-706` fires `SandboxDestroy` when Layer 3 was enabled, `--no-destroy` is unset, and the terminal reason isn't `target_reached`. |
| `plan-live.txt` capture | **Implemented.** `internal/harness/sandbox_deploy.go` 3-stage flow; `test_command.go:491-498` pulls plan stdout from both the success and the `SandboxDeployError` paths. |

### Blockers found during planning — Layer 3 is currently unreachable end-to-end

| # | Blocker | Evidence | Slice |
|---|---|---|---|
| **B1** | **False-green env leak.** `sandboxCommandEnv()` (`internal/cli/test_command.go:559`) returns only `SCW_ACCESS_KEY` + `SCW_SECRET_KEY`. `execCommandRunner.Run` builds the subprocess env as `withEnvOverrides(stripGCPAuthEnv(os.Environ()), cmd.Env)` — an override map can *set* but never *unset*. So an inherited `SCW_API_URL` survives into the "real" apply and silently retargets it at mockway, which then reports `sandbox_deploy/apply: pass`. This is not hypothetical: mockway's own `make demo-env` writes `/tmp/mockway.env` exporting exactly that variable, and `cloudEnv()` sets it for every Layer 2 run. **No test asserts the sandbox env is free of it.** | `exec_runner.go:60-63`, `test_command.go:559-574` | S139 |
| **B2** | **Self-managed-project deadlock.** ADR-0010 decision 4 requires `scaleway_account_project` in the generated HCL, and `validateLayer3ProjectResource` (`generate_command.go:742`) hard-fails generation without it. But Layer 2 gates Layer 3, the *same* HCL is applied to mockway first, and mockway has no `/account/v3/projects` route — `handlers.go:447` registers only `/account/v2alpha1/ssh-keys`, so the project call falls through to `UnimplementedHandler` and 501s. Mock apply fails ⇒ Layer 3 never runs. **Every Layer 3 run today dies at Layer 2 before reaching Scaleway.** | mockway `handlers/handlers.go:447`, `handlers/unimplemented.go:9` | S140 |

B1 and B2 interact nastily: B2 means nobody has ever gotten far enough to notice B1.

### The four guardrails

All four were selected by the user as must-land-before-first-real-apply.

1. **Endpoint assertion** (S139) — seal the sandbox subprocess environment and fail closed if the effective endpoint isn't `api.scaleway.com`. Closes B1.
2. **Post-destroy orphan sweep** (S141) — today `no_orphans` is only ever evaluated against mockway state, so Layer 3 can leave billable resources behind and still report clean. Query the real API after destroy.
3. **Resource-type allowlist** (S142) — deny expensive types (k8s, HA RDB, xlarge Redis) unless explicitly opted in, so a runaway LLM iteration becomes a fast structural failure instead of a bill.
4. **Interrupt-safe destroy** (S141) — a Ctrl-C mid-apply leaves created resources in `terraform-live.tfstate` that auto-destroy never runs on. Handle signals; add a `reap` command for leftovers.

### Live-account constraints (verified 2026-08-22 against the real org)

Credentials are present and working (`~/.config/scw/config.yaml`, `scw account project list` succeeds). The org is **not empty**, which constrains the design:

| Fact | Consequence |
|---|---|
| Org `aacaa819-14b5-4c63-a4a4-58d54f81e6b6` contains two projects: `default` (id == org id) and **`openclaw` — "Openclaw AI Assistant Infrastructure"** | `openclaw` is live infrastructure the user depends on. Destroy, sweep and `reap` must be **structurally incapable** of touching it. Not "careful" — incapable. |
| Top-level `default_project_id` == the `default` project | A generated resource that omits `project_id` lands in `default`, **outside** the run's project. A sweep that only inspects the run's project would report clean while the stray resource bills forever. This is a hole in the naive sweep design; S141-T1a closes it. |
| `~/.config/scw/config.yaml` has a real `profiles:` block whose `myProfile` sets its own `api_url`, `default_project_id` and credentials | The Scaleway SDK reads config files, not just env vars. Stripping env is necessary but **not sufficient** — `SCW_PROFILE` / `SCW_CONFIG_PATH` must be stripped too (they are, S139-T2), and the endpoint assertion must run against the *resolved* config, not just the env map. |

**Hard rule for this arc: the only project any destroy/reap path may ever delete is one whose id was created by that same run and is recorded in that run's `terraform-live.tfstate`.** Everything else is off-limits by construction.

### Why `block-paris` is the canary

`scenarios/training/block-paris.yaml` is a single `scaleway_block_volume` plus a `region_restriction` policy check and `no_orphans`. Cheapest and fastest Scaleway resource in the set, no dependency graph, and destruction is immediate — so a failed teardown is unambiguous rather than an eventual-consistency question.

**Known limitation, accepted:** `block-paris` declares no `connectivity` / `http_probe` / `dns_resolution` criteria, so `RealProbeHarness` (`internal/harness/real_probe.go`) stays unexercised against real infrastructure in this arc. That is the natural phase-2 item — `lb-paris` (public LB + IP → `http_probe`) is the obvious follow-on canary. Recorded in the close-out as the next arc's opener, not smuggled into this one.

## Slices

| Slice | Title | Effort |
|---|---|---|
| S139 | Sealed sandbox environment + endpoint assertion (closes B1) | ~2–3 hr |
| S140 | mockway `account/v3` project CRUD + non-empty-delete semantics (closes B2) | ~2–3 hr |
| S141 | Real-API orphan sweep + interrupt-safe destroy + `reap` command | ~3–4 hr |
| S142 | Resource-type allowlist for sandbox deploy | ~1.5–2 hr |
| S143 | **Canary execution**: `block-paris` end-to-end against real Scaleway + ADR-0023 + arc close-out | ~2 hr + operator-gated run |

S139, S140 and S142 are mutually independent and can be worked in any order. S141 depends on S140 (the orphan sweep asks the project API whether the project is empty, and the mock path needs the same route to be testable). S143 is last and requires credentials.

## Standing rules

Inherited from prior arcs — restated because this arc touches real money.

- **NEVER hand-edit `pitfalls/*.yaml`.** Auto-learning writes them; prompts + code are the legitimate intervention points.
- **Don't let codex nitpick.** Substantive findings only; stop after 2 no-substantive passes.
- **Contract coverage, not line count** for any new tests (`feedback_test_coverage_metrics.md`).
- **Cost-sensitive CI** — nothing in this arc may add a scheduled/nightly job that spends real Scaleway money. Layer 3 stays opt-in and off by default in `internal/config/config.go:219-221`.
- **New in this arc — fail closed.** Every guardrail added here defaults to the *safe* branch when it cannot determine the answer. An orphan sweep that can't reach the API is a FAIL, not a skip. An endpoint it can't parse is a FAIL, not a pass.
- **Repo mechanics (verified 2026-08-22).** `main` carries an active ruleset (`deletion`, `non_fast_forward`, `pull_request` with `required_approving_review_count: 1`) on both infrafactory and mockway — classic branch protection returns 404, so check `gh api repos/<owner>/<repo>/rulesets`, not `/branches/main/protection`. Self-merge therefore needs `gh pr merge --squash --admin` (owner override), authorised by the user for this arc only. **Wait for CI on the actual head SHA** — `gh pr checks --watch` can return stale results after a force-push; confirm with `gh run list --branch <b> --json headSha,status`.
- **Don't commit `package-lock.json` churn.** Local npm (11.4.2) strips `libc: [glibc|musl]` fields from optional native deps that dependabot's newer npm wrote. `npm ci` rewrites the file as a side effect. Always `git checkout main -- ui/package-lock.json` before committing UI work unless a dependency change is the actual intent.
- **New in this arc — no real API calls in `go test ./...`.** All real-Scaleway execution stays behind the existing `INFRAFACTORY_ENABLE_REALTOOL_LAYER3*` env gates.

---

## S139 — Sealed sandbox environment + endpoint assertion

### Motivation

Closes B1. Until this lands, a green Layer 3 result is not evidence of anything: the most likely single cause of a "successful" first run is that it quietly went to mockway. This has to be the first slice so that every later slice's test signal is trustworthy.

The fix mirrors an existing, proven pattern in this codebase — `stripGCPAuthEnv` (`internal/cli/exec_runner.go:37`) already strips a family of inherited env vars for exactly this class of bug (ADR-0014 rule 4: the Google provider escaping to the real cloud because of inherited ADC vars). The difference is that GCP's strip is unconditional and global, while Scaleway's must apply **only** to sandbox commands — Layer 2 needs `SCW_API_URL` set.

### Tickets

| id | detail | priority | deps |
|---|---|---|---|
| S139-T1 | Add `StripEnv []string` to `harness.Command` (`internal/harness/static.go:12`). Honour it in `execCommandRunner.Run` — entries are removed from the inherited environment *before* `withEnvOverrides` applies `cmd.Env`. Strip is exact-key-match plus trailing-`*` prefix match, so `SCW_DEFAULT_*` works. Unit-test the runner directly: parent env containing `SCW_API_URL=http://mockway` + `StripEnv: ["SCW_API_URL"]` ⇒ absent from the resulting env slice. | P0 | — |
| S139-T2 | `SandboxDeployHarness.Run` and `SandboxDestroyHarness.Run` set `StripEnv` on every `Command` they build: `SCW_API_URL`, `SCW_INSECURE`, `SCW_DEFAULT_PROJECT_ID`, `SCW_DEFAULT_ORGANIZATION_ID`, `SCW_PROFILE`, `SCW_CONFIG_PATH`. The last two matter because a developer's `~/.config/scw/config.yaml` can silently supply an alternate endpoint or org. | P0 | S139-T1 |
| S139-T3 | Extend `sandboxCommandEnv()` (`test_command.go:559`) to emit the full real-provider env, not just the two keys: `SCW_ACCESS_KEY`, `SCW_SECRET_KEY`, `SCW_DEFAULT_ORGANIZATION_ID` (**new required credential** — `scaleway_account_project` needs an org to create the project in), `SCW_DEFAULT_REGION=fr-par`, `SCW_DEFAULT_ZONE=fr-par-1`. Region/zone come from new optional `scaleway.region` / `scaleway.zone` config fields defaulting to the Paris values every scenario in `scenarios/training/*-paris.yaml` assumes. Missing org id ⇒ same fail-closed preflight failure shape the two existing keys use. | P0 | — |
| S139-T4 | Add `assertRealScalewayEndpoint(env map[string]string) error`: fail unless the effective `SCW_API_URL` is absent (provider default) or exactly `https://api.scaleway.com`. Call it from `sandboxCommandEnv` after the strip list is applied, and again as an explicit `sandbox_deploy/preflight` stage so the assertion is visible in `StageSummary` output rather than buried. Failure detail must name the offending value — the whole point is that the operator sees "refusing to apply: SCW_API_URL=http://localhost:8080" instead of a false pass. | P0 | S139-T2, S139-T3 |
| S139-T4a | The endpoint assertion must run against the **resolved** provider config, not just the env map — the SDK reads `~/.config/scw/config.yaml`, and this machine's config has a real `profiles:` block whose `myProfile` carries its own `api_url` and `default_project_id`. Stripping `SCW_PROFILE`/`SCW_CONFIG_PATH` (T2) forces the default profile; assert additionally that the default profile's `api_url`, if present, is `https://api.scaleway.com`. Fail closed if the config file is unreadable *and* no explicit endpoint is set. | P0 | S139-T4 |
| S139-T5 | Regression test, the load-bearing one: `t.Setenv("SCW_API_URL", "http://localhost:8080")`, run the sandbox deploy path with a recording runner, assert (a) the subprocess env has no `SCW_API_URL`, and (b) the preflight assertion passed. Then a second case where config/flags force a non-Scaleway endpoint and assert the stage **fails**. Name them so intent survives: `TestSandboxEnvNeverInheritsMockwayURL`, `TestSandboxPreflightRejectsNonScalewayEndpoint`. | P0 | S139-T4 |
| S139-T6 | Update `internal/cli/cloud_parity_test.go:30` — its scaleway env expectation currently lists `SCW_API_URL`+`SCW_DEFAULT_PROJECT_ID` as required, which is a Layer 2 statement. Split the expectation per layer so the parity test doesn't fight the new sandbox shape. | P1 | S139-T3 |
| S139-T7 | Single PR. Title: `S139: seal sandbox env + assert real Scaleway endpoint (Layer 3 false-green fix)`. Description must state the failure mode in one paragraph — this is the finding a future reader most needs. | P0 | S139-T5, S139-T6 |

### Exit criteria

1. A parent environment polluted with `SCW_API_URL=http://localhost:8080` cannot reach a sandbox `tofu apply` — proven by test, not by inspection.
2. `sandboxCommandEnv` fails closed on a missing `SCW_DEFAULT_ORGANIZATION_ID`.
3. `sandbox_deploy/preflight` appears as a distinct stage in `test` output with an explicit pass/fail.
4. `go test -tags noui ./...` green; `bash scripts/check_all.sh` green.

---

## S140 — mockway `account/v3` project CRUD

### Motivation

Closes B2. This is the slice that makes Layer 3 reachable at all.

Beyond unblocking: modelling the project in mockway earns a genuine safety property. Real Scaleway **refuses to delete a non-empty project**. If mockway enforces the same rule, then a stack whose destroy ordering is wrong fails in Layer 2 in seconds — before it has cost anything — instead of failing at real-Scaleway teardown and leaving billable orphans behind. That is precisely the "Layer 2 gates Layer 3" bargain from ADR-0010 decision 1, applied to the one resource that governs blast radius.

Cross-repo: this is a **mockway PR**, following the sibling-fake conventions (`../mockway/AGENTS.md`).

### Tickets

| id | detail | priority | deps |
|---|---|---|---|
| S140-T1 | Confirm the wire shape before writing handlers. Read the `scaleway/scaleway` provider's `scaleway_account_project` resource against the schema the harness already fetches (`internal/harness/provider_schema.go`) and the public Account API v3 reference. Record method/path/status/body for create, get, list, delete in the PR description. mockway's fidelity strategy is **spec-driven** (per its AGENTS.md) — derive from the spec, don't guess from the error messages. | P0 | — |
| S140-T2 | `handlers/account.go` (new): `CreateProject`, `GetProject`, `ListProject`, `UpdateProject`, `DeleteProject`. Follow the existing handler idiom exactly — `decodeBody` → `app.repo.*` → `writeJSON` / `writeCreateError` / `writeDomainError`, list via `writeList(w, "projects", items)`. Pattern reference: `handlers/iam.go`. | P0 | S140-T1 |
| S140-T3 | Repository layer: project store with `id`, `name`, `description`, `organization_id`, `created_at`, `updated_at`. Auto-assign a UUID on create. Default-project semantics: seed one default project at boot so existing non-Layer-3 scenarios (which pass `SCW_DEFAULT_PROJECT_ID=00000000-...`) keep working unchanged. **This must not regress the 44/44 baseline.** | P0 | S140-T2 |
| S140-T4 | Non-empty-delete enforcement: `DeleteProject` walks the other resource stores for any object carrying that `project_id` and refuses with the real API's error shape when any remain. Detail must enumerate the blocking resource ids — that string is what the operator reads when a teardown goes wrong. | P0 | S140-T3 |
| S140-T5 | Register `r.Route("/account/v3", ...)` in `handlers/handlers.go` alongside the existing `/account/v2alpha1` block (~line 447). Leave the v2alpha1 ssh-keys alias untouched. | P0 | S140-T2 |
| S140-T6 | Contract tests per the family convention: `CRITICAL[account-project-delete-nonempty]:` docstring on `DeleteProject` paired with `TestContract_account_project_delete_nonempty`, and `CRITICAL[account-project-create-returns-id]:` paired likewise. `handlers/contract_audit_test.go` enforces the pairing automatically. Use testify (`require`/`assert`) per the repo's AGENTS.md testing rule. | P0 | S140-T4, S140-T5 |
| S140-T7 | Expose projects through the admin `/mock/state` surface so topology derivation and the existing orphan check can see them. Verify `internal/harness/topology_derive.go` tolerates the new key (it derives from raw mock state — a new top-level collection must not panic it). | P0 | S140-T3 |
| S140-T8 | Add `examples/working/account_project/` with HCL creating a project plus one child block volume wired via `project_id`, so the standalone `go test ./examples/...` matrix covers it. Mirrors the sibling example convention. | P1 | S140-T5 |
| S140-T9 | mockway PR. Title: `feat(account): account/v3 project CRUD with non-empty-delete enforcement`. Then an infrafactory-side check: run `block-paris` with `sandbox_deploy.enabled: true` and confirm **Layer 2 now passes** with `scaleway_account_project` in the HCL (Layer 3 will still fail on credentials — that's expected and is the correct next failure). | P0 | S140-T6, S140-T7 |

### Exit criteria

1. Layer 2 mock apply succeeds on HCL containing `scaleway_account_project` — B2 closed, verified by running `block-paris`.
2. Deleting a non-empty project against mockway fails with the resource ids named.
3. mockway `go test ./...` green; contract audit reports the 2 new pairs.
4. **44/44 baseline unregressed** — a full sweep is not required here, but the Scaleway subset must be spot-checked because T3 touches default-project seeding.

---

## S141 — Real-API orphan sweep + interrupt-safe destroy

### Motivation

Guardrails 2 and 4. These are the two ways Layer 3 can cost money silently.

Today `no_orphans` for a Layer 3 run is evaluated against **mockway state** — `evaluateSupportedCriteria` reads the mock deploy result, and nothing ever asks Scaleway whether anything survived. A destroy that half-worked reports clean.

The interrupt case is worse because it's the common one during development: Ctrl-C during a slow apply, or a context timeout. `terraform-live.tfstate` records what was created; nothing destroys it; the next run starts fresh and the resources bill forever.

The self-managed project from S140 makes the sweep cheap and exact: **the run's entire blast radius is one project**, so "did we leak?" reduces to "is that project gone, and if not, what's still in it?"

### Tickets

| id | detail | priority | deps |
|---|---|---|---|
| S141-T1 | `internal/harness/orphan_sweep.go` (new): `ScalewayOrphanSweep` with an injected HTTP doer (testable per the `RealProbeHarness` pattern — see `real_probe.go:37-67`). Reads the project id from `terraform-live.tfstate`, then `GET /account/v3/projects/{id}`. 404 ⇒ clean. 200 ⇒ enumerate the project's remaining resources and return them as `feedback.Failure` entries. **Any transport/auth error is a FAIL, never a skip** — an unverifiable sweep is exactly when you most want a red result. | P0 | S140 |
| S141-T1a | **Stray-resource check.** The project-scoped sweep is not sufficient on its own: a resource whose HCL omitted `project_id` is created in the org's `default` project, survives the run-project delete, and is invisible to a sweep that only looks at the run project. Assert that **every** resource in `terraform-live.tfstate` carries the run project's id; any resource with a different (or absent) project id is a FAIL naming the resource and where it actually landed. Cheap, exact, and catches the failure the sweep alone would miss. | P0 | S141-T1 |
| S141-T1b | **Protected-project denylist.** `SandboxDestroy`, the sweep and `reap` must refuse to delete any project id that was not created by the run being torn down — read the id from that run's `terraform-live.tfstate` and refuse anything else. Additionally hard-code a refusal for any project whose id equals the organization id (the `default` project). Test it directly: point `reap` at a state file naming a foreign project id and assert it refuses without issuing a delete. This is the guard that protects the `openclaw` project. | P0 | S141-T1 |
| S141-T2 | Wire the sweep into `test_command.go` after `SandboxDestroy` succeeds, gated on Layer 3 enabled. Emit as stage `sandbox_deploy/orphan_sweep`. When a scenario declares `destruction: no_orphans` **and** Layer 3 is on, this sweep — not the mock-state check — is what satisfies the criterion. The mock-state check still runs for Layer 2. | P0 | S141-T1 |
| S141-T3 | Same wiring on the failure path in `run_command.go:688-706`, so an auto-destroy after a failed run is also verified. A failed run that auto-destroys but leaks must report the leak loudly — this is the single highest-value alarm in the arc. | P0 | S141-T2 |
| S141-T4 | Signal handling: install a `SIGINT`/`SIGTERM` handler for the window between sandbox apply start and destroy completion. On signal, run `SandboxDestroy` with a fresh (non-cancelled) context and a bounded timeout, then exit non-zero. Print what it's doing — an operator who hits Ctrl-C twice needs to understand why it isn't exiting instantly. Second signal aborts immediately with a loud "resources may be live, run `infrafactory reap`" message naming the state file path. | P0 | — |
| S141-T5 | `infrafactory reap <scenario>` command: locate the scenario's `terraform-live.tfstate`, report what it contains, run `tofu destroy` against it with the sealed sandbox env from S139, then run the S141-T1 sweep. `--dry-run` lists without destroying. Register in `cmd/` following the existing command contract (ADR-0002). | P0 | S141-T1, S139 |
| S141-T6 | Tests: sweep returns clean on 404; sweep FAILS on 200-with-resources and names them; sweep FAILS on transport error; `reap --dry-run` mutates nothing; signal handler triggers destroy (drive it with a fake runner and a synthetic signal rather than actually killing the test process). | P0 | S141-T5 |
| S141-T7 | Single PR. Title: `S141: real-API orphan sweep + interrupt-safe destroy + reap command`. | P0 | S141-T6 |

### Exit criteria

1. A Layer 3 `no_orphans` verdict is backed by a real API query, not mock state.
2. Killing a run mid-apply destroys what was created, or exits telling the operator exactly how to reap it.
3. `infrafactory reap` destroys leftover live state and verifies the result.
4. Every unverifiable outcome is a FAIL.

---

## S142 — Resource-type allowlist for sandbox deploy

### Motivation

Guardrail 3. The iteration loop is an LLM writing HCL; the failure mode this defends against is a repair iteration that reaches for `scaleway_k8s_cluster` in a scenario meant to provision a block volume. Against mockway that costs nothing and gets caught. Against real Scaleway it's a slow, expensive apply followed by a slow destroy — several times over, since the loop iterates.

The check belongs next to `validateLayer3ProjectResource` in `generate_command.go`: post-generation, pre-apply, **before any API call is made**, and feedbackable so the LLM self-corrects on the next iteration rather than terminating the run.

### Tickets

| id | detail | priority | deps |
|---|---|---|---|
| S142-T1 | Config: `validation.layers.sandbox_deploy.allow_resource_types []string` on the existing `LayerConfig` (`internal/config/config.go:128`) — needs its own type since `LayerConfig` is shared by all four layers; introduce `SandboxLayerConfig` embedding `LayerConfig`, mirroring how `StaticLayerConfig` already extends it with `PolicyPaths`. Empty list ⇒ **deny-all with a clear message**, not allow-all. Fail closed. | P0 | — |
| S142-T2 | Default allowlist in `config.go` defaults (~line 219) covering the canary and the cheap tier: `scaleway_account_project`, `scaleway_block_volume`, `scaleway_block_snapshot`, `scaleway_vpc`, `scaleway_vpc_private_network`, `scaleway_lb`, `scaleway_lb_ip`, `scaleway_lb_backend`, `scaleway_lb_frontend`, `scaleway_domain_record`, `scaleway_iam_*`, `scaleway_registry_namespace`. Glob suffix `*` supported. Notably **absent**: `scaleway_k8s_*`, `scaleway_rdb_*`, `scaleway_redis_*`, `scaleway_instance_server`. | P0 | S142-T1 |
| S142-T3 | `validateLayer3ResourceAllowlist(outputDir, allowed)` in `generate_command.go`, called from the same place as `validateLayer3ProjectResource` (`:735`). Parse `resource "<type>" "<name>"` declarations with a regex — consistent with how `genesysFlowResourceRe` (`:767`) already works in this file. Denied ⇒ **feedbackable** failure listing the denied types and the allowed set, so the next iteration can route around it. | P0 | S142-T2 |
| S142-T4 | Document the escape hatch in `infrafactory.yaml` with a comment: widening the list is a deliberate, cost-bearing act. Add the expensive types as a commented-out block so opting in is a one-line uncomment with the cost implication visible right there. | P1 | S142-T2 |
| S142-T5 | Tests: denied type ⇒ feedbackable (not terminal) failure; allowed set ⇒ pass; glob matching; **empty allowlist ⇒ deny**; Layer 3 disabled ⇒ check skipped entirely. | P0 | S142-T3 |
| S142-T6 | Single PR. Title: `S142: resource-type allowlist for Layer 3 sandbox deploy`. | P0 | S142-T5 |

### Exit criteria

1. A generated stack containing `scaleway_k8s_cluster` cannot reach a real apply under default config.
2. Denial is feedbackable — the loop self-corrects instead of dying.
3. Empty/absent allowlist denies everything.

---

## S143 — Canary execution + ADR-0023 + close-out

### Motivation

The point of the arc. Everything before this is scaffolding; this slice is the first time infrafactory bills a real cloud.

**This slice is operator-gated.** It cannot run autonomously — it needs credentials that only the user can supply, and it spends real money. The autonomous loop stops at the end of S142 and hands back.

### Unattended execution (user directive, 2026-08-22)

The user reviewed the recommendation to hold S143 until they were back and **chose to proceed with the full real apply unattended**. That is their call and this arc executes it. Two things follow:

- **The guardrails must land and be proven before the canary runs.** S139–S142 are not optional preamble here; they are the only thing standing between an unattended loop and the `openclaw` project. Any guardrail whose test does not demonstrably bite (synthetic drift: break the guard, watch the test fail) blocks S143.
- **Failure is loud and conservative.** If destroy fails, retry once; if the sweep still reports anything live, stop the arc, leave the resources in place rather than improvising cleanup against a live account, and write the resource ids plus the exact `reap` invocation to the close-out where the user will see them first.

Residual risk is genuinely small — the allowlist denies every expensive type, the canary is a single block volume, and the blast radius is one purpose-created project — but it is not zero, and the honest failure mode is "a few cents of block storage survives until the user runs `reap`".

### Operator prerequisites (all verified present 2026-08-22)

| Requirement | Why | Notes |
|---|---|---|
| Scaleway account + org id | `scaleway_account_project` creates into an organization | `SCW_DEFAULT_ORGANIZATION_ID` |
| API key with **project-manager** rights | Creating/deleting projects is an org-level operation; a plain resource-scoped key will 403 at the very first resource | `SCW_ACCESS_KEY` / `SCW_SECRET_KEY` |
| Project quota headroom | Scaleway caps projects per org; the self-managed model consumes one per concurrent run | Check current count before the first run |
| Billing visibility | Confirm the operator can see the org's consumption page to independently verify zero spend after teardown | The sweep is our check; the console is the operator's |

### Tickets

| id | detail | priority | deps |
|---|---|---|---|
| S143-T1 | **Pre-flight dry run, no apply.** With credentials exported, run `block-paris` with Layer 3 enabled and the apply stage stubbed out, and confirm: preflight passes, endpoint asserts to `api.scaleway.com`, allowlist passes, `plan-live.txt` is captured and shows exactly the expected resource set (one project + one volume, nothing else). **Read the plan before allowing an apply.** | P0 | S139–S142 merged |
| S143-T2 | **First real apply.** `INFRAFACTORY_ENABLE_REALTOOL_LAYER3=1` with `--no-destroy`, so the resources persist and the operator can eyeball them in the Scaleway console. Verify the project exists, the volume exists, and it lives *inside that project* — not in the org default. Capture everything: stage summary, `plan-live.txt`, `terraform-live.tfstate`. | P0 | S143-T1 |
| S143-T3 | **Destroy + sweep.** `infrafactory reap block-paris` (or a normal destroy run). Confirm the volume is gone, the project delete succeeds, the orphan sweep returns clean, and the console independently shows nothing left. | P0 | S143-T2 |
| S143-T4 | **Full lifecycle, unattended.** Re-run without `--no-destroy` end-to-end: apply → policy check → destroy → sweep, all in one invocation, no manual steps. This is the actual arc goal. | P0 | S143-T3 |
| S143-T5 | **Negative test — the one that proves the guardrails.** Deliberately interrupt a run mid-apply (Ctrl-C) and confirm the signal handler destroys what was created and the sweep confirms clean. If it can't be triggered reliably by hand, force it with a short context timeout. | P0 | S143-T4 |
| S143-T6 | Record real-vs-mock behavioural deltas in `docs/mock-gaps.md` — anything real Scaleway did that mockway didn't (timing, eventual consistency, error shapes, required fields). **This is the fidelity payload of the whole arc**; the deltas are why Layer 3 exists. Feed genuine mock gaps back to mockway as follow-up tickets. | P0 | S143-T4 |
| S143-T7 | **ADR-0023: Layer 3 sealed environment and real-orphan verification.** Codifies: (a) the sandbox subprocess environment is sealed, not merely overridden, and the endpoint assertion is fail-closed; (b) for Layer 3, `no_orphans` means a real-API sweep, not mock state; (c) blast radius is exactly one self-managed project, which is what makes (b) cheap; (d) the allowlist is deny-by-default. Amends ADR-0010 with a pointer (it stays Accepted — decision 4 is unchanged, S140 made it true). Crosses the ADR threshold: it changes what a passing Layer 3 result *means*. | P0 | S143-T6 |
| S143-T8 | Arc close-out: `docs/status/ARCHIVE.md` section, `docs/NEXT_SESSION.md` repoint, `STATUS.md` update, `AGENTS.md:177` refresh (its Layer 3 paragraph predates all of this). Next-arc opener: **`lb-paris` as probe canary** — the real-probe path is still unexercised against real infrastructure. | P0 | S143-T7 |
| S143-T9 | PR. Title: `S143: first real Scaleway apply (block-paris canary) + ADR-0023 + arc close-out`. | P0 | S143-T8 |

### Exit criteria

1. `block-paris` completes apply → verify → destroy against real Scaleway in a single unattended invocation.
2. The orphan sweep confirms clean **and** the Scaleway console independently confirms it.
3. An interrupted run self-cleans.
4. Real-vs-mock deltas are written down.
5. ADR-0023 merged; close-out done.

---

## Verification

```bash
# Unit + integration (no real API calls — must stay true)
go test -tags noui ./internal/harness ./internal/cli ./internal/api ./internal/config
go test -tags noui ./...
bash scripts/check_all.sh

# mockway side (S140)
cd ../mockway && go test ./... && go test ./examples/...

# Layer 3 against real Scaleway — S143 only, operator-gated, costs money
export SCW_ACCESS_KEY=... SCW_SECRET_KEY=... SCW_DEFAULT_ORGANIZATION_ID=...
INFRAFACTORY_ENABLE_REALTOOL_LAYER3=1 \
  go test ./internal/cli -run TestTestCommandRealToolLayer3Smoke -count=1 -timeout=600s
```

## Out of scope

- **The other 15 Scaleway scenarios.** Canary first. Expansion is the next arc.
- **Real-probe validation** (`connectivity` / `http_probe` / `dns_resolution` against real infrastructure). `block-paris` declares no probes. `lb-paris` is the designated follow-on.
- **Layer 3 for GCP / AWS / Genesys.** Structurally blocked: those providers bake endpoints into the HCL provider block, so the same-HCL dual-apply contract doesn't hold. Would need per-layer provider-block rendering — a separate arc with its own ADR.
- **Cost estimation.** Still deferred per ADR-0010 (no reliable Scaleway pricing source). The allowlist is the cost control, not a price model.
- **Concurrent Layer 3 runs.** Single-run guard already exists in the UI; CLI is single-process. Project-per-run would make concurrency safe, but nothing needs it yet.
- **CI integration.** Cost-sensitive-CI standing rule. Layer 3 stays manual and opt-in.

## Autonomous execution loop prompt

> Execute `docs/plans/layer3-real-scaleway-plan.md` slices **S139 through S142 only**. S143 is operator-gated — it requires real Scaleway credentials and spends money; stop at the end of S142 and report back.
>
> One PR per slice. Run `go test -tags noui ./...` and `bash scripts/check_all.sh` before each PR. For S140, work in `../mockway` and follow that repo's AGENTS.md conventions (spec-driven fidelity, testify assertions, `CRITICAL[id]:` ↔ `TestContract_<id>` pairing).
>
> The fail-closed rule is absolute: wherever a guardrail cannot determine the answer, the answer is FAIL. Do not add a skip path, and do not soften an assertion to make a test pass.
>
> No real Scaleway API calls at any point in S139–S142. If a test would need credentials, gate it behind the existing `INFRAFACTORY_ENABLE_REALTOOL_LAYER3` env var and leave it skipped.

## Fresh-context checklist

1. `AGENTS.md` § "Planning a New Arc" + § "Fresh Context"
2. This plan
3. `docs/decisions/0010-layer3-real-scaleway-deploy.md` (Layer 3 governance; supersedes ADR-0003)
4. `docs/decisions/0014-provider-endpoint-flag-discipline.md` rule 4 — the env-strip precedent S139 mirrors
5. `docs/plans/layer3-production-plan.md` — S30, what was built and why
6. `CONCEPT.md:519` + `:626` — self-managed project lifecycle contract
7. `internal/cli/test_command.go:52-130` (`cloudEnv`) and `:559` (`sandboxCommandEnv`) — the two env paths this arc separates
