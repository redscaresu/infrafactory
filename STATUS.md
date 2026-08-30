# STATUS

Last updated: 2026-08-24

## Current phase

- 🔧 **S153b (2026-08-30)** — the review of S153a found 15 more findings, several
  of them **regressions S153a itself introduced**. Worst: the empty-state shortcut
  released a record with a PASS without re-running the orphan sweep, so a teardown
  whose sweep had *failed* was laundered green on the next pass and the orphans
  became invisible. The sweep is now re-run against the record's project id, and
  the record is released only if it passes.

  Also fixed: `exec.CommandContext` sent **SIGKILL** to `tofu`, so an interrupted
  apply could leave a resource live and absent from state — now SIGINT plus a
  bounded `WaitDelay`, because tofu forks provider plugins that hold the output
  pipes. An undecodable record had **no CLI route out** (`live teardown` failed at
  load, `live reap` failed forever) — new `live forget` releases without
  destroying, says exactly what it gives up, and preserves the unparseable bytes
  beside the record rather than overwriting the project id they may still contain.
  `validateID` now guards **reads** as well as writes. Records written with a
  relative `WorkDir` resolve deterministically. The deploy failure hint is keyed
  off whether registration *succeeded*, so it no longer says "nothing to tear
  down" in the one case where a project is live and unrecordable. A cancelled
  parent is no longer reported as a Ctrl-C. `live ls --output json` marks
  unreadable records instead of emitting phantom deployments.

  **And generation is unblocked.** `claude_adapter.go` filtered only
  `CLAUDECODE=`, while a parent Claude Code session exports nine more
  (`CLAUDE_CODE_MESSAGING_SOCKET`, `CLAUDE_CODE_BRIDGE_SESSION_ID`, …). The nested
  `claude` inherited them, behaved as part of the parent session, and hung on the
  first phase that used a **tool** — `self_review`, the only one that writes files
  — until the 300s timeout. Phases 1 and 2 are pure text and completed, which is
  why it looked like a `self_review` bug rather than an environment one. Now
  prefix-matched so a new variable is excluded by default.

  **Verified, not inferred**: provider 2.81.0's `scaleway_instance_private_nic`
  has no `project_id` attribute, so the containment conflict cannot be fixed in
  HCL. And **#163 (TypeScript 5→7) does not install** — `@sveltejs/kit@2.70.3`
  declares `peerOptional typescript@"^5.3.3 || ^6.0.0"`, so npm refuses without
  `--force`. TypeScript 6 would satisfy that range.

