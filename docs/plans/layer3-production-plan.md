# Plan: Layer 3 Production Readiness (Slice 30)

## Context

Slices 26-29 built the Layer 3 harness infrastructure: sandbox deploy/destroy (`tofu apply/destroy` with `terraform-live.tfstate`), real network probes (TCP, HTTP, DNS), credential validation, UI toggle, and opt-in E2E tests. All of this works against mockway as a proxy for real Scaleway.

What's missing is the operational glue to make Layer 3 reliable against **real Scaleway APIs** — plan capture, auto-destroy on failure paths, holdout integration, and validation that the generated HCL actually bootstraps its own project. This slice closes those gaps so a user can confidently run `infrafactory run --no-destroy` with `sandbox_deploy.enabled: true` against real Scaleway.

## Quick Reference

| Key | Value |
|---|---|
| Slice | 30 |
| Ticket IDs | S30-T1 through S30-T5 |
| Depends on | Slices 22-29 (all done) |
| ADRs | ADR-0010 (Layer 3), ADR-0009 (incremental) |
| Config gate | `validation.layers.sandbox_deploy.enabled: true` |
| Credentials | `SCW_ACCESS_KEY`, `SCW_SECRET_KEY` env vars (org-level permissions) |
| Project mgmt | Self-managed via `scaleway_account_project` in generated HCL (ADR-0010) |

## What's Already Working (verified during planning)

These were initially flagged as gaps but are actually implemented:

| Capability | Where |
|---|---|
| API `layer3_enabled` per-run override | `server.go:40`, `ui_command.go:191-192` — API flag overrides config |
| `GET /api/scenarios/{path}/layer3-status` | `handlers_scenarios.go:137-204` — returns credential/config readiness |
| `terraform-live.tfstate` preserved in incremental mode | `generate_command.go:252-274` — only `*.tf`/`*.tf.json` deleted |
| `Layer3Guidance` in prompts (includes `scaleway_account_project`) | `claude_adapter.go:215-224`, `openrouter_adapter.go:174` |
| `Layer3Enabled` flows into generator `Request` | `generator.go:33`, both adapters pass it to prompt templates |
| Run metadata records `layer3_enabled` | `run_command.go:100,249` |

## Remaining Gaps

### Gap 1: No `plan-live.txt` artifact capture (Contract #8)

**Problem**: The sandbox deploy harness runs `tofu apply` directly. There's no `tofu plan` step before apply, so there's no plan output to persist. CONCEPT.md Contract #8 specifies `plan-live.txt` as a per-iteration artifact.

**Fix**: Add a plan step to `SandboxDeployHarness.Run()` before apply. Capture stdout to `plan-live.txt` via the runstore.

**Files**:
- `internal/harness/sandbox_deploy.go` — add `tofu plan -state=terraform-live.tfstate` stage before apply
- `internal/harness/sandbox_deploy.go` — extend `SandboxDeployResult` with `Plan StageResult`
- `internal/cli/test_command.go` — persist `plan-live.txt` from deploy result
- `internal/cli/run_command.go` — persist `plan-live.txt` in iteration artifacts
- `internal/harness/sandbox_deploy_test.go` — update tests for 3-stage flow

### Gap 2: Auto-destroy real resources on run failure (Contract #14)

**Problem**: When a run fails (budget exhausted or stuck) without `--no-destroy`, the run command doesn't auto-destroy real Scaleway resources. Mock resources get reset, but real resources are left running, incurring billing.

The destroy matrix from Contract #14:
| Outcome | `--no-destroy` | Default |
|---|---|---|
| Converges | Skip destroy. State persists. | Destroy mock + real. |
| Fails | Skip destroy. State persists. | Auto-destroy real (billing protection). Reset mock. |

Currently: `run_command.go` only runs holdouts + destroy on `target_reached && !NoDestroy`. Failed runs just exit.

**Fix**: After the iteration loop, if Layer 3 is enabled and `!controls.NoDestroy`, run sandbox destroy regardless of outcome. This mirrors the mock destroy that already happens in test_command.go.

