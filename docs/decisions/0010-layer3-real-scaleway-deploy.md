# ADR-0010: Layer 3 Real Scaleway Deploy

## Status
Accepted (supersedes ADR-0003)

## Context
ADR-0003 permanently blocked Layer 3 (real Scaleway sandbox deploy) due to cost, credentials, and safety concerns. The Mock Fidelity Limitations section in CONCEPT.md documents why the mock cannot fully validate behavioral correctness — the FK constraints model provider expectations, not actual Scaleway API behavior.

With the incremental deployment model (ADR-0009, Slices 22-25) maturing, the gap between mock validation and real-world behavior becomes the primary remaining risk. Closing this gap requires running `tofu apply` against real Scaleway infrastructure.

Governance prerequisites from ADR-0003:
1. **Cost**: no reliable Scaleway pricing source exists. User controls cost through scenario scope. Development environments are assumed non-live.
2. **Credentials**: user provides `SCW_ACCESS_KEY`/`SCW_SECRET_KEY` env vars. Same mechanism as mock credentials, but with real values.
3. **Safety**: Layer 2 (mockway) gates Layer 3 — structural errors caught in seconds before real API calls. Auto-destroy on failure prevents orphaned billable resources.

## Decision
Enable Layer 3 real Scaleway deployment as an optional validation layer, superseding ADR-0003's permanent block.

1. **Layer 2 gates Layer 3** — Layer 3 only runs if Layers 1+2 pass. Fail fast on cheap mock validation before spending real API calls and time.
2. **Same HCL, dual apply** — the same generated `.tf` files are applied to both mockway and real Scaleway within a single iteration. Separate `.tfstate` files: `terraform.tfstate` (mock) and `terraform-live.tfstate` (real).
3. **Credentials via env vars** — `SCW_ACCESS_KEY`/`SCW_SECRET_KEY` with real org-level permissions. No new credential management.
4. **Project bootstrap in HCL** — the generated HCL includes `scaleway_account_project` as a resource. No pre-existing sandbox project required. Factory creates and destroys the project as part of the IaC lifecycle.
5. **Destroy behavior respects `--no-destroy`**:
   - `--no-destroy`: keep all resources (mock + real) regardless of run outcome.
   - No `--no-destroy` + converges: `tofu destroy` against both mock and real Scaleway after verification.
   - No `--no-destroy` + fails: auto-destroy real resources, reset mock. Prevents orphaned billable resources.
6. **Config-controlled** — `validation.layers.sandbox_deploy.enabled: true` in `infrafactory.yaml`. Default remains `false` for backwards compatibility and CI safety.
7. **Layer 3 failures feed back** — real Scaleway failures are included in structured failure JSON and fed into the next iteration's 3-phase pipeline, same as Layer 1/2 failures.
8. **UI + CLI parity** — Layer 3 is triggered the same way from both CLI and UI. The UI shows Layer 3 status alongside Layer 2 in the Live page.

## Consequences
**Benefits**:
- Closes the mock fidelity gap — validates actual API behavior, sequencing, eventual consistency, and real network probes.
- Unlocks `dns_resolution` acceptance criteria and real `connectivity`/`http_probe` probes.
- Project bootstrap via HCL means no long-lived sandbox project or manual setup.
- Layer 2 pre-check keeps the feedback loop fast for structural issues.

**Tradeoffs**:
- Real API calls cost money (user-controlled via scenario scope).
- Real resource provisioning is slow (K8s clusters: minutes, RDB instances: minutes).
- Iteration loop is slower when Layer 3 is enabled — Layer 2 catches most issues fast, but Layer 3 failures require a full re-iteration.
- Two `.tfstate` files per scenario adds state management complexity.
- Cost estimation remains deferred (no reliable Scaleway pricing source).

**Migration**:
- ADR-0003 is superseded. `S9-T8` is unblocked.
- `validation.layers.sandbox_deploy.enabled` defaults to `false` — existing workflows unchanged.
- CI/regression scenarios continue to run against mockway only unless explicitly configured.