- 🔧 **S153a hardening (2026-08-30)** — a post-merge audit of #170 returned
  **15 findings, several of them real leaks** in code that destroys real
  infrastructure. #170 merged without a review pass; the loop now runs on every
  PR before merge, not after.

  Two were principles written into a doc and not implemented beside it.
  **ADR-0024 rule 3 promises "unreadable means expired"** — but `Reapable`
  filtered only decodable records, so an undecodable one never reached the reaper
  and `MarkReleased` could not clear it either: it failed every pass, forever,
  with no way out. And **`live reap --dry-run` returned `nil`**, discarding
  failures already recorded for unreadable records, so a dry run exited 0 while
  something that might be running was unaccounted for.

  The leaks: **second-resolution deployment ids** meant two deploys of one
  scenario in the same second shared a record path *and* a workdir, so the second
  apply adopted the first's state and left a project running with nothing that
  knew how to destroy it — the per-deployment workdir defeated by the id, and the
  arc's own test slept 1100ms to dodge it. **No SIGINT guard on `deploy`**, so
  Ctrl-C during a ~140s apply killed the process with the project already created
  and the record not yet written — `live ls` showed nothing and it billed
  indefinitely. **A relative store root** made a scheduled reaper in any other
  directory report "nothing has expired" and exit 0. **Non-atomic writes**
  manufactured exactly the truncated record that could not be recovered. And
  **unsanitised ids** reached `filepath.Join`.

  Also fixed: teardown of an already-destroyed deployment reported a permanent
  false leak alarm (destroy empties the state the next pass reads); `--output
  json` was unparseable for `deploy` and `live reap`; the deploy failure hint
  pointed at teardown even when nothing was recorded; `live ls` swallowed stray
  args; and `docs/layer3-coverage.md` contradicted itself on the training-set
  denominator — the same unenforced-prose bug S152 took credit for catching.

  **Two misdiagnoses of the private-networking blocker, both retracted.** The
  repo said `IPAMFullAccess`; the real API wanted `write compute_private_networks`
  (`PrivateNetworksFullAccess`, granted). Private *networks* now create; private
  *NICs* still cannot, and no permission fixes it — the resource takes no
  `project_id`, so it lands in the ADR-0010 containment project while the server
  is in the run's own. Then I claimed `vpc_required.rego` was "wired for AWS only"
  and narrowed the pitfall on that basis. Also false: `filterPolicyPathsByCloud`
  drops only *other* clouds, so all five `policies/scaleway/*.rego` run. A
  generation run failed on exactly that rule. Pitfall restored, claim retracted in
  four files.

  **What is actually true is worse: the two gates contradict each other.** Layer 1
  requires a private NIC on every `scaleway_instance_server`; Layer 3 cannot apply
  one while the run creates its own project. **No Scaleway compute scenario
  satisfies both today.** That — not IPAM, not permissions, not the pitfall — is
  what blocks generated HCL from reaching real infrastructure.

  **Generation was also blocked on this machine, and is now fixed.**
  `claude_adapter.go` filtered only `CLAUDECODE=`, while a parent Claude Code
  session exports nine more `CLAUDE_CODE_*` variables. The nested `claude`
  inherited them, behaved as a child session, and hung on the first phase that
  used a **tool** — `self_review`, the only one that writes files — until the 300s
  timeout. With the family unset, all three phases complete.

- ✅ **Live-services canary green (2026-08-30)** — `deploy` → `live ls` →
  `live teardown` proven against **real Scaleway**, first attempt. Deploy 35.8s,
  teardown 36.1s, **HTTP 200 through a real load balancer**, account back to its
  3 baseline projects with the canary project returning 404 — verified against
  the API, not from the command's own report.

  Seeded HCL, deliberately: the `lb-serving-paris` fixture stood in for generated
  output so any failure would be S151–S153 code rather than generation (the S143
  pattern). **D6 reproduced itself in the new teardown path** — Scaleway
  auto-created a `Default security group`, destroy could not delete the project
  while it existed, and the S146 purge-and-retry cleared it *and said so*. Without
  that reporting the teardown would have read as an ordinary clean run.

  **Gap it exposed**: `deploy` records the *declared* image without verifying what
  is running. The record said `nginx:1.27`; the instance served
  `python3 -m http.server`. Queued for S155, where upgrade makes it load-bearing.

- 🚧 **S153 landed (2026-08-30)** — the first slice where something stays up,
  and the thing that takes it down, in one PR. `infrafactory deploy` applies a
  validated scenario and records it; `live teardown <id>` destroys one;
  `live reap` destroys everything past its TTL.

  **`deploy` deliberately does not generate.** `run` proves a change is safe;
  `deploy` takes what `run` validated and keeps it. The allowlist is still
  re-checked at deploy time, deny-by-default — *already validated* is a claim
  about a previous command.

  **Every deployment gets its own workdir** (`<live root>/workdirs/<id>`), never
  the shared `output/<scenario>`. Two deployments of one scenario would otherwise
  share a single `terraform-live.tfstate` and the second apply would overwrite
  the first's, orphaning real resources with nothing left that knows how to
  destroy them — the D6 class again, invisible to any cost check.

  **The registry is not authority for what gets destroyed.** It says which
  deployment; the state file says which project, and `AssertProjectDeletable`
  gets both, so a disagreement is fatal. A deployment whose state has vanished is
  reported and **not** released: its resources may still be running, and
  releasing it would retire the only record that says so.

  Registration happens on the failure path too — a half-finished apply leaves
  real resources, and the record is the only thing that brings the reaper back.

  **Known hole, deliberately left open**: the reaper trusts the store to know
  every live deployment. A wiped working directory loses records while the cloud
  keeps the resources. Reconcile-against-API is the largest remaining gap in
  ADR-0024 and is queued for S157.

