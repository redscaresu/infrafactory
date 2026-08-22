# ADR-0023: Layer 3 Sealed Environment and Real-Orphan Verification

## Status
Accepted (amends ADR-0010)

## Context

ADR-0010 enabled Layer 3 (real Scaleway deploy) and Slices 26–30 built the harness. Planning the first actual real-Scaleway run (2026-08-22, `docs/plans/layer3-real-scaleway-plan.md`) established that the harness had **never made a single call to `api.scaleway.com`** — every stage of it was validated against mockway standing in for Scaleway, and both real-tool tests are env-gated with no record of either running with credentials.

Reading the code for the first time with "does this actually reach Scaleway?" as the question surfaced two structural problems and one measurement problem.

**The environment is not sealed.** `sandboxCommandEnv` returns an *override map*, and `execCommandRunner` composes the subprocess environment as `withEnvOverrides(stripGCPAuthEnv(os.Environ()), cmd.Env)`. An override map can set a key but never unset one. An inherited `SCW_API_URL` therefore survived into the sandbox apply and silently retargeted it at mockway, which then reported `sandbox_deploy/apply: pass`. The variable is routinely present: `cloudEnv` sets it for every Layer 2 run, and mockway's `make demo-env` writes it to `/tmp/mockway.env`. This is the same class of bug as ADR-0014 rule 4, where the Google provider escaped to the real cloud because of inherited ADC variables.

**Env-stripping alone is insufficient.** The Scaleway SDK reads `~/.config/scw/config.yaml` regardless of environment, and that file's top-level keys constitute the default profile. A developer whose config sets `api_url` gets a redirected apply with a perfectly clean environment.

**"No orphans" was never measured against reality.** `destruction: no_orphans` is evaluated against mockway state. For a Layer 3 run this means a destroy that half-worked reports clean while real resources keep billing. Nothing ever asked Scaleway what survived.

Two further constraints came from the live account. It contains a project (`openclaw`) carrying infrastructure the user depends on, and the configured `default_project_id` points at the org's `default` project — so a generated resource that omits `project_id` lands *outside* the run's own project, where a project-scoped sweep would not see it.

## Decision

Four rules govern any Layer 3 execution. All four fail closed: where a guard cannot determine the answer, the answer is failure, never skip.

1. **The sandbox environment is sealed, not merely overridden.** `harness.Command` carries `StripEnv`, a per-command list of keys removed from the inherited environment before overrides apply. All sandbox commands declare `harness.SandboxStripEnv` (`SCW_API_URL`, `SCW_INSECURE`, `SCW_DEFAULT_PROJECT_ID`, `SCW_DEFAULT_ORGANIZATION_ID`, `SCW_PROFILE`, `SCW_CONFIG_PATH`). The strip is per-command rather than global because Layer 2 legitimately requires `SCW_API_URL`. *(S139)*

2. **The endpoint is asserted against the resolved configuration, not just the environment.** `assertRealScalewayEndpoint` checks the override map, the inherited variable, and the default profile in `~/.config/scw/config.yaml`, refusing anything that is not `https://api.scaleway.com`. Named entries under `profiles:` are ignored by design — `SCW_PROFILE` is stripped, so they are unreachable. *(S139)*

3. **For Layer 3, `no_orphans` means a real-API sweep.** After destroy, the run queries Scaleway to confirm its project is gone or empty, and separately asserts that every resource in `terraform-live.tfstate` carried the run project's id — catching resources that landed in the org default project, which a project-scoped sweep alone would miss. A sweep that cannot reach the API is a failure. *(S141)*

4. **Blast radius is exactly one purpose-created project, and nothing else is deletable.** Per ADR-0010 decision 4 each run creates its own `scaleway_account_project`. No destroy, sweep or `reap` path may delete a project whose id was not created by that run and recorded in its `terraform-live.tfstate`; a project id equal to the organization id (the `default` project) is refused unconditionally. This is what makes rule 3 cheap, and what protects pre-existing projects. *(S141)*

Supporting these, the sandbox environment requires `SCW_DEFAULT_ORGANIZATION_ID` (a project must be created somewhere) and pins region/zone from config rather than inheriting them, and expensive resource types are denied by default via a config allowlist enforced before any API call *(S142)*.

## Consequences

**Benefits**:
- A green Layer 3 result becomes evidence. Previously the most likely cause of a passing run was that it never left the mock.
- The failure mode of a mistake is a red test, not a silent bill.
- Project-per-run means Scaleway's own refusal to delete a non-empty project acts as a free, loud orphan detector.
- Pre-existing infrastructure is protected structurally rather than by care.

**Tradeoffs**:
- Layer 3 now requires a third credential (`SCW_DEFAULT_ORGANIZATION_ID`) and an API key with project-manager rights.
- The endpoint assertion reads a file outside the repo, so tests touching `sandboxCommandEnv` must isolate `XDG_CONFIG_HOME` or they become machine-dependent.
- The allowlist makes expensive scenarios opt-in, so widening Layer 3 coverage is a deliberate, cost-bearing act.

**Relationship to ADR-0010**: amends rather than supersedes. ADR-0010's decisions stand — including decision 4 (self-managed project lifecycle), which S140 made actually reachable by teaching mockway `/account/v3/projects`. This ADR constrains *how* a Layer 3 run may execute and what a passing result means.

**Enforcement**: the guards carry synthetic-drift coverage — removing `SCW_API_URL` from `SandboxStripEnv` fails three tests, verified before S139 merged. Following the project's "drift becomes failed `go test`" pattern.
