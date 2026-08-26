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

**Amendment — verify the cleanup you just did, and tolerate a flaky API once (2026-08-22, closing the two follow-ups the canary left open).**

*Sweep on the failure path.* Rule 3 made `no_orphans` a real-API sweep, but only on the path where the run **succeeded**. A run that failed auto-destroyed its real resources and then trusted that destroy worked — so the cleanup most likely to leave orphans, a half-finished apply, was the one path that never verified itself. `run_command.go` now captures the sweep target before destroy (same ordering the success path learned the hard way) and sweeps afterwards.

The run has already failed by then, so this cannot change the exit code. What it changes is what the operator is told: an unreachable or dirty sweep now reports explicitly, and every failure raised while real resources may still exist carries the exact recovery command (`infrafactory reap <scenario>`) in its **detail**, not only in the structured log — the failure list is what a human reads in the terminal the moment the run stops, and the alternative to reading it there is finding out from a bill. This extends rule 3's principle rather than changing it: "we could not check" and "nothing leaked" must never look alike, on either path.

*Bounded apply retry.* Layer 3 had no retry, so the transient the canary hit — a create error returned *after* the resource existed, leaving it tainted — failed an otherwise-correct run, and under `infrafactory run` burned a repair iteration teaching the LLM nothing, because the HCL was fine. Apply now runs at most `sandboxApplyAttempts` (2) times. A second apply replaces the tainted resource, which is the documented tofu behaviour, so the retry is a genuine fix rather than a hope.

Two bounds keep this from weakening the guarantees above. It is **one** retry, not a loop: a genuinely broken plan fails fast rather than billing for every attempt. And it **never retries a cancelled context** — on the interrupt path the whole point is to stop touching the API and get to destroy, so a retry there would create exactly what the operator just asked us to stop creating. Retries are surfaced (`succeeded on attempt 2`) rather than swallowed, because a silently-retried apply is indistinguishable from one that worked first time, and the difference is a real API flapping.

**Amendment — the recovery command must be pasteable (codex review pass 1).** The `infrafactory reap <scenario>` hint added above is only worth printing if it actually runs. Scenario paths are interpolated into a shell command the operator copies at the moment real resources may be billing, so they are shell-quoted (`shellQuote`, standard shlex algorithm) rather than concatenated. Already-safe paths stay unquoted so the line remains easy to scan under stress. Verified by round-tripping paths containing spaces, quotes and metacharacters through `/bin/sh`.

**Amendment — clean up after a failed apply, not only a successful one (2026-08-23, from the lb-paris probe canary).** The sweep-on-failure amendment above fixed `run`. `test` kept the original gate: sandbox destroy and sweep ran only when the apply had **succeeded**. That is backwards. `tofu` creates resources one at a time and writes each to state as it goes, so an apply that dies partway is precisely the case that has left real infrastructure behind.

The canary demonstrated it rather than arguing it: `scaleway_account_project` and `scaleway_lb_ip` were created, the next call failed on an API permission error, cleanup was skipped because the apply had not succeeded, and a real project plus a real load-balancer IP billed until they were reaped by hand. Cleanup is now gated on the sandbox layer having **run**, not on it having worked.

Note what did work, on first real use: the failure arrived as `insufficient permissions: read loadbalancer` rather than `exit status 1`, and the retry reported `failed 2 attempts, so not a transient blip`. Both landed the previous day and both paid for themselves here.

**Amendment — the credential is part of the blast radius (2026-08-23).** Rules 3 and 4 bound what a run *does*; nothing bounded what its credential *could* do. Every Layer 3 run so far authenticated as `openclaw-terraform` — an API key belonging to the organization **owner**, with full rights over every project including the one holding live infrastructure. The sealed-environment rule made that inevitable rather than accidental: S139 strips `SCW_PROFILE` and `SCW_CONFIG_PATH` so a developer profile cannot redirect the endpoint, which also forces the *default* profile, and on this machine the default profile is the owner.

So the entire "blast radius is one disposable project" guarantee rested on software: `AssertProjectDeletable`, the sweep, the allowlist. Correct code, but one bug away from reaching real infrastructure, with nothing behind it.