- 🚧 **S152 landed (2026-08-30)** — the app gets a version. `service:` in
  `scenario.schema.json` names the image, tag, port, health path and TTL that a
  live-service scenario runs, and `scenarios/training/web-live-paris.yaml` is the
  first to declare one. Infrastructure-only scenarios are unaffected: no
  `service:` block means nothing changes.

  **The tag must not move.** `latest`, `stable`, `edge`, `main`, `master` and
  friends are refused by the loader, because an upgrade from a tag that moves
  cannot be told from a no-op and a soak failure cannot be attributed to a
  version. This catches the common case and is not a proof of immutability — a
  numeric tag like `1` moves too, and only a digest is genuinely fixed. TTL is
  bounded at 168h as a typo control, not a cost control: `400h` where `4h` was
  meant should fail at validation rather than on an invoice.

  Caught by the audit tests, and worth recording because it is the S147 failure
  class exactly: `docs/layer3-coverage.md` counts `**runnable**` rows as
  scenarios that **have run**, so adding `web-live-paris` as runnable would have
  made the doc claim a real-cloud run that never happened. The table had no way
  to say *ungated but never run*. It now does — `runnable, unrun`, a fourth
  bucket in both the doc and `TestLayer3CoverageDocTotalsMatchItsTable`. Ungated
  is 4 of 18; have-run remains 3 of 18, and the row becomes `**runnable**` the
  day it goes green, not before.

- 🚧 **S151 landed (2026-08-30)** — first slice of the live-services arc
  (`docs/plans/live-services-arc-plan.md`). `internal/livestore` records
  infrastructure that deliberately outlives the run which created it, and
  `infrafactory live ls` reports what is running and how long it has left.

  Nothing persists yet — this slice is bookkeeping, on purpose: there must be a
  record of what is out there before anything is allowed to stay up. The
  decisions are in **ADR-0024**, and all of them fail toward teardown. An expiry
  is mandatory and there is no value meaning "forever". A record that will not
  decode, or that lacks an expiry or a project id, is reported as **expired and
  reapable** rather than skipped — "we could not check" must never look like
  "nothing is running", so `live ls` exits non-zero and names the record.

  Caught while writing it: `MarkReleased` originally routed through `Put`, which
  validates. A damaged record is still reapable and its resources are still
  real, so the reaper would have destroyed them, failed to record that it had,
  and destroyed the same already-destroyed deployment on every later pass.
  Releasing now bypasses validation; creating still does not.

  Live deployments will get their own Scaleway project rather than teaching the
  orphan sweep to discriminate live resources from strays — that would make
  sweep correctness depend on registry accuracy, where one stale entry is a
  silent leak (the D6 class). **The ephemeral invariant is therefore unchanged:
  per-run projects are still destroyed and swept.**

- 🔒 **S150 landed (2026-08-24)** — first slice of the Presentable arc, taken before S144 so the workflow that will hold cloud credentials is born into a hardened pipeline. `govulncheck` in CI, explicit `permissions:` on every workflow, SHA-pinned actions on the two workflows with elevated capability. **The scanner found ~20 known stdlib advisories on its first run**: `go.mod` pinned `go 1.25.0` and several were genuinely reachable (`GO-2026-6218` in `net/url` via `http.Client.Do` from `internal/api/server.go`), so every binary built here was linked against a vulnerable stdlib. Now pinned to `go 1.25.13`. Posture and the goldfinger comparison in `docs/ci-security-posture.md`.

