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

Supporting these, the sandbox environment requires `SCW_DEFAULT_ORGANIZATION_ID` (a project must be created somewhere) and pins region/zone from config rather than inheriting them *(S139)*.

5. **Expensive resource types are denied by default, before any API call.** `validation.layers.sandbox_deploy.allow_resource_types` gates which types may reach a real apply, checked after generation and before apply so a denied type costs nothing. The list is deny-by-default — empty or absent denies everything — because the iteration loop is an LLM writing HCL, and the failure mode of a permissive default is a bill that repeats once per repair iteration. *(S142)*

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

**Amendment — fallback project containment (2026-08-22).** Rule 3 detects resources created outside the run project; it does not stop them being created there. A generated resource that omits `project_id` does not fail — it lands wherever the provider resolves the default project, which on a normal account is the organization's `default`, alongside whatever real infrastructure lives there. `scaleway.fallback_project_id` points that default at a dedicated, disposable project instead, so a stray is contained rather than mixed in. It is refused if set to the organization id, since that is the default project itself and the setting would silently be a no-op. Detection is unchanged: the sweep still reports strays as leaks.

**Implementation status (2026-08-22)**: all five rules are landed. S139 (rules 1–2), S141 (rules 3–4: sweep, stray-resource check, `AssertProjectDeletable`), S141b (the interrupt path), S141c (fallback-project containment), S142 (rule 5).

The interrupt path closed the last gap. `withSandboxInterruptGuard` installs a SIGINT/SIGTERM handler for the window in which real resources exist: the in-flight tofu call unwinds on the cancelled context, then destroy runs on a **fresh** one, because doing work after cancellation is the entire point. A second signal restores default handling and aborts, printing the state path and the exact recovery command. `infrafactory reap <scenario>` handles the cases where the process never got to run its own cleanup, gated by the same `AssertProjectDeletable` and verified by the same sweep — a reap that cannot prove the account is clean fails.

**Amendment — capture before destroy (2026-08-22, from the first canary run).** Rule 3 originally had the sweep read `terraform-live.tfstate` at sweep time. That is after `tofu destroy` has emptied it, so the project id was gone and the sweep could never determine the blast radius — it failed closed on teardowns that had actually worked. Correct for a fail-closed guard, useless as a tool. Capture and verification are now split: `harness.CaptureSweepTarget` reads the live state **before** destroy, and the sweep verifies against that captured target. The stray-resource check moved with it for the same reason.

This is the first defect the arc found that no unit test or mock could have: it only appears when a real destroy actually empties real state.

**Amendment — a failing guard must say why (2026-08-22, from the LLM-generated canary run).** The rules above decide *whether* a Layer 3 run may proceed and what a pass means. They said nothing about what a **failure** reports, and the answer turned out to be almost nothing: both Layer 3 paths built their `FailureSummary.Detail` from the bare exec error, so a real Scaleway failure surfaced as `exit status 1` and the provider's message — captured by the harness in `SandboxDeployError.Apply.Stderr` — was discarded. The sandbox destroy path had the same hole, which matters more: a failed real destroy is the orphaned-billing case.

Fail-closed is necessary but not sufficient. A guard that stops the run without saying why is only half a guard, and the asymmetry is specific to Layer 3: against a mock, reproducing `exit status 1` under a debugger is free; against a real API it costs money and a project-create round trip, so the first report has to carry the message. Layer 2 had solved this long ago in `mockDeployFailureDetail`; the shared `stderrFailureDetail` helper now backs all four paths (mock deploy, destroy, sandbox deploy, sandbox destroy), ANSI-stripped and bounded by the same `failureStderrDetailMaxChars` budget.

The failure this surfaced is recorded in `docs/layer3-real-vs-mock-deltas.md` (D1): real Scaleway returned a create error *after* the block volume existed, leaving it tainted with computed fields unset. It was transient — a re-apply succeeded — and Layer 3 has no retry, so a single API blip fails an otherwise-correct run. That retry is the arc's top follow-up, not part of this decision.

**Enforcement**: the guards carry synthetic-drift coverage — removing `SCW_API_URL` from `SandboxStripEnv` fails three tests, verified before S139 merged. Following the project's "drift becomes failed `go test`" pattern.