Layer 3 now authenticates as a dedicated IAM **application** (`infrafactory-layer3`), never a user, with a policy granting `ProjectManager` plus only the allowlisted product families (`BlockStorageFullAccess`, `LoadBalancersFullAccess`, `VPCFullAccess`). Instances, IAM, registry, serverless, object storage and billing are all absent, verified by probe: creating an Instance flexible IP, a registry namespace, or an IAM application all return 403, while an LB IP returns 200.

That last point is what makes it defence in depth rather than decoration: `openclaw-prod` and its flexible IP are Instances resources, so a guardrail bug can no longer reach them **at all** — the API refuses before any of our checks run.

Two limits worth stating rather than discovering later. Product permission sets are project-scoped and each run creates its own project, so the rules are organization-scoped to cover projects that do not exist yet; the key can therefore still act on block/LB/VPC resources in *existing* projects.

And three entries in the default `allow_resource_types` are now **allowed locally but refused by the API**: `scaleway_iam*`, `scaleway_registry_namespace`, and `scaleway_domain*`. The first two are obvious — a sandbox that can mint IAM credentials is not a sandbox. Domains are excluded for the same least-privilege reason and a specific one: `DomainsDNSFullAccess` would let a canary modify real DNS, and no scenario in the suite manages domains today. (`dns_resolution` probes only *resolve* names; they need no domain permission.)

A run that generates one of those types will pass the allowlist and then fail at the real API with a 403, which reads like a bug unless you know this. Widening the policy is the fix when a scenario genuinely needs one — and it is a blast-radius decision, exactly like widening the allowlist, belonging in the same review.

**Amendment — Instances granted, and what that costs (2026-08-23).** The credential amendment above listed Instances as absent, and said that was what put `openclaw-prod` and its flexible IP structurally out of reach. `InstancesFullAccess` has now been added to the policy, by explicit decision, to allow a backend server behind the load balancer — which is the difference between proving a load balancer *accepts TCP* and proving it *serves traffic*.

State the cost plainly rather than let it decay into folklore. The API no longer refuses this key on Instances operations, so `openclaw-prod` is protected by software again — the deny-by-default allowlist (which still excludes `scaleway_instance_*`), `AssertProjectDeletable`, the orphan sweep, and project-per-run. That is the position every other resource family is already in, and it is the position the whole arc was built to make safe; it is simply no longer *also* backstopped by the API for this family.

Two gates remain and only one moved. Widening the allowlist to admit `scaleway_instance_*` is a separate decision from widening the policy, and should be taken separately.

Still absent, and deliberately: IAM (no privilege escalation), registry, serverless, object storage, billing, domains.

**Amendment — the allowlist gates every real apply, not just generated ones (2026-08-24).** Rule 5 says expensive types are denied by default before any API call. The check only ever ran in the *generation* path, which was sufficient while every Layer 3 run came from the generator. The S144 PR gate broke that assumption: it stages committed HCL from `examples/layer3-gate/` and calls `test`, so the allowlist was configured and never consulted, and a pull request could have introduced a `scaleway_k8s_cluster` that applied for real.

Enforcement moved to where the guarantee belongs — immediately before the sandbox deploy, so it covers any route to a real apply rather than one particular caller. The gate's HCL arrives from a pull request, which is precisely the untrusted-input case rule 5 exists for.