- 🚦 **S148 in progress (2026-08-26)** — `make demo-gate` puts a generated change on a PR, labels it, waits on the deployment approval and follows the run; `make demo-preflight` fails rather than reads, and checks the account is clean before you start. **The demo is agent-authored**: `infrafactory generate` writes the HCL and a script only carries it to a PR, so the agent in the talk's title is on screen rather than taken on faith.

  Finding that changed the design: a real generation of `lb-serving-paris` is **refused by the gate** — the generator emits `scaleway_instance_security_group` and private networking, which need IPAM and are not allowlisted. `block-paris` generates cleanly in 76s and passes every check, so it is the demo scenario. The same run also showed the function ban was too strict to survive contact with the generator (`tags = concat(...)`), which is now a pure-function allowlist.

  Outstanding: S148-T2 (recorded fallback — needs one approved real run to record) and S148-T3 (three rehearsals, operator-only).

- 🔒 **S144 landed (2026-08-25)** — the arc's spine. `.github/workflows/layer3-gate.yml`: apply the `layer3-gate` label to a PR and, **after a human approves the deployment**, the gate applies that PR's infrastructure code to real Scaleway, probes it, destroys it, sweeps the account and comments the stage summary back. That comment is the artifact the talk rests on.

  **27 codex passes, 25 findings.** Almost all of one class: the gate evaluates PR-supplied HCL with real cloud credentials in the environment, and every enumerate-the-dangerous-ones guard produced a bypass. What survived is deny-by-default at each layer — an allowlist of top-level block types, providers restricted to `scaleway/scaleway` **pinned to an exact version** the base-branch binary carries plus a trusted `.terraform.lock.hcl` copied from the base checkout, every resource required to bind `project_id` to the run's own disposable project (or, for child resources, an in-stack parent), and **all function calls refused** — `user_data = file("/proc/self/environ")` reads `SCW_SECRET_KEY` and ships it to a machine the PR boot-scripts. The binary and harness come from the base branch; only HCL comes from the PR, because a maintainer label is not a security boundary.

  The `layer3` environment carries **required reviewers**, deliberately. The approval prompt is the thesis made visible: the *without breaking it* is a human approving a real-cloud apply, and the machine then proving it applied, verified and destroyed.

  Verified end to end against real Scaleway with every restriction active: 144s, provider 2.81.0 from the trusted lock, account clean.

- 🔒 **S145 + S146 landed (2026-08-24)** — the evidence and the coverage. `examples/layer3-plan-lied/` holds two infrastructure changes that clear `validate`, clear `plan`, clear a **mock apply**, and are refused by real Scaleway, both 10/10 reproducible: a block volume asking for `iops = 9000` (the API sells 5000 or 15000) and a DNS zone under a credential not permitted to create one. Which of the two a mock *could* have caught is written down, because presenting the first as though it were the second is the fastest way to lose a technical audience.

  `lb-serving-paris` is the first scenario to satisfy an `http_probe` against real Scaleway — one instance serving behind a real load balancer, green end to end in **144 seconds**. Building it surfaced **D6**: Scaleway auto-creates a `project_default` security group on the first Instance in a project, Terraform never destroys it, and so `tofu destroy` cannot delete the project. ADR-0010's disposable project — the foundation of every blast-radius claim in the arc — had silently stopped holding for any scenario declaring compute, and nothing billable survived, so a cost check would have kept reporting clean. `destroySandbox` now purges what the API auto-created inside the run's own project and retries once: guarded by `AssertProjectDeletable` in the wrapper rather than at the call sites, restricted to groups the API marks `project_default` *and* reports as belonging to the run's project, and **reported in the stage summary** — the first verification run passed with the purge firing invisibly, and that output was the only way to tell the fix from a coincidence. Also mockway#22: the provider polls a v2alpha1 endpoint mockway answered with 501, and Layer 2 gates Layer 3.