**Files**:
- `internal/cli/run_command.go` — add post-loop Layer 3 cleanup for non-`--no-destroy` failed runs
- `internal/cli/run_command_test.go` — test: failed run auto-destroys real resources
- `internal/cli/run_command_test.go` — test: `--no-destroy` preserves real resources on failure

### Gap 3: Holdout checks with Layer 3 (Contract #10)

**Problem**: When Layer 3 is enabled and a run converges, holdout checks should run against both mock and real state (dual-apply pattern). Currently `runCriteriaOnlyHoldouts` only runs `executeTest` with default options — it doesn't pass through Layer 3 enablement.

**Fix**: Pass Layer 3 config through to holdout test execution so sandbox deploy/destroy and real probes execute during holdouts.

**Files**:
- `internal/cli/run_command.go` — `runCriteriaOnlyHoldouts` already calls `executeTest` which reads `runtime.Config.Validation.Layers.SandboxDeploy.Enabled`, so this may already work. Needs verification test.
- `internal/cli/run_command_test.go` — add test: holdouts with Layer 3 enabled execute sandbox deploy + real probes

### Gap 4: `scaleway_account_project` validation (Contract #12)

**Problem**: Per ADR-0010, the generated HCL always includes `scaleway_account_project` as a resource when Layer 3 is enabled — infrafactory self-manages project lifecycle. The prompt guidance tells the LLM to include it, but there's no post-generation validation that it actually did. If the LLM omits it, `tofu apply` deploys into whatever `SCW_DEFAULT_PROJECT_ID` happens to be set (or the org default), breaking the self-managed lifecycle contract.

**Fix**: Add a post-generation check: if Layer 3 is enabled, verify at least one `resource "scaleway_account_project"` exists in the generated `.tf` files. Missing → feedbackable failure so the LLM can self-correct on the next iteration.

Also: remove the `scaleway.sandbox_project_id` config field and `SCW_DEFAULT_PROJECT_ID` passthrough from `sandboxCommandEnv()` — these are incompatible with the self-managed model and create confusion.

**Files**:
- `internal/cli/generate_command.go` — add `validateLayer3ProjectResource()` check after `writeGeneratedFiles`
- `internal/cli/generate_command_test.go` — test: Layer 3 enabled + missing resource = feedbackable failure
- `internal/cli/generate_command_test.go` — test: Layer 3 disabled = check skipped
- `internal/cli/test_command.go` — remove `SCW_DEFAULT_PROJECT_ID` / `sandbox_project_id` from `sandboxCommandEnv()`
- `internal/config/config.go` — remove `SandboxProjectID` field from `ScalewayConfig`
- `internal/api/handlers_scenarios.go` — remove `project_id_configured` from layer3-status response

### Gap 5: Close `S9-T8` governance ticket

**Problem**: `S9-T8` is still marked `blocked` in BACKLOG.md. It's the original sandbox/live deploy ticket from Slice 9 that was superseded by Slices 26-29 (ADR-0010). It should be closed.

**Fix**: Update BACKLOG.md to mark `S9-T8` as `done` with a note referencing Slices 26-29.

**Files**:
- `BACKLOG.md` — change S9-T8 status from `blocked` to `done`

---

## Tickets

### S30-T1: Capture `plan-live.txt` artifact during sandbox deploy

**Scope**: Add `tofu plan` stage to `SandboxDeployHarness`, persist output as iteration artifact.

**Acceptance criteria**:
1. `SandboxDeployHarness.Run()` executes `tofu plan -state=terraform-live.tfstate` before `tofu apply`.
2. `SandboxDeployResult` includes `Plan StageResult` with stdout/stderr.
3. `plan-live.txt` is written to `.infrafactory/runs/{scenario}/{run_id}/iterations/{n}/plan-live.txt` when Layer 3 is enabled.
4. `GET /api/runs/{scenario}/{run_id}/iterations/{n}/files/plan-live.txt` returns the artifact (existing runstore infrastructure handles this).
5. Harness unit tests updated for 3-stage flow (plan → init → apply).

**Files**: `internal/harness/sandbox_deploy.go`, `internal/harness/sandbox_deploy_test.go`, `internal/cli/test_command.go`, `internal/cli/run_command.go`