All three of the pre-apply checks moved with it — the project resource (ADR-0010's blast-radius boundary), the type allowlist, and a new escape-hatch refusal.

**Untrusted HCL is validated by parsing, and deny-by-default on block type.** This took three attempts, and the failures are more instructive than the answer.

A resource-type allowlist is sufficient for *generated* HCL, because the generator emits flat `resource` blocks and nothing else. It is not sufficient for HCL arriving from a pull request. First a `module` block declares resources the scan never sees; then a `provisioner` executes commands during apply inside a process holding live credentials. Both were denylisted — and `data "external"` with `program = [...]` still ran arbitrary commands, at *plan* time, before any check that inspects plan output could help.

Then the scanner itself proved unsound: `resource /*x*/ "scaleway_k8s_cluster"` is valid HCL that a `resource\s+"` pattern does not match, and the grammar permits comments and whitespace almost anywhere, so no expression closes the class.

The configuration is now parsed (`hashicorp/hcl/v2`) and validated against an **allowlist of block types** — `terraform`, `provider`, `variable`, `output`, `locals`, `resource`, `data` — with providers and data sources restricted to Scaleway, resource types checked against `allow_resource_types`, and `provisioner`/`connection` refused anywhere in the tree. Unparseable input is refused rather than applied: unknowable is not the same as harmless.

**"Before the real apply" was the wrong bar; "before any tofu" is the right one.** The structural checks originally ran immediately before the *sandbox* deploy — but Layer 2's mock deploy runs `tofu init/apply` on the same configuration first, in the same process, whose environment holds the credentials. A `provisioner` or `data "external"` would therefore have executed during the mock deploy, before the check that exists to refuse it. Validation now happens before either layer runs.

A refused configuration means **no tofu at all**, including for cleanup: `tofu destroy` evaluates the same configuration, and destroy-time provisioners exist, so "run it just to tidy up" would grant exactly the execution the refusal denied. That leaves one honest gap — resources recorded by an earlier run are not destroyed — and it is reported loudly rather than papered over, because a human deciding what to do beats a machine executing HCL it has just declared untrustworthy.

**The workflow is part of the artefact too.** `pull_request` loads the workflow file from the PR head, so a same-repo PR could rewrite the gate itself to skip the trusted checkout and print the credentials — making every other control decorative. The gate therefore runs on `pull_request_target`, which loads the workflow from base.

That reverses a warning an earlier revision of this ADR carried, and the reversal is not a change of mind so much as a distinction. `pull_request_target` is dangerous when it grants secrets to a job that then checks out **and executes** PR code. This job never does: it takes exactly one thing from the PR — `.tf` files, copied as data — and everything executable comes from base, namely the workflow, the infrafactory binary, mockway at a pinned SHA, and tofu. The HCL is then validated by that trusted binary before any apply. The same-repo guard is kept regardless, so a fork cannot reach the job at all.

**Existence of a project is not the same as using it.** Checking that a `scaleway_account_project` is declared satisfies the letter of ADR-0010 while a fixture pins its actual resources elsewhere with `project_id = "<another project>"` — and on this account "elsewhere" includes the project holding live infrastructure. A `project_id` must now be a direct reference to the stack's own project resource; literals, variables and data lookups are all refused, because only a reference is provably the project the sweep will destroy.

**The validator cannot live in the artefact being validated.** The gate originally built `infrafactory` from the PR and then ran that binary with `SCW_ACCESS_KEY` and `SCW_SECRET_KEY` in its environment. A pull request can change `internal/cli` as easily as it changes HCL, so a compromised binary exfiltrates the credentials before performing any of the checks it is supposed to perform — and every control above becomes decorative. The tool is now built from the **base** commit and only the HCL comes from the PR head. That is also the honest model: the PR proposes infrastructure, trusted machinery applies it.

**A provider binary is code.** `required_providers { scaleway = { source = "attacker/scaleway" } }` keeps the local name that satisfies the `provider "scaleway"` check, while `tofu init` downloads and executes that plugin with the credentials in scope. The source must now be exactly `scaleway/scaleway`, and any other required provider is refused.

**What is NOT closed, stated plainly.** Terraform evaluates expressions at plan time, and functions like `file()` can read from the runner. A fixture could put `file("/proc/self/environ")` where its value reaches tofu's output. Both publication channels are now filtered to structured stage lines: the PR comment and the Actions log, which on a public repository is equally readable. tofu's own output never reaches the console directly — infrafactory captures it into stage results — so filtering what the workflow echoes closes the automatic channel. The cost is accepted deliberately: a genuine gate failure is less debuggable from the log, and the answer is to reproduce it locally with the same fixture, which needs no credentials for a plan-time failure.

That narrows the exposure rather than removing it. An attacker-chosen expression still executes; it just has nowhere obvious to print.

The control that actually answers this threat is the one already in place: the real-cloud path runs only for same-repo pull requests, and the `layer3` environment requires a reviewer. It takes a trusted collaborator to reach the credentials at all, and a human approves each run. Structural validation raises the cost of a mistake; it does not replace the human gate, and it should not be described as though it does.

Two general lessons, both learned the expensive way. **A denylist of known escapes is a race you lose** — each fix invited the next bypass, and only enumerating what is permitted ended it. And **a scanner that is not a parser will always have a gap**, because the grammar is richer than the pattern. Neither would have mattered for generated HCL; both matter the moment the input comes from a pull request.

**Consequence for cleanup.** With validation now able to refuse *before* any apply, "did this run attempt an apply" stopped being a usable cleanup gate — a refusal can coincide with an earlier run's resources still being recorded, and those still need destroying. The live state is now the only gate: what matters is whether resources may exist, not who created them.

Generalising: a control attached to *one path into* a dangerous operation is a control that a second path silently bypasses. Attach it to the operation.

**Enforcement**: the guards carry synthetic-drift coverage — removing `SCW_API_URL` from `SandboxStripEnv` fails three tests, verified before S139 merged. Following the project's "drift becomes failed `go test`" pattern.

**Amendment — the allowlist admits two Instances types (2026-08-24).** The Instances amendment above closed by saying that widening the allowlist to admit `scaleway_instance_*` was a separate decision, to be taken separately. This is that decision.

`scaleway_instance_ip` and `scaleway_instance_server` are now allowed. They are named individually rather than as a glob: this is a deny-by-default list, and `scaleway_instance_*` would also admit snapshots and images no scenario needs.

The reason is `http_probe`. `lb-paris` asserts *connectivity* — a TCP connect — and its own comment explains why: it declares no compute, so the load balancer's backend has no servers and Scaleway answers HTTP with 503. `lb-serving-paris` puts one small instance behind the frontend, and the probe then proves the frontend **serves traffic** rather than merely accepting connections. Against real Scaleway the whole scenario — apply, HTTP 200 through the load balancer, destroy, sweep — completes in 144 seconds.

The cost is that a *second* family is now protected by software rather than by the API, joining block, LB and VPC. The guards are unchanged and still apply: project-per-run, `AssertProjectDeletable`, the orphan sweep, and the allowlist itself.

**Amendment — the API creates resources Terraform will not destroy (2026-08-24).** ADR-0010's disposable project is the foundation of every blast-radius claim here, and it silently stopped holding for any scenario that declares compute.

`tofu destroy` cannot delete a project that still contains anything, and Scaleway puts something in it without being asked: the first Instance in a fresh project causes a **"Default security group"** to be auto-created there. It is absent from the plan, absent from the state file, and not Terraform's to remove. Destroy therefore ends with `precondition failed: resource is still in use`, and the run leaks a project. Nothing billable survives, so a cost-based check reports clean while projects accumulate one per run.

`destroySandbox` now purges what the API auto-created inside the run's own project and retries the destroy once. Three constraints make that safe to state:

1. **Scoped to the run's project**, which callers have already put through `AssertProjectDeletable`.
2. **Only `project_default` groups.** A security group the run's own HCL declared is Terraform's to destroy; purging it here would hide a real destroy bug.
3. **Reported, never silent.** What was removed goes in the stage summary. That is not decoration — the first verification run passed with the purge firing invisibly, and the stage output was the only way to distinguish the fix working from the destroy having coincidentally succeeded.

The purge is deliberately lenient where the sweep is strict. `ScalewayOrphanSweep` remains the authoritative "did we leak?" answer and still fails closed; a purge that cannot list a zone simply moves on, because a transient list error must not turn into a failed teardown when the sweep is there to catch a genuine leak.

No mock surfaces this class of defect at all. The auto-creation is a behaviour of the real API, not of the configuration, so mockway — which creates exactly what it is asked for and nothing else — deletes its project cleanly every time.

**Amendment — the coverage document's counts are CI-enforced (2026-08-24).** `docs/layer3-coverage.md` is not commentary. It is the artifact someone reads to decide which scenario to point at real, billable infrastructure next, and its central claims are counts: how many scenarios have run, how many each gate blocks, which resource types are admitted locally and refused by the credential.

Those counts were hand-maintained, and over a single slice they drifted four times — a scenario counted in the numerator and not the denominator, a gated remainder that no longer matched the table, a "three families" claim that had become four, and an enumerated allowlist that had fallen behind `infrafactory.yaml`. Each was a paragraph disagreeing with a table two screens away, and each hand-fix introduced the next.

`TestLayer3CoverageDocTotalsMatchItsTable` and `TestLayer3CoverageDocAllowlistMatchesConfig` now derive the totals and the gated remainder from the table's own rows, and diff the enumerated allowlist against the config. Both carry synthetic-drift coverage.

This extends the project's existing "drift becomes a failed `go test`" pattern — ADR-0021's cloud-prefix lockstep, the sibling contract audits, the pitfalls/OPA dedup check — to prose. The justification is the same one the pattern always rests on: a convention nobody can violate accidentally is worth more than a convention written down. It applies here because the prose makes checkable numerical claims about committed configuration, not because documentation in general should be tested.

**Amendment — the README's gate claim is CI-enforced (2026-08-24).** The README describes what Layer 3 guarantees, and a reader has no way to check whether the described pre-merge gate actually runs. It briefly did not: the section was written while `.github/workflows/layer3-gate.yml` still lived on an unmerged branch, so a public safety claim outran the repository by one merge.

`TestReadmeLayer3GateClaimMatchesWorkflows` ties the two together in both directions. With no gate workflow present the README must carry its "not yet merged" qualification; once the workflow lands the same test fails until the qualification is removed. Both directions carry synthetic-drift coverage.

The asymmetry is the point. Overstating a safety control is the dangerous direction — someone relies on a gate that is not there — but understating it is how a repository ends up with a working control nobody knows about. Neither should survive a merge.

**Amendment — function calls are allowlisted, not banned (2026-08-26).** S144 closed a credential-exfiltration path by refusing **every** function call in HCL that Layer 3 evaluates. The reasoning was sound and the scope was wrong.

The path is real: `user_data = file("/proc/self/environ")` reads `SCW_ACCESS_KEY` and `SCW_SECRET_KEY` out of the runner's environment and hands them to a machine whose boot script the pull request wrote, and `data "external"` and provisioners were already blocked, so an ordinary attribute was what remained. A blanket ban closed it.

It also broke the product's main path, which was not noticed because the rule was written against the gate and tested against the gate. `validateLayer3HCLShape` runs on **generated** HCL as well as on a PR's, and the first real generation after it landed emitted

    tags = concat(var.tags, ["web-server"])

which Layer 3 refused. `concat` computes over its arguments and reaches nothing else; refusing it protected against nothing and made Layer 3 unusable for every generated stack.

The rule is now an allowlist of **pure** functions — ones whose result cannot depend on anything outside their arguments. That keeps deny-by-default, which is the property that matters on this surface: an explicit list of dangerous names (`file`, `templatefile`, `fileset`, `filebase64`, `abspath`, `pathexpand`) would be another enumerate-the-bad-ones list, and every one of those attempted here produced a bypass. Adding an entry asks one question: can this function's result depend on anything other than what it was passed?

Two things worth keeping from how this was found. The rule was **verified against the generator rather than against the tests** — 40 review passes and a full suite all agreed the blanket ban was fine, and a single real `infrafactory generate` disagreed. And a check that runs on two paths needs exercising on both: this one was written for untrusted input and silently applied to trusted input, where its cost was invisible until something real ran through it.

**Related, and recorded here because it constrains the demo:** a real generation of `lb-serving-paris` is refused by the gate for a different reason — it emits `scaleway_instance_security_group` and private networking, which need `IPAMFullAccess` and are not allowlisted. That is the two gates working as designed, not a defect. `block-paris` generates cleanly and is therefore the demo scenario. Widening the allowlist to accommodate a richer generated stack remains a blast-radius decision, unchanged by the demo's convenience.