- 🔒 **S147 landed (2026-08-24)** — `docs/layer3-evidence.md`: the six findings Layer 3 produced that Layer 2 and unit tests did not, each with its *mechanism*, because "the real cloud is different" is folklore and a mechanism is not. The denominator leads rather than trails: **3 of 17** Scaleway scenarios have ever run against the real API, on **1 of 4** clouds, over roughly 50 applies mostly repeating two fixtures. Three scenarios is not a trend and no rate is claimed from it. Numbers are measured — eleven real gate runs, the six green ones at 60–95s, `lb-serving-paris` at 143/144/144s — and there are no euro figures, because Scaleway pricing is not codeable and an invented number is worse than none.

  Four consecutive review passes then found counting errors in `docs/layer3-coverage.md`, each one a paragraph disagreeing with a table two screens away, and fixing each by hand introduced the next. The totals, the gated remainder and the enumerated allowlist are now derived from the table's own rows and diffed against `infrafactory.yaml` by `TestLayer3CoverageDoc*` — the project's drift-becomes-a-failed-test pattern applied to prose.

- 🔒 **S149 landed (2026-08-24)** — README section for a stranger arriving after the talk: what Layer 3 proves, **what it does not**, and why only Scaleway. The negative list is the load-bearing one — the pattern has never run on the other three clouds, 3 of 17 Scaleway scenarios have ever touched the real API, and a green run means the cloud accepted the change and it cleaned up, not that the change is good. Caught in review: the section described the label-triggered PR gate as though it existed, and it is still on the unmerged S144 branch — a public safety claim readers could not verify. Now qualified, and `TestReadmeLayer3GateClaimMatchesWorkflows` fails in *both* directions, so the claim and `.github/workflows/layer3-gate.yml` rise and fall together.

- 🚧 **Active arc**: `docs/plans/presentable-arc-plan.md` (S144–S150) — making infrafactory demonstrable on stage for the *"Letting Agents Change Production Without Breaking It"* talk, 2–6 weeks out. Scoped by what the talk must be able to **show**. The spine is S144, a label-triggered PR gate that applies to real Scaleway, destroys, sweeps, and comments the stage summary back — which is what turns "before it ever reaches a production repo" from aspiration into an artifact. S145 builds the evidence that `plan` passes where the real cloud refuses; S150 closes the CI-hardening delta found against `redscaresu/goldfinger` (govulncheck, explicit `permissions:` blocks, SHA-pinned actions).

- 🎯 **Baseline: 44/44 deterministic** + **fakegenesys v0.2.1** + **27 paired contracts across the family** (CI-enforced via `handlers/contract_audit_test.go` in all 4 siblings) + **ADR-0021 cloud-prefix lockstep CI-enforced**.
- ✅ **Last arc complete**: `docs/plans/layer3-real-scaleway-plan.md` (S139–S143) — **Layer 3 now applies to and destroys real Scaleway infrastructure.** Code-complete since S30, it had never called `api.scaleway.com`; it does now. Both canary runs are green end-to-end (`preflight → init → plan → apply → destroy → orphan_sweep`), run 1 with seeded HCL to isolate the harness and run 2 with the **LLM generating the HCL** — `target_reached` in one iteration. Sweeps clean and independently confirmed against the account; it ends in the same state it started. Guardrails: sealed sandbox env + fail-closed endpoint assertion, mockway `account/v3` projects with non-empty-delete, real-API orphan sweep with a state-derived stray check, `AssertProjectDeletable`, interrupt-safe destroy + `infrafactory reap`, a disposable fallback project, and a deny-by-default resource-type allowlist — each with synthetic-drift coverage.

The canary found three defects nothing else could. Run 1: the orphan sweep read `terraform-live.tfstate` *after* destroy emptied it, and mockway's seeded default project counted as a Layer 2 orphan — which would have failed `no_orphans` for **every scenario in the suite** (both fixed in S143a). Run 2: real Scaleway returned a block-volume create error *after* the volume existed, leaving it tainted; transient, and a re-apply succeeded. Diagnosing it was hard because Layer 3 reported only `exit status 1` — both sandbox paths discarded the provider's message. Fixed via the shared `stderrFailureDetail` helper; ADR-0023 amended (fail-closed is necessary but not sufficient).

Deltas in `docs/layer3-real-vs-mock-deltas.md`. Close-out in `docs/status/ARCHIVE.md`.