### S30-T2: Auto-destroy real resources on failed runs

**Scope**: When a run fails and `--no-destroy` is not set, auto-destroy real Scaleway resources after the iteration loop exits.

**Acceptance criteria**:
1. Failed run (`repair_budget_exhausted` or `stuck`) without `--no-destroy` triggers `SandboxDestroy.Run()` if Layer 3 was enabled and at least one sandbox deploy succeeded.
2. Auto-destroy failure is logged but does not mask the original run failure.
3. `--no-destroy` + failed run preserves real resources (no destroy attempt).
4. Regression tests for all four cells of the destroy behavior matrix (Contract #14).

**Files**: `internal/cli/run_command.go`, `internal/cli/run_command_test.go`

### S30-T3: Validate `scaleway_account_project` in generated HCL + remove `sandbox_project_id`

**Scope**: Enforce ADR-0010 self-managed project lifecycle. Validate generated HCL includes `scaleway_account_project` when Layer 3 is enabled. Remove the `sandbox_project_id` config field that conflicts with self-managed mode.

**Acceptance criteria**:
1. After `writeGeneratedFiles`, if `Layer3Enabled`, scan `.tf` files for `resource "scaleway_account_project"`.
2. Missing → feedbackable failure (not terminal), included in `FeedbackJSON` for next iteration.
3. Layer 3 disabled → check skipped.
4. `scaleway.sandbox_project_id` config field removed from `ScalewayConfig`.
5. `sandboxCommandEnv()` no longer passes `SCW_DEFAULT_PROJECT_ID`.
6. `layer3-status` endpoint no longer reports `project_id_configured`.
7. Unit tests for validation and config cleanup.

**Files**: `internal/cli/generate_command.go`, `internal/cli/generate_command_test.go`, `internal/cli/test_command.go`, `internal/config/config.go`, `internal/api/handlers_scenarios.go`, `internal/api/handlers_scenarios_test.go`

### S30-T4: Holdout + Layer 3 verification

**Scope**: Verify holdout checks execute Layer 3 when enabled. Add coverage.

**Acceptance criteria**:
1. Test proving `runCriteriaOnlyHoldouts` with `sandbox_deploy.enabled=true` triggers sandbox deploy and real probes during holdout execution.
2. If holdout sandbox deploy fails, failure is captured in holdout results (not a crash).

**Files**: `internal/cli/run_command.go` (if changes needed), `internal/cli/run_command_test.go`

### S30-T5: Close S9-T8 + update docs

**Scope**: Governance cleanup and doc updates.

**Acceptance criteria**:
1. `S9-T8` status changed to `done` in BACKLOG.md with note: "Superseded by Slices 26-30 (ADR-0010)".
2. STATUS.md updated with Slice 30 summary.
3. CURRENT_TICKET.md populated for session execution.

**Files**: `BACKLOG.md`, `STATUS.md`, `CURRENT_TICKET.md`

---

## Execution Order

```
S30-T1 (plan-live artifact) ──┐
S30-T3 (project validation) ──┼── can run in parallel
S30-T4 (holdout verification) ┘
         │
S30-T2 (auto-destroy on failure) ── depends on understanding T1's harness changes
         │
S30-T5 (docs cleanup) ── last, after all code is done
```

## Verification

```bash
# Unit tests
go test -tags noui ./internal/harness ./internal/cli ./internal/api ./internal/runstore

# Full suite
go test -tags noui ./...
bash scripts/check_all.sh

# Opt-in real Scaleway (requires credentials + budget)
INFRAFACTORY_ENABLE_REALTOOL_LAYER3=1 go test ./internal/cli -run TestTestCommandRealToolLayer3Smoke -count=1 -timeout=300s
```

## Out of Scope

- **Cost estimation** — deferred indefinitely per ADR-0010 (no reliable Scaleway pricing source).
- **Zone/region availability validation** — depends on real API behavior, not predictable.
- **Credential validity check** (pre-auth against Scaleway API) — tofu init/apply will surface this; adding a separate check adds complexity for marginal benefit.
- **Concurrent sandbox deploy locking** — single-run guard already exists in UI; CLI is single-process.