**Layer 3 follow-ups now closed** (post-arc). A failed run no longer just destroys and hopes: it captures the sweep target before destroy and sweeps afterwards, and any failure raised while real resources may still exist carries `infrafactory reap <scenario>` in its detail, where the operator actually reads it. Apply also retries once — bounded, surfaced as `succeeded on attempt 2`, and never on a cancelled context, so the interrupt guard still holds.

Codex review loop now runs on every PR (2 consecutive clean passes to converge; passes recorded in `docs/review-passes/`). Pass 1 on the arc caught one real defect: the `infrafactory reap` recovery hint was not shell-quoted, so a scenario path containing a space produced a command that would not run.

**Dependency pipeline unblocked** (2026-08-23). Doc Hygiene required a `STATUS.md` edit for any `go.mod`/`go.sum` change — which dependabot cannot make — so with required status checks on `main`, **every Go dependency PR was permanently unmergeable**; eight had piled up, the oldest three weeks old. Dependency-manifest-only changes are now exempt, all-or-nothing: a PR that bumps `go.mod` *and* touches `internal/` still needs `STATUS.md`. `scripts/check_doc_hygiene_test.sh` pins both halves and runs in CI.

**Probe canary run (2026-08-23)**: `lb-paris` gained a `connectivity` criterion and ran against real Scaleway. The real-probe path works — it resolved the LB's public IP from `terraform-live.tfstate` and opened a TCP connection to it, the first time that code has run against anything but a mock. It also found a **real leak**: `test` only cleaned up after a *successful* sandbox apply, so a partial apply (killed by an IAM permission error) left a real project and LB IP billing until reaped by hand. Fixed — cleanup is now gated on the sandbox layer having run, not having worked. Deltas in `docs/layer3-real-vs-mock-deltas.md` (D4).

**Superseded next-arc opener**: `lb-paris` as probe canary — but note it currently declares **no probes** (only `region_restriction` + `no_orphans`), so the arc needs probe criteria added to the scenario first; it is not just a run. The five scenarios that do declare probes (`compute-lb-multi-paris`, `mysql-ha-paris`, `redis-xlarge-session-paris`, `web-app-paris`, `gcp-cloud-sql`) are all substantially more expensive. Costs real money either way, so scope it deliberately.

## Recent arcs

| Arc | Outcome | PRs |
|---|---|---|
| S89–S93 (2026-06-03) | 🎯 39/39 first deterministic; fakeaws Secrets Manager soft-delete fix; AWS phase2 audit (10/10 Cat C). Option C scaffold shape adopted. | fakeaws#6, #69, #70 |
| S84–S88 (2026-06-03) | gcp-full-stack convergence (servicenetworking escape pitfall); `scripts/sweep_39.sh` panic gate. | #64, #65, #66 |
| S79–S83 (2026-06-02) | Sibling-mock drainage + N3 carve-out validation; `cmd/s3router/` shim added. | fakeaws#5, #58–#61 |
| S74–S78 (2026-06-02) | AWS + Scaleway phase3 collapse; `make sweep-39`; N3 GCP-escape carve-out. | 5 PRs |
| S54–S73 | GCP phase2 collapse, sustain ratchets, N3/N10/N13 architecture build-out. | ~22 PRs across 4 sub-arcs |

Full per-arc close-outs in `docs/status/ARCHIVE.md`.

## OSS-readiness

All four repos (`infrafactory`, `mockway`, `fakegcp`, `fakeaws`) ship Apache-2.0 + SECURITY.md + CODE_OF_CONDUCT.md + CONTRIBUTING.md + CHANGELOG.md + release workflow. Pre-commit hook (`gitleaks` + `go test`) installable via `make install-hooks`. Full-history `gitleaks detect` zero leaks.

**User-only click-ops pending** (private → public visibility flip + branch protection on each repo).

## Known blockers

None.

## Open tickets

None — `docs/tickets/rename-learning-system.md` closed by S104.

## Update policy

- Trim this file every arc close-out — historical detail belongs in `docs/status/ARCHIVE.md`.
- Goal: ≤ 30 lines. If it grows past 50, time to trim again.
- ADRs and `CONCEPT.md` carry durable architecture decisions; STATUS.md is just the current-shape pointer.
