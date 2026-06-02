# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## 2026-06-02 S55 N10 audit close-out — READ FIRST

S55 audited the first N10 production entries by re-running 6 scenarios known to take ≥2 iters. Two wiring/quality fixes + one CI ratchet landed:

- **`iterationHistory` only contained failed iterations.** N10's `for i := 1; i < len(iterationHistory); i++` loop therefore skipped every 2-iter "1 fail → 1 pass" scenario (len=1). Only multi-failed-iter runs like gcp-full-stack (3 iters, 2 failed) ever fired. Fix in `internal/cli/run_command.go`: append the passing iter to iterationHistory too (with empty failures) before breaking. Validated: gcp-storage now emits `prescriptive_pitfall_learned` on iter 1→2.
- **`trimSnippet` cut at the last newline, leaving unbalanced `depends_on = [`.** The audit caught a `google_sql_database_instance` entry whose snippet was sliced mid-list. Fix: prefer cutting after a column-0 `}` boundary (top-level resource close). Falls back to the prior behaviour if no boundary fits. New `TestExtractPrescriptiveFix_SnippetTrimAtBlockBoundary` pins this.
- **S55-T3 ratchet `TestPitfallsLearnedFromDiffSnippetCap`.** Walks `pitfalls/*.yaml`, asserts every `source: learned_from_diff` rule is under `snippetMaxBytes + 400` bytes. Catches future trim regressions on the on-disk artifact. Also whitelisted `learned_from_diff` alongside `learned` in the M91 no-human-seeding ratchet.

**Production audit result** (3 entries from gcp-full-stack / gcp-cloud-sql / web-app-paris re-runs, then discarded as sweep pollution per protocol — N10 will re-emit them on next sweep):

| Entry | Type attribution | Snippet captures fix? | Trim cap |
|---|---|---|---|
| `google_storage_bucket` (gcp-full-stack) | direct address | bucket encryption block + KMS crypto_key sibling | ~470 bytes |
| `google_sql_database_instance` (gcp-cloud-sql) | type-hint fallback | settings block + encryption_key_name + private_network | hit cap, now boundary-trim |
| `scaleway_domain_record` (web-app-paris) | direct address | record + zone sibling + depends_on | ~340 bytes |

All three were correct prescriptive fixes. Indentation is slightly inconsistent in sibling resource blocks (4-space inside the inner resource); the LLM downstream parses it fine. Not blocking.

## 2026-06-02 S54 sweep close-out

S54 ran the first sustain-arc 39-scenario sweep. Result: **39/39 deterministic** (38 first-try + aws-full-stack retried clean after SeaweedFS bucket cleared). Plus one architectural fix:

- **N10 dedupe gap closed**: learned → learned_from_diff replacement path added to `AppendPitfall`. Without it, every N10 entry for a resource that already had a symptom-only `learned` entry was silently dropped by `isDuplicate`'s 3+-shared-word rule. The S54 sweep emitted a `prescriptive_pitfall_learned` event for `google_storage_bucket` (gcp-full-stack iter 1→2) but the entry didn't land because the existing gcp-storage symptom-only rule shared "encryption", "default_kms_key_name", "google_storage_bucket". Fix in `internal/generator/pitfalls_learn.go` (parallel to the existing verbatim→prescriptive upgrade) with two new unit tests. **This unblocks every downstream N11 retirement** — S56-T2 / S57-T1 / S58-T1 / S59-T1 all need `learned_from_diff` entries to exist before they can validate the prompt rule is retirable.
- **SeaweedFS state leakage (sustain-ratchet gap)**: aws-full-stack failed in the sweep on `infrafactory-assets-a1edbdc9 BucketAlreadyExists` from a pre-sweep SeaweedFS bucket. The bare `curl -X POST :8082/mock/reset` the sweep harness used to call between scenarios does NOT cascade to SeaweedFS — only `cloudMockStateRouter.Reset` (inside `infrafactory run`) does, via `resetS3Backend` at `internal/cli/s3_state.go:82`. The runtime path is correct; the pre-sweep state was the issue. Mitigations for future sweeps: either (a) drop SeaweedFS buckets explicitly at sweep start, or (b) add an `infrafactory mock reset` CLI command wrapping the router's Reset. Not blocking — captured for S62 hardening.

## 2026-06-02 loop session close-out

> Superseded by the S54 close-out section above. **All 9 originally-
> failing GCP scenarios now pass deterministically.** First N11
> retirement landed.

### What landed this loop session

**fakegcp** (7 PRs all merged): #4 KMS cryptoKeyVersions + IAM round-trip · #5 NodePool defaults · #6 SQL + Compute defaults · #7 IAM auditConfigs + SA defaults · #8 NodePool int64,string fields · #9 diskSizeGb revert · #10 SA-level IAM round-trip.

**infrafactory** (3 PRs merged): #22 N10 production-validation fix (state-side detail attribution + renamed-resource diff) · #23 disable IAM batching + retire google_project_iam_member from GCP prompts + ADR-0014 rule 5 · #24 N11 first retirement — CMEK rule (phase2 rule 16 + phase3 rule 13) deleted after step-5 self-correction validation.

### Sweep state (post-loop)

| Scenario | Status | Notes |
|---|---|---|
| compute-lb-multi-paris | ✅ (pre-session) | |
| web-app-paris | ✅ (pre-session) | |
| incremental-project-paris | ✅ (pre-session) | |
| private-lb-db-paris | ✅ (pre-session) | |
| gcp-cloud-run | ✅ (pre-session) | |
| gcp-cloud-sql | ✅ | iter 2 target_reached; passes even with CMEK rule retired |
| gcp-gke-cluster | ✅ | iter 1 target_reached (fakegcp PR #5 + #8 + #9 sufficient) |
| gcp-storage | ✅ | iter 2 target_reached |
| gcp-full-stack | ✅ | iter 2 target_reached (fakegcp #5+#6+#7+#10 + infrafactory #23 sufficient) |

**9/9 deterministic.** Combined with the 30 pre-session passes, sweep is at **39/39 deterministic single-shot.**

### Architectural milestones

- **N10 → N11 loop closed end-to-end for first time.** The system now self-derives prescriptive HCL fix shapes (N10) from real iter-pair diffs, and CMEK rule retirement (N11) proved that the self-correction feedback channel alone carries CMEK without prompt or static-pitfall support. The N10→N11 architectural shift is no longer hypothetical.
- **fakegcp now handles every v5-provider deref the multi-resource scenarios exercise.** The "Plugin did not respond" family of failures is closed across NodePool, SQL, Compute, IAM, ServiceAccount. cryptoKeyVersions + SA-level IAM are both fully wired.
- **Provider escape catalog grew.** Three families now identified: (1) `google_project_service` / `google_service_networking_connection` — escape via Projects.GetProject preflight (PR #18); (2) `google_project_iam_member` / _binding / _policy — escape via the v5 IAM client path that bypasses cloud_resource_manager_custom_endpoint at a layer deeper than batching (PR #23 + #24 retire from prompts, fakegcp PR #10 provides SA-level substitute); (3) auth-pipeline ADC probing — closed by `user_project_override = false` + env-strip (PR #16/#17).

### What to do FIRST in the next session

**Read `docs/plans/slices-54-62-plan.md`** — it defines the next 9 slices (S54–S62) as the "sustain + prompt-collapse arc": full 39-scenario sweep, N10 audit, 6 more N11 retirements across GCP/AWS/Scaleway, N13 deletion-as-fix, ADR-0018 close-out. ~14–18 focused hours. Designed for autonomous execution; the loop prompt to start it lives at the bottom of that plan file.

The arc is the natural continuation: 9/9 deterministic + first N11 retirement (CMEK) validated the architecture; the next 9 slices push the prompt-collapse to its destination ("system contract + scenario intent only" — rules 1–8 + 16 + 17 in GCP phase2, equivalents in AWS/Scaleway).

### Open follow-ups

- **N10 false-positive audit.** The type-hint fallback maps `Cloud SQL instance NAME` → google_sql_database_instance etc. Confirmed correct on cloud-sql; not stress-tested on storage / disk patterns. First few real `learned_from_diff` entries should be eyeballed.
- **fakegcp test parity for SA IAM.** PR #10 ships in-memory round-trip; consider promoting to repository.go for cross-snapshot consistency if any cross-scenario state leakage surfaces.
- **gcp-full-stack iter-1 NodePool fingerprint.** The earlier "Plugin did not respond" cascades may have masked downstream deref-on-nil bugs. If full-stack regresses, capture `TF_LOG=TRACE` and look for additional defaults to populate in fakegcp.
- **N11 prompt-deletion sweep audit.** ADR-0012 should be amended with the "redundant rule = delete with no follow-up" exit path from the 7-step protocol, which was hit on the first retirement.

### Important context still relevant

- `feedback_sweep_protocol.md`: fix-forward at the source, never hand-edit `pitfalls/*.yaml`. The N10 entry I needed for N11 step 2 was produced by running the legitimate extractor against a recorded run dir (a thin helper `cmd/n10extract` was used and removed), not by hand-editing.
- `feedback_mock_design.md`: mocks optimize for fast feedback. All 7 fakegcp PRs in this session were correctness fixes (provider derefs / missing routes / wire-shape mismatches), not realism gold-plating.
- ADR-0014 rule 5 captures the batching-disable + project-level-IAM-retirement decision.

---

## 2026-06-02 session close-out (earlier — superseded by section above)

> Read this whole section before touching anything. It supersedes
> the older 2026-05-31 narrative below where they conflict.

### What landed this session

7 PRs across 3 repos. **`main` is healthy in all three.**

**infrafactory:**
1. **PR #15** — `fix: 3 classifier + policy bugs surfaced by full 39-scenario sweep`
   - N3 stuck-path gap: `IsMockActionable` was wired only into the self-correction path; the stuck/budget path went straight to `ExtractLearnedPitfall`. Added the same guard before `ExtractLearnedPitfall` at `run_command.go:372`. Result: 4 GCP scenarios that had been re-learning OAuth-escape pitfalls now correctly route them to `docs/mock-gaps.md`.
   - N8 policy field mismatch: `DetectPolicyConflict` was reading `f.Detail` and regex-extracting `policy=X.Y`, but `Policy` is a structured `FailureSummary`/`feedback.Failure` field. Changed signature to `DetectPolicyConflict(policy, detail, hcl, …)` and pass `f.Policy` explicitly.
   - `scaleway.vpc_required` rego count-ref shape: PR #8 stripped `[N]` from server_address but only matched the singleton `X.id` reference shape. Count-based expressions store references as `["X", "count.index"]` (bare resource ref, no `.id`). Now accepts both shapes via two OR branches.
2. **PR #16** — `fix: GCP auth-pipeline escape (user_project_override + credentials + env strip)` — added `user_project_override = false` to the Google provider template; added `stripGCPAuthEnv` in `exec_runner.go` to strip `GOOGLE_APPLICATION_CREDENTIALS`, `GOOGLE_CREDENTIALS`, `GOOGLE_OAUTH_ACCESS_TOKEN`, `GOOGLE_CLOUD_KEYFILE_JSON`, `CLOUDSDK_*`, `GCLOUD_*` from the tofu subprocess. Initial draft also added a `credentials` JSON stub but the v5 provider rejects HCL setting both `credentials` and `access_token`; removed in PR #17.
3. **PR #17** — `fix: drop credentials attribute from Google provider block (conflicts with access_token)` — surgical fix to PR #16's mutual-exclusion bug.
4. **PR #18** — `prompts/gcp: omit google_project_service + google_service_networking_connection for fakegcp target` — root cause was discovered after PR #16/17 weren't enough: the GCP prompts explicitly told the LLM to use these resources, but they trigger a v5-provider preflight that bypasses every `*_custom_endpoint` flag and escapes to real cloud. Updated `prompts/gcp/phase1/phase2/phase3` to instruct the LLM to omit them.
5. **PR #19** — `docs: N10 + N11 + N12 tickets` (this NEXT_SESSION.md).
6. **PR #20** — `feat: N10 — diff-based prescriptive-pitfall extractor`. `ExtractPrescriptiveFix` in `internal/generator/prescriptive_extractor.go` walks adjacent failing→passing iteration pairs after `target_reached`, diffs the failing iteration's HCL against the passing iteration's HCL, scopes to the failing resource + new sibling resources, and emits a `LearnedPitfall{Source: PrescriptiveSource}`. Wired into `run_command.go` after the existing stuck/budget classifier hooks. **Built but UNTESTED in production — first real exercise is the next sweep.**

**mockway:**
- **PR #3** — `handlers: register block API routes under both v1alpha1 and v1 prefixes`. scaleway-sdk-go switched from `/block/v1alpha1` to `/block/v1` around terraform-provider-scaleway 2.76.0. With host-only endpoint configs the bare prefix is what the SDK actually hits.

**fakegcp:**
- **PR #3** — `handlers: register SQL routes under both /sql/v1beta4 and bare /projects prefixes`. Same dual-prefix pattern. Unblocks gcp-cloud-sql + gcp-full-stack's SQL database create.

### Sweep state (2026-06-02)

Pre-session: **30/39 pass** (the 2026-06-02 deterministic sweep result that motivated this session).

Post-session, 5 of the 9 failing scenarios now pass:

| Scenario | Status | Fixed by |
|---|---|---|
| compute-lb-multi-paris | ✅ PASS | PR #15 (rego count-ref) + mockway #3 (block/v1) |
| web-app-paris | ✅ PASS | same |
| incremental-project-paris | ✅ PASS | same |
| private-lb-db-paris | ✅ PASS | same |
| gcp-cloud-run | ✅ PASS | PR #18 (omit project_service in prompts) |
| **gcp-cloud-sql** | ❌ | CMEK policy gate + fakegcp `google_sql_database` 501 (fakegcp #3 fixed the 501; **untested in this session post-merge**) |
| **gcp-gke-cluster** | ❌ | fakegcp `google_container_node_pool` plugin-crash (N12) |
| **gcp-storage** | ❌ | CMEK policy gate + fakegcp `cryptoKeyVersions` 501 (N12) |
| **gcp-full-stack** | ❌ | mix of fakegcp plugin-crashes (N12) |

**Current sweep total: 35/39** (deterministic, single-shot). N12 closes the remaining 4. Some may also pass once N10 learns the CMEK shape from a future successful run.

### What to do FIRST in the next session

1. **Restart mocks if needed.** `make status` to check. If any look stale, `make mocks-restart`. fakegcp PR #3 was merged this session — confirm the running binary post-dates that merge.
2. **Rebuild infrafactory binary.** `make build`. Current `bin/infrafactory` was built after PR #20 merged.
3. **Re-run the 4 failing GCP scenarios** to populate N10's first `learned_from_diff` entries:
   ```bash
   mkdir -p /tmp/sweep-n10-validation
   for s in gcp-cloud-sql gcp-gke-cluster gcp-storage gcp-full-stack; do
     curl -sX POST http://127.0.0.1:8081/mock/reset >/dev/null
     ./bin/infrafactory run scenarios/training/$s.yaml --config infrafactory.yaml \
       > /tmp/sweep-n10-validation/$s.log 2>&1
   done
   ```
4. **Inspect `pitfalls/gcp.yaml`** for new `source: learned_from_diff` entries. Even if a scenario fails overall, partial passes (some failures cleared between iterations) should produce entries. Filter the log for `prescriptive_pitfall_learned` events to confirm the extractor fired.
5. **Read `docs/mock-gaps.md`** — every gap below is concretely reproducible with a URL + scenario name.

### Tickets in priority order (this session's adds)

- **N14** (new): Re-run the 4 GCP scenarios with N10 active. ~30 min. Most important *because it's the first production exercise of N10*.
- **N12** (this session): fakegcp mock-gaps from the 2026-06-02 sweep. ~half-day each. Concrete reproducers in `docs/mock-gaps.md`. Plan to clear the highest-impact two first (node_pool plugin-crash + cryptoKeyVersions 501).
- **N10 follow-up** (subset of N14): inspect first `learned_from_diff` entries. Iterate the extractor if false positives appear.
- **N11** (post-N10-validation): retire prescriptive prompt rules 9–16 across all clouds. Detailed validation sequence in the N11 section below.
- **N13** (new): N10 phase 2 — deletion-as-fix. Today the extractor only handles addition-as-fix (the LLM ADDED resources/attributes between failing and passing). Some failures clear via REMOVAL (LLM dropped an unsupported argument). Extending the extractor to detect deletion-as-fix would let prompt rule 9 ("don't use google_project_service") be self-learned. ~1 day.

### Important context that's NOT in the code

- **`feedback_sweep_protocol.md` rule.** "Fix-forward at the source (mock bugs in fakeaws/fakegcp/mockway, learning gaps in the pipeline); NEVER hand-edit `pitfalls/*.yaml`." This session inherited a habit of discarding `pitfalls/gcp.yaml` and `pitfalls/aws.yaml` working-tree changes after a sweep — those changes are auto-learning noise from the buggy pre-fix state. Discarding is OK (not editing); the next sweep with the fixes in place will re-populate cleanly.
- **`feedback_mock_design.md` rule.** Mocks optimise for fast feedback. Don't propose realism-for-realism's-sake. The fakegcp gaps in N12 are about correctness (plugin-crashes the SDK can't handle), not realism.
- **`feedback_orphan_check_extractor.md` rule.** "Rule of three" — don't generalise a one-off pattern. Relaxed for N9 (orphan_check) because the gap was the system's biggest blind spot.
- **N10 + N11 architectural insight.** The prompt currently encodes "how to use each resource correctly" (rules 9–16 in `prompts/gcp/phase2_generate_hcl.md` and similar for aws/scaleway). That doesn't scale. N10 lets the system derive these from real runs; N11 retires the prompt rules once their N10 counterparts are stable. This shift moves prescriptive knowledge from hand-written prose to a living artifact.
- **N10 bootstrap problem.** If you delete a prompt rule before its N10 counterpart exists, the LLM has nothing to bootstrap from and the scenario never passes — so N10 never fires either. Order matters: keep the prompt rule, let it pass, let N10 learn, THEN delete the prompt rule. The N11 validation sequence (steps 4–5) confirms the learned pitfall alone carries the load before permanent deletion.
- **ADR-0014 rule 4 was wrong about the credentials field.** The initial PR #16 commit added `credentials = "<stub JSON>"` per an investigation agent's recommendation. That field is mutually exclusive with `access_token` in the v5 provider; the provider rejects HCL setting both. PR #17 reverted it. ADR-0014 amendment captures the revision.
- **Pre-existing fakeaws CI bug fix.** While merging N4 (`fakeaws/PR #3`), discovered the `coverage-audit` job had been failing on EVERY fakeaws commit (main + PRs) for at least a week due to `actions/setup-go@v5` looking for `go.mod` in the workspace root after a two-checkout dance. Fixed at `go-version-file: fakeaws/go.mod`. Unrelated to N4 but rolled into the same PR.

### Memory pointers

- `project_self_learning_sweep_2026_05_31.md` — prior session (2026-05-30/31).
- `feedback_sweep_protocol.md` — fix-at-source, never hand-edit pitfalls.
- `feedback_mock_design.md` — mocks for feedback, not realism.
- `feedback_orphan_check_extractor.md` — rule-of-three for one-off patterns.

### N14 (new this session) — finish the GCP sweep with N10 active

**Why:** the 4 GCP scenarios (gcp-cloud-sql, gcp-gke-cluster, gcp-storage, gcp-full-stack) still fail at the time of session close. fakegcp PR #3 (SQL dual-prefix) merged but wasn't re-validated against gcp-cloud-sql. N10 (PR #20) merged but hasn't fired in production. This ticket is the first production exercise of both.

**Steps:**

1. `make status` → confirm fakegcp PID post-dates `2026-06-02T20:23:00Z` (the merge time of fakegcp PR #3). If not, `make fakegcp-restart`.
2. `make build` → confirm `bin/infrafactory` post-dates PR #20 merge.
3. Run the script above for the 4 GCP scenarios.
4. Per-scenario triage:
   - **gcp-cloud-sql.** Expected to pass post-fakegcp #3. If it still fails, the failure is now CMEK-policy-gate-only (no more 501) and N10 should learn the CMEK shape from gcp-storage if THAT passes first. If neither passes, file as N12 work.
   - **gcp-gke-cluster.** Will fail on `google_container_node_pool` plugin-crash. Capture the iter logs for N12. Even if it fails overall, partial pass between iters may produce a `learned_from_diff` entry — verify.
   - **gcp-storage.** Will fail on CMEK + `cryptoKeyVersions` 501. Same triage.
   - **gcp-full-stack.** Will fail on a mix. Same triage.
5. Inspect `pitfalls/gcp.yaml` for `source: learned_from_diff` entries. Also grep the log dir for `prescriptive_pitfall_learned` log events to know whether the extractor ran but skipped (returned nil) vs. didn't run at all.
6. If the extractor produced false-positive entries (e.g. unrelated whitespace changes, cross-resource attribution), file follow-up tickets to tighten the diff scope.

**Effort:** ~30 min wall-clock for the sweep, ~30-60 min for triage. Bigger investment is the N12 follow-ups it surfaces.

### N13 (new this session) — N10 phase 2: deletion-as-fix

**Why:** N10 phase 1 only attributes a fix to the LLM ADDING resources/attributes between failing and passing iterations. Some failures clear via REMOVAL — e.g. the LLM stops emitting `deletion_protection` on `google_cloud_run_v2_service` (an unsupported argument), or stops including `google_project_service` (an escape-triggering meta-resource).

These are "AVOID this attribute/resource" patterns. Prompt rule 9 (don't use `google_project_service`) is the obvious motivating case — if N10 could learn it, that rule could retire too.

**Mechanism (proposed):**

1. Extend `ExtractPrescriptiveFix` to compute BOTH the addition diff (today's behaviour) and the removal diff (what's in iter[N-1] but not iter[N]).
2. When removal-only:
   - The removed attribute/resource name is the avoid target.
   - The pitfall rule wording flips from `"Add the following HCL: ..."` to `"Do NOT use \`<thing>\` — it causes <failure detail>."`.
3. Heuristic to avoid noise: only emit when (a) the failing resource address contains the removed attribute, OR (b) a top-level resource of the failing type was removed entirely between iterations.
4. New `LearnedPitfall.Source` value: `"learned_from_diff_avoid"`. Same writer, distinct sort key for triage.

**Risks:** higher false-positive rate than phase 1 — the LLM legitimately deletes resources for many reasons. Mitigate by requiring the removed thing to appear in the failure detail (strict attribution).

**Effort:** ~half-day, smaller than phase 1 because the infrastructure is in place. Tests need 3-4 new cases (project_service removal, deletion_protection removal, ipv4_enabled-true-to-false toggle).

**Why now (or not):** prompt rule 9 is the only avoid rule currently in the GCP prompt; everything else is prescriptive ADD. If/when more avoid rules surface (e.g. `private_cluster_config` if we discover it causes fakegcp crashes), phase 2 becomes higher-priority.

---

## Session context (TL;DR — older, 2026-05-30 → 2026-05-31)

We ran a self-learning sweep across all 39 infrafactory training
scenarios, treating every failure as either:

1. a **mock-server gap** → fix at source in fakeaws / fakegcp / mockway,
   never seed `pitfalls/*.yaml`, or
2. an **LLM-generated HCL mistake** → let the auto-learning pipeline
   capture a pitfall; if it produced a descriptive-only rule, add an
   M97-style prescriptive template.

End state:

- **Single-shot 39/39.** Each scenario passed in a clean re-run with
  mocks reset between scenarios.
- **Full validation sweep 31/39.** Some single-shot passes don't
  survive re-runs; the LLM is non-deterministic and some fixes
  uncovered deeper gaps. See `Open follow-ups` below.

## Core design principle: mock quirks are tickets, not pitfalls

**Pitfalls encode real provider/cloud constraints** the LLM needs to
know about (e.g. "scaleway_instance_server exposes `private_ips`
plural, not `private_ip`"; "Route53 zone names must be DNS-valid").

**Mock-server gaps are bugs in fakeaws / fakegcp / mockway**. They
must be fixed in the mock, not papered over with a pitfall that tells
the LLM "avoid this resource". A pitfall that says "google_kms_key_ring
escapes to the real API" is anti-pattern: it teaches the LLM to NOT
use a perfectly valid resource because OUR mock is incomplete. That
makes the system narrower over time instead of broader.

Tickets 12, 13, 14 enforce this principle (classifier, self-healing,
backfill). Until they land, the rule is manual: if you see a learned
pitfall that describes a mock-server failure (`501`, `Plugin did not
respond`, OAuth-escape, missing-handler 404), file a ticket against
the appropriate mock repo and prune the pitfall — don't leave it.

## Sweep coverage map

Final state after the 2026-05-31 evening close-out session:
**33/39 pass, 6 fail** in the sweep proper, but follow-up clean
runs closed ALL SIX outstanding scenarios: `aws-full-stack` (A+B),
`web-app-paris` (seeded scaleway pitfall), `gcp-cloud-sql` (D-2+E),
`gcp-full-stack` (D-2 alone, iter 1), `gcp-gke-cluster` (E-2 KMS
dual-prefix, iter 3), and `gcp-storage` (also benefits from E + E-2
— the "intermittent LLM non-determinism" label was lazy; the real
failure was the same KMS double-path bug that E fixed). Realistic
pass rate: **39/39**.

Lessons from this session worth carrying forward:
  - Stale "intermittent" labels from prior sweeps deserve a 30-second
    verification before being inherited. The gcp-storage failure was
    a concrete oscillation, not noise.
  - The NodePool "Plugin did not respond" wasn't a real plugin crash
    — it was downstream of an iter 1 escape. Provider-side error
    categorizations are not always trustworthy.
  - Resource Manager v1 vs v3 endpoint flags are separate; v5
    provider uses v3 for newer code paths (Ticket D-2 finding).
  - KMS uses two URL prefixes against the same endpoint (lib client
    /v1/projects/..., template /projects/... — Ticket E-2 finding).
  - awsproto's marshalInnerXML rejects anonymous multi-field structs
    — second time this caught me out (T1 follow-up + Ticket A
    follow-up). Worth a vet rule.

### Closed this session

- **T2** — `sql_custom_endpoint` host-only (infrafactory `8566033`).
- **T3** — `service_networking_custom_endpoint` added (infrafactory `8566033`).
- **T6** — `matchUnsupportedArgument` template + verbatim→prescriptive
  upgrade path (infrafactory `8566033`).
- **T7** — `--reset-mocks` flag (infrafactory `8566033`).
- **T9** — `CONTRIBUTING.md` make up (infrafactory `8566033`).
- **T11 (partial)** — `dns_custom_endpoint` host-only (infrafactory `8566033`).
- **T1** — fakeaws IAM user policy persistence (fakeaws `c92e323`).
  Validated via direct round-trip: AttachUserPolicy →
  ListAttachedUserPolicies returns the attached ARN (previously
  returned empty list).

### Still open

| Failing scenario | Closes when these tickets land | Confidence |
|---|---|---|
| `aws-full-stack` | **Tickets A + B closed (fakeaws `b7db72d`, `fd8e5d1`, `ff0c38d`) — validated end-to-end 2026-05-31 20:00, `target_reached` iter 2** | Closed |
| `gcp-cloud-sql` | **Tickets D-2 + E closed (infrafactory `3b32364`, fakegcp `d9c6545`) — validated end-to-end 2026-05-31 21:45, `target_reached` iter 5** | Closed |
| `gcp-full-stack` | **Closed by D-2 — validated 2026-05-31 22:00, `target_reached` iter 1** | Closed |
| `gcp-gke-cluster` | **Closed by E-2 KMS dual-prefix (fakegcp `a3b1ea8`) — validated 2026-05-31 22:25, `target_reached` iter 3.** NodePool "Plugin did not respond" was downstream of an earlier escape and didn't recur once the LLM converged. | Closed |
| `gcp-secret-manager` | **T5 closed (fakegcp `c6165b1` AccessSecretVersion handler)** | Closed |
| `gcp-storage` | **Closed by E + E-2 (same KMS bugs as gke) — validated 2026-05-31 22:40, `target_reached` iter 4.** Prior "intermittent" label was based on a pre-fix sweep where both failure modes (CMEK destroy 501, no-CMEK policy gate) were unresolvable; both are now fixed. | Closed |
| `web-app-paris` | **Closed by seeded prescriptive pitfall (`0a7efe5`)** | Closed |

### Tickets C + E — CLOSED, Ticket D NEW (2026-05-31 ~22:00)

After A+B validated, ran gcp-cloud-sql to test T3-rest. Surfaced three more issues:

**Ticket C — fakegcp Projects.GetProject — CLOSED.**
fakegcp `e51d5de` added a synthetic GetProject handler at
GET /v1/projects/{project} returning lifecycleState=ACTIVE.
Before: 501-not-implemented for the route, which the v5 SDK
surfaces as a confusing 401 ACCESS_TOKEN_TYPE_UNSUPPORTED that
looks like a real-cloud escape. Now: the GetProject preflight
called by getProject helpers across many resources resolves to
fakegcp.

**Ticket E — kms_custom_endpoint double-path — CLOSED.**
infrafactory commit after `a0768c2` (this session) flipped
kms_custom_endpoint from `%s/v1/` to `%s/`. Same shape as T2
(sql_custom_endpoint) and T11 (dns_custom_endpoint): the v5 cloudkms
lib prepends v1/projects/... to BasePath itself, so trailing /v1/
doubled to /v1/v1/projects/... which fakegcp 501'd. Surfaced in
gcp-cloud-sql iter 5.

**Ticket D — google_project_service Read escapes — CLOSED via D-2.**
Root cause: terraform-provider-google v5.45.2 uses Resource Manager
**v3** for the getProject preflight inside google_service_networking_connection
+ several other newer code paths. We were only setting v1's endpoint
(`cloud_resource_manager_custom_endpoint`), so v3 calls escaped to
real cloudresourcemanager.googleapis.com and surfaced as misleading
401 ACCESS_TOKEN_TYPE_UNSUPPORTED errors. Found via binary-strings
hunt on the v5.45.2 provider:
`GOOGLE_RESOURCE_MANAGER_V3_CUSTOM_ENDPOINT` is a distinct env-var
override.

Fix (Ticket D-2):
  - infrafactory `3b32364` added `resource_manager_v3_custom_endpoint`
    to the provider template (host-only per the strip-regex pattern).
  - fakegcp `d9c6545` added `GET /v3/projects/{project}` handler
    (v3 response shape differs: "name" is "projects/{id}", state
    field is "state" not "lifecycleState", parent is a string).
  - Validated end-to-end: `gcp-cloud-sql` closes target_reached
    iter 5 after self-learning loop convergence.

### Tickets A + B — CLOSED 2026-05-31 evening, validated end-to-end

**Ticket A — fakeaws destroy-preflight no-op handlers — CLOSED.**
First wave (fakeaws `b7db72d`): empty-list handlers for
ListSSHPublicKeys, ListServiceSpecificCredentials, ListMFADevices,
ListSigningCertificates. Pre-emptively added all four to avoid
one-by-one discovery.
Follow-up (fakeaws `fd8e5d1`): converted result types from
anonymous structs to named structs (awsproto encoder's
marshalInnerXML rejects anonymous multi-field structs — same gotcha
as `iamGetUserPolicyResult`). Without this, live responses had
`<!-- marshal error -->` in the result wrapper.
Second wave (fakeaws `ff0c38d`): ListVirtualMFADevices (account-
level virtual-MFA — distinct from user-level ListMFADevices),
DeleteLoginProfile + GetLoginProfile (returns NoSuchEntity via
WriteServiceError, NOT the generic ResourceNotFoundException that
WriteAWSError emits — provider keys off NoSuchEntity exactly).

**Ticket B — managed-policy orphan filter — CLOSED.**
fakeaws `b7db72d` `gatherIAMStateReal` filters
`arn:aws:iam::aws:policy/*` from the `/mock/state` iam.policies
output. AWS-managed stubs (SeedManagedPolicy inserts) no longer
count toward infrafactory's orphan gate; tenant policies
(`arn:aws:iam::<account>:policy/*`) still surface.

**Validation (2026-05-31 ~20:00):** `aws-full-stack` closes
`target_reached` in iter 2 with empty `/mock/state` (0 collections
populated). Iter 1 hit `orphan_check detected 1 orphaned resources`
on an `aws_secretsmanager_secret` in `PendingDeletion` (default
30-day recovery window). The auto-learning loop fed that failure
back to the LLM; iter 2's HCL added `recovery_window_in_days = 0`
(or `force_destroy`) and destroy completed cleanly. The self-
learning pipeline worked — no static pitfall needed.

**Recommended order** (lowest-cost-per-closed-scenario first):

| # | Ticket | Effort | Closes |
|---|---|---|---|
| 1 | **Ticket A** — fakeaws ListSSHPublicKeys etc | ~15 min | `aws-full-stack` destroy |
| 2 | **Ticket B** — managed-policy orphan filter | ~30 min | `aws-full-stack` orphan-count |
| 3 | Ticket 5 — Secret Manager version 404 | ~1 hr | `gcp-secret-manager` |
| 4 | Ticket 3 (rest) — fakegcp Service Networking routes | ~1-2 hr | `gcp-cloud-sql` rest |
| 5 | Ticket 4 — plugin-crash family (NodePool + SQL) | ~3-4 hr per resource | `gcp-gke-cluster`, `gcp-full-stack` |
| 6 | Ticket 11 (rest) — audit other `*_custom_endpoint`s | ~1 hr | preventive |
| 7 | Ticket 10 — mirror "one-shot demo" in mock READMEs | ~15 min | (docs) |
| 8 | Ticket 8 — fakeaws subnet MapPublicIpOnLaunch persistence | ~1-2 hr | latent |
| 9 | `web-app-paris` regression investigation | unknown | regression |

**Realistic total to 39/39 deterministic**: ~6-8 hour session,
mostly mock-source work. Ticket 4 is still the deep one (provider
source-reading per resource); everything else is pattern-match.

## Next-session candidates (proposed 2026-05-31 23:50 after 39/39)

Now that the sweep is at 39/39 realistic, the remaining work is
durability + cleanup rather than scenario-closure. Six candidates
ranked by value-per-hour:

### N1. Full 39-scenario deterministic sweep — ~30-40 min

**Why:** The session closed all six previously-failing scenarios
via *one-off* validations. That proves each one CAN pass in
isolation but NOT that the whole set stays green simultaneously
under shared mock state. A clean sweep is the actual evidence for
the 39/39 claim.

**Fix:** Run `bash /tmp/sweep-revalidate.sh` (or regenerate from
this session's pattern). Reset mocks between scenarios. Tally
pass/fail. If any scenario regresses, treat the regression as a
new ticket — likely something we missed in the one-off validations
(e.g., scenario-to-scenario state leak that mock-reset doesn't
catch).

**Effort:** 30-40 min wall-clock, ~15 min active.

### N2. Prune stale verbatim mock-quirk pitfalls — ~1 hr

**Why:** `pitfalls/aws.yaml` and `pitfalls/gcp.yaml` accumulated
~10 verbatim mock-quirk entries during the 2026-05-31 sweep
(`aws_kms_key` DateType, `aws_iam_user_policy` empty-result wait
loop, `aws_subnet` MapPublicIpOnLaunch, `google_dns_record_set`
Plugin-did-not-respond, `google_container_node_pool` "not
implemented", `google_kms_crypto_key_iam_member`, etc.). Every one
of these underlying bugs was FIXED this session (T-A, T-B, T-C,
T-D-2, T-E, T-E-2, and the recovery commits). The pitfall entries
are now stale misinformation polluting future LLM prompts — they
teach the LLM to avoid resources that now work fine.

**Fix:** Per `feedback_sweep_protocol.md`: "Pruning stale entries
is OK after a mock-source fix." For each verbatim entry, verify
the underlying mock bug is fixed (run the relevant scenario, see
the resource now works), then delete the entry. Update the
`m91_no_seeding` ratchet test if needed.

**Effort:** ~1 hr (5-10 min per entry, ~10 entries).

**Pair with N3 for max effect** — if T12 lands first, future
sweeps won't accumulate these in the first place, and N2 becomes
one-time cleanup.

### N3. T12 — `isMockActionable` classifier predicate — ~half day

**Why:** The auto-learning pipeline currently writes ANY recurring
failure to `pitfalls/*.yaml` regardless of whether the failure is
LLM-actionable (real provider/cloud constraint) or mock-actionable
(mock-server gap that the LLM can't work around). The mock-actionable
ones are the verbatim mock-quirks N2 has to prune by hand. Without
T12 they keep accumulating every sweep.

**Fix:** Already specified in detail in section 12 below
(`Failure classifier — keep mock quirks OUT of pitfalls`).
Signals: `501 Not Implemented`, `Plugin did not respond`,
`OAuth ... access token`, `couldn't find resource (N retries)`,
`404 ... ResourceNotFoundException` on a Describe* path. When the
predicate fires, append to `docs/mock-gaps.md` instead of
`pitfalls/<cloud>.yaml`. Extend the M91 ratchet to fail CI if any
learned entry matches the predicate.

**Effort:** ~half day. Strict superset of the existing pipeline;
detection rules are simple substring/regex matches.

### N4. awsproto anonymous-struct compile-time guard — ~1-2 hr

**Why:** `WriteQueryRPCResponse(w, "Action", &struct{...}{})` with
a multi-field anonymous struct silently emits
`<!-- marshal error: xml: unsupported type: struct{...} -->`
inside the result wrapper. The handler returns 200 and looks fine
in unit tests if the assertions are loose; only live curl reveals
the broken output. Bit me TWICE this session:
  - T1 `iamGetUserPolicy` initially used an anon struct (fixed
    inline + documented on `iamGetUserPolicyResult`).
  - Ticket A first wave used anon structs for 4 new destroy-preflight
    handlers (`fd8e5d1` named-struct fix + tightened tests).
The single-field anon struct in `iamListGroupsForUser` happens to
slip through, which masks the pattern further.

**Fix:** Two options:
  (a) Refactor `WriteQueryRPCResponse` to require a non-anonymous
      typed argument (use a marker interface or method receiver).
      Compile-time enforcement.
  (b) Add a `go vet` analyzer that walks calls to
      `awsproto.WriteQueryRPCResponse` and flags anonymous-struct
      args with >1 field.
(a) is cleaner; (b) is less invasive. Either prevents the next
person from re-discovering the bug via a confused 30-min debug
session.

**Effort:** ~1-2 hr. (a) needs refactoring all existing call sites;
(b) is a new analyzer in `internal/tools/`.

### N5. `make restart-fakegcp` / `make restart-fakeaws` targets — ~15 min

**Why:** `make up` starts mocks via `go run ./cmd/fakegcp`, which
compiles ONCE at boot. After a commit, the running mock is on the
OLD binary. `kill $(cat pidfile)` only kills the `go run` wrapper
(captured by `$!`), not the compiled child process. Caused a 20-min
misdiagnosis this session on Ticket D-2 ("v3 endpoint flag isn't
working" → no, the binary is stale).

**Fix:** Add Makefile targets that do `pkill -f "fakegcp --port 8081"`
(matches the actual binary by command line, not pid) + restart via
`go run`. Mirror for fakeaws (`pkill -f "fakeaws --port 8082"`) and
mockway. Optionally a single `make restart-mocks` that does all three.

**Effort:** ~15 min.

### N6. Mockway README mirror demo — ~15 min

**Why:** fakeaws + fakegcp READMEs both have a "One-shot demo
(with sibling repos)" subsection pointing at infrafactory's
`make up`. Mockway hasn't been touched yet. Cross-repo polish item.

**Fix:** Copy the existing fakeaws/fakegcp subsection text into
mockway's README, adjust the example scenario path to a Scaleway
one (e.g. `scenarios/training/block-paris.yaml`).

**Effort:** ~15 min.

### N7. `make clean-bg` target + session-close convention — ~30 min

**Why:** Background-task hygiene bit me again this session. The
prior incident (documented in the existing "Session-close hygiene"
section below) covered `Monitor()` calls. This session's lingering
task was an auto-backgrounded `Bash` command — specifically the
`make up` from session start, which the harness backgrounded
because `make` spawns long-running children (mockway, fakegcp,
fakeaws, UI). The wrapper shell stayed in the task tracker for
the entire 4-hour session, triggering the "Background work is
running" prompt at exit.

The broader rule: **any harness task with `run_in_background: true`
needs an explicit `TaskStop` when its purpose is served**, not
just `Monitor()`. This includes:
  - `Bash` with `run_in_background: true` (validation runs,
    background builds).
  - `Bash` commands the harness auto-backgrounds because of
    long-running children (`make up`, `nohup ...`).
  - `Monitor()` calls (already covered in the existing convention).

**Fix:** Two parts:
  (a) **`make clean-bg`** (or `make clean`) Makefile target that
      kills any lingering `bash /tmp/sweep-*.sh`, `tail -F
      /tmp/sweep-*.log`, and per-mock `go run` wrappers from prior
      sessions. Cheap, removes friction at session start when the
      prior session crashed mid-run.
  (b) **Convention update**: extend the existing close-out checklist
      to call out `Bash run_in_background` tasks too, not just
      `Monitor`. A future session that runs a long sweep via
      `Bash(run_in_background=true)` should pair it with a
      `TaskStop` when the sweep finishes (or when bailing out on
      first failure).

(b) is a memory-update; (a) is a small Makefile addition.

**Effort:** ~30 min total.

### N8. `policy_pitfall_conflict` detection — ~2-3 hr

**Why:** The 2026-06-01 deterministic sweep surfaced
`web-app-paris` + `compute-lb-multi-paris` failures that looked
like LLM oscillation but were actually a real policy bug
(`policies/scaleway/vpc_required.rego` count-vs-singleton, fixed
in infrafactory PR #8). The auto-learning loop couldn't catch
this because the failure shape doesn't fit its model: the LLM's
HCL was *correct* (matched the existing prescriptive
`scaleway_instance_server` pitfall verbatim) but the policy
rejected it anyway. The loop wrote no new pitfall (correctly —
the LLM made no mistake), and the system bailed `stuck` after 2
iterations with no actionable signal beyond "same failure twice
in a row." Investigation cost ~30 min of human time that the
system could have flagged directly.

There's a detectable signature: **LLM's HCL matches an existing
prescriptive pitfall's prescription AND the same policy fires twice
in a row.** That's the "policy disagrees with its own pitfall"
signal — almost always a policy bug, not an LLM mistake.

This sits in the same anti-pattern family as T12 (mock-quirk
classification): a class of failure that should NOT live in
`pitfalls/*.yaml` because the fix isn't on the LLM side. T12 routes
mock-server gaps to `docs/mock-gaps.md`; N8 routes policy bugs to
`docs/policy-gaps.md`. Same shape, different category.

**Fix:** Extend the recurrence-detection pipeline:

1. After 2 consecutive failures with the same `policy=` failure
   detail, compare the iteration's generated HCL against the
   keyword set extracted from any existing same-resource
   prescriptive pitfall (e.g., for
   `policy=scaleway.vpc_required` on `scaleway_instance_server`,
   the keyword set is `["scaleway_instance_private_nic",
   "private_network_id", "server_id"]`).
2. If the HCL contains all the prescribed keywords AND the policy
   still fires → write a structured entry to `docs/policy-gaps.md`
   instead of `pitfalls/<cloud>.yaml`. Optional sugar: open a
   GitHub issue against the policy file via `gh issue create`.
3. Extend the M91 no-seeding ratchet to assert no learned pitfall
   matches the policy-pitfall-conflict signature — turning the
   principle into CI enforcement.

Keyword extraction can start simple (regex against the rule body
for backticked-identifier tokens; reject common words). False
positives flag for human review; false negatives just fall back
to the existing "stuck after 2" terminal state.

**Effort:** ~2-3 hr. The hard part is the keyword-set extraction
heuristic; everything else mirrors T12's classifier shape.

Pair with N3 (T12) — both push the same kind of "this isn't an
LLM mistake" failure out of the pitfalls file into the right
queue. If you're doing T12 anyway, this is a small extension on
top.

### N10. Diff-based prescriptive-pitfall extractor — ~1 day (~8 hr focused)

**Why:** Today's `ExtractLearnedPitfall` captures the *symptom* (the
stderr-shaped failure detail) as the pitfall rule, not the *fix*.
The 2026-06-02 sweep showed why this matters: `gcp-storage` learned
`google_storage_bucket has no encryption.default_kms_key_name —
customer-managed encryption not configured` but the LLM still
oscillated because nothing in the rule told it to
"create a `google_kms_key_ring` + `google_kms_crypto_key` and
reference its `.id` via an `encryption {}` block." Prescriptive
guidance currently only comes from hand-crafted prompts (rules 13–16
of `prompts/gcp/phase2_generate_hcl.md`).

This extractor closes that gap: when a run eventually passes after N
failed iterations, diff `iter[N].generated/*.tf` against
`iter[N-1].generated/*.tf`, attribute the additions to the failures
they cleared, and write a prescriptive pitfall containing the
minimal HCL snippet that made the difference.

**Once landed, prompt rules 13–16 become candidates to delete** —
the system will re-derive them from the first successful run for
each pattern (gated on the N11 follow-up below).

**Trigger.**
- `terminal_reason == target_reached` AND at least one prior iteration
  failed.
- For each adjacent `(iter[N-1], iter[N])` pair where `iter[N-1]`
  had failures and `iter[N]` cleared at least one of them, run the
  extractor against each cleared failure.

**Diff mechanics** — new `internal/generator/prescriptive_extractor.go`:

```go
func ExtractPrescriptiveFix(
    failedDir, passingDir string,
    failure FailureSummary,
    cloud, scenario, timestamp string,
) (*PitfallEntry, error)
```

1. Parse all `*.tf` in both dirs with `hashicorp/hcl/v2/hclparse`.
2. Build per-iteration maps: `{resource_address → *hclsyntax.Body}`.
3. Locate the failing resource by address (`failure.Resource` or
   extract from `failure.Detail`).
4. Scoped diff: new attribute blocks on the failing resource AND new
   resources whose `.id` is referenced from those new attributes.
5. Serialise the additions back to HCL via `hclwrite`; strip
   comments, normalise whitespace, cap at ~600 chars.
6. Rule string: `"<resource>: <one-line summary>. Minimal HCL:
   <snippet>"`.

**Wiring** — `internal/cli/run_command.go`:

After the existing `oscillation_pitfall_learned` block, when
`terminalReason == "target_reached"` and `hasPriorFailures`, walk
adjacent clearing pairs and call `ExtractPrescriptiveFix` per
cleared failure. Append via existing `AppendPitfall`.

**Storage / dedup.**
- Same `pitfalls/<cloud>.yaml`, same writer.
- New `source: learned_from_diff` discriminator (distinct from
  `learned`).
- Extend `isDuplicate` to ignore whitespace differences in the rule.

**Auto-write decision (resolved 2026-06-02):** confident enough to
write to `pitfalls/` automatically. CI ratchet guards quality
(no rules longer than 600 chars, no comment-only diffs, no rules
that match a mock-actionable signal). Revisit if false positives
accumulate.

**Cross-cloud isolation:** a `google_storage_bucket` CMEK fix
learned from `gcp-storage` should not be re-extracted from
`gcp-full-stack`. Existing `isDuplicate` should handle this — add
an explicit test case.

**Scope:** validate/apply failures only. Orphan_check stays with N9's
specialised sub-shape table; the extractor doesn't try to learn
those.

**Risks & mitigations:**
- **Noise diff** (unrelated LLM changes) → scope to the failing
  resource's address subtree only.
- **Multi-failure interleaving in one iteration** → per-failure
  attribution; skip if no diff entry touches the failure's address.
- **HCL parser quirks** → fall back to skipping if parsing fails.
- **Deletion-as-fix** (e.g., LLM removes `deletion_protection`) →
  out of scope for phase 1; phase 2 if needed.

**Effort:** Phase 1 extractor + table tests ~5-6 hr; Phase 2
`run_command.go` wiring + integration test ~2 hr; Phase 3
`feedback_sweep_protocol.md` + ADR-0012 amendment ~30 min. **Total
~8 hr, one PR.**

**Validation:**
1. Take `gcp-storage` (currently failing on CMEK), keep prompt rule
   16 in place, let it pass once.
2. Confirm the extractor wrote a prescriptive `google_storage_bucket`
   pitfall to `pitfalls/gcp.yaml`.
3. Revert prompt rule 16 to the pre-2026-06-02 wording, blank the
   pitfall, re-run — the prior run's learned pitfall should get the
   LLM to pass on iter 1. Proves prescriptive learning works
   end-to-end without prompt support.

---

### N11. Retire prescriptive prompt rules once N10 stable — ~2 hr

**Why:** N10 produces prescriptive pitfalls automatically. Most of
`prompts/gcp/phase2_generate_hcl.md` (and the parallel `aws/` and
`scaleway/` phase-2 prompts) is hand-written prescriptive guidance
that N10 can re-derive from real runs. Prompt-as-source-of-truth
doesn't scale — every cloud has dozens of these gotchas, and the
system already sees each one when a scenario passes.

**In scope for retirement** (gcp/phase2_generate_hcl.md, mirrors in
phase1/phase3 + the aws/ + scaleway/ equivalents):

| Rule | Pattern | Why N10 can learn it |
|---|---|---|
| 9 (no `google_project_service` / `google_service_networking_connection`) | Avoid (deletion-as-fix) | Phase-2 of N10 (see Risks); not phase-1 |
| 10 (VPC + subnetwork wiring) | Add resources + reference attrs | Addition-as-fix; phase 1 |
| 11 (firewall `network` not `subnetwork`) | Attribute correction | Single-attribute diff; phase 1 |
| 12 (IAM principals format) | Attribute correction | Single-attribute diff; phase 1 |
| 13 (GKE single-node-pool strategy) | Attribute + sibling-resource pattern | Phase 1 |
| 14 (Cloud SQL flags + name suffix) | Multi-attribute pattern | Phase 1 |
| 15 (GCS `force_destroy` + uniform access) | Multi-attribute pattern | Phase 1 |
| 16 (CMEK mandatory) | Add KMS sibling resources + `encryption {}` block | The 2026-06-02 motivating case for N10 — phase 1 |

**NOT in scope** (keep in prompt):
- Rule 1–8 (system-level: provider source, file structure, variables
  with defaults, no data sources, no LLM-credentials). These are
  *meta* rules about how to write HCL at all, not specific
  resource gotchas — N10 can't learn them because they apply to
  every scenario from iter 1 onwards.
- Rule 17 (region restriction) — bound to scenario params, not a
  static fix.
- Rule 18 (naming convention) — same.

**Gated on N10 stability.** Don't delete blindly. The validation
sequence below is critical because a passing-with-prompt-rule
scenario can MASK whether the learned pitfall actually carries the
load — the prompt rule and the pitfall might both be active when
the LLM succeeds, and you wouldn't know which one mattered.

**Validation workflow (per rule):**

1. **Sweep with N10.** Run a full sweep. Each pass should cause N10
   to extract a `learned_from_diff` pitfall for the patterns the
   LLM used.
2. **Inspect.** Open `pitfalls/<cloud>.yaml`. For each prompt rule
   under consideration, check whether there's a `source:
   learned_from_diff` entry covering that same pattern.
   - If yes: that rule is retirement candidate.
   - If no: leave the rule. Either the pattern wasn't exercised in
     any passing scenario, or N10 mis-attributed (file a follow-up
     to tighten the extractor).
3. **Delete the prompt rule** locally (don't commit yet).
4. **Blank the matching pitfall entry** in `pitfalls/<cloud>.yaml`
   locally (the one N10 wrote in step 1).
5. **Re-run the scenario that exercises that pattern.** Expected
   result: **the scenario fails**. No prompt rule, no pitfall = no
   signal to the LLM. This confirms the prompt rule was load-bearing
   and the auto-learned pitfall is the actual replacement.
6. **Restore the auto-learned pitfall.** Re-run. Expected: **the
   scenario passes on iter 1 or 2.** This confirms the pitfall
   alone is sufficient.
7. Commit the prompt-rule deletion.

If step 5 doesn't show a regression, the prompt rule was redundant
to something else (another prompt rule, the provider's own
validation, a static pitfall). That's fine — just delete with no
follow-up.

If step 6 doesn't recover, the auto-learned pitfall is malformed.
Open an N10 follow-up to fix the extraction; don't delete the
prompt rule.

**Effort:** ~30 min per rule × ~5 retiring rules per cloud × 3
clouds = ~7-8 hr if pursued comprehensively. Much smaller (~1-2 hr)
if scoped to GCP rules 13–16 only.

**Why this is worth doing:** the prompt collapses from "playbook of
every gotcha across every resource" to "system contract + scenario
intent." The system's prescriptive knowledge becomes a living
artifact in `pitfalls/<cloud>.yaml` that auto-updates as the LLM
and the mocks evolve, instead of stale hand-written prose.

---

### N12. fakegcp gaps from 2026-06-02 sweep — ~half-day each

The 9-scenario re-validation showed fakegcp has several
mock-side gaps beyond `google_project_service` / SQL dual-prefix
(both fixed this session). The N3 classifier correctly routed
each to `docs/mock-gaps.md`; the fixes belong against fakegcp,
not infrafactory.

Open mock-gaps (see `docs/mock-gaps.md` for full failure detail):

| Resource | Signal | Affects | Likely fix |
|---|---|---|---|
| `google_container_node_pool` | `plugin did not respond` | gcp-gke-cluster, gcp-full-stack | Response shape: `GetNodePool` is missing fields the v5 SDK derefs (likely `networkConfig`, `management`, `upgradeSettings` defaults). Reproduce locally with `TF_LOG=TRACE` against a minimal HCL; compare against real GCP `Get` response. |
| `google_sql_database_instance` | `plugin did not respond` | gcp-full-stack | Same investigation. `GetSQLInstance` response likely missing `settings.activationPolicy` / `settings.ipConfiguration` defaults. |
| `google_compute_instance` | `plugin did not respond` | gcp-full-stack | Same investigation. `GetInstance` response likely missing `disks[].source`, `networkInterfaces[].fingerprint`, or `selfLink`. |
| `google_kms_crypto_key_iam_member` | `Provider produced inconsistent result after apply` | gcp-gke-cluster | The Apply response differs from the planned state. fakegcp's `SetIamPolicy` likely returns a different `etag` than the provider expected. |
| `cryptoKeyVersions` 501 | `501: not implemented` | gcp-storage | Missing route: `GET /v1/projects/{p}/locations/{l}/keyRings/{r}/cryptoKeys/{k}/cryptoKeyVersions`. Returns a list of versions; with one synthetic `PRIMARY` version. |
| `google_service_account` | 401 escape | gcp-full-stack | Possibly NOT a real escape — fakegcp's response shape may be missing fields and the SDK formats the 401 misleadingly. Reproduce + verify. |

**Approach per resource:**
1. Run minimal HCL against fakegcp with `TF_LOG=TRACE`.
2. Capture the HTTP request + fakegcp's response.
3. Compare against real GCP API reference for that endpoint.
4. Fill in the missing fields with sensible synthetic values
   (stable, derived from input where possible — same discipline as
   the existing `GetProject` handler).
5. Add a `TestXXX_PluginAcceptsResponse` test that POSTs+GETs
   through fakegcp and asserts the response shape passes
   `googleapi.Decode`.

**Effort:** ~half-day per resource (5 above = 2-3 days). Some may
share root causes (e.g., several plugin-crashes might share a
missing `selfLink` field).

**Why now:** the mock-gaps file is the official backlog for fakegcp;
each row is a concrete reproducer with the failing URL + scenario.

---

### N9. `orphan_check` extractor — classify the 5 sub-shapes — ~1 day

**Why:** `orphan_check detected N orphaned resources` is a
single failure signature that masks at least **five distinct root
causes**, each needing a different fix channel. The auto-learning
pipeline currently can't differentiate, so it writes nothing (the
failure isn't a stderr-shape it knows) and the LLM oscillates or
the run terminates `stuck`. aws-full-stack hit this in the 2026-06-01
deterministic sweep; the sub-shape was sub-shape #1 (Secrets Manager
soft-delete) and the dynamic loop broke it ~50% of the time. Other
sub-shapes (#2-5) the system can't fix at all from inside the
loop — they're mock or policy bugs.

This is the highest-leverage learning-pipeline extension because
orphan_check is the only validation-layer failure mode currently
opaque to the learning system, and it covers a real gap in
production sweeps.

**The five sub-shapes:**

| # | Cause | Fix channel |
|---|---|---|
| 1 | **LLM-side soft-delete**: aws_secretsmanager_secret without `recovery_window_in_days = 0`, aws_kms_key without `force_destroy`, etc. Provider's destroy returns 200, mock state lingers in `PendingDeletion`. | `pitfalls/<cloud>.yaml` — prescriptive rule for the resource type. |
| 2 | **Mock-side auto-seeded catalogue**: fakeaws's SeedManagedPolicy auto-creates `arn:aws:iam::aws:policy/*` rows. Tenant never owned them; orphan check sees them. | `docs/mock-gaps.md` — file/PR against the mock to filter from `/mock/state`. *T-B this session was an example.* |
| 3 | **Mock-side CASCADE missing**: parent deleted, child rows orphaned because the mock's schema lacks `ON DELETE CASCADE`. | `docs/mock-gaps.md` — fix in the mock's repository schema. |
| 4 | **Mock-vs-provider state divergence**: mock Create succeeds, provider Read returns null/partial (wrong URL, missing handler), tfstate doesn't track → destroy can't find → mock retains. | `docs/mock-gaps.md` — same shape as T-D-2 / T-E this session. |
| 5 | **Provider-side soft-delete** (distinct from #1): provider reports destroy success but underlying API only schedules deletion. Sometimes the mock can hard-delete on its side as a faithful-but-faster behaviour; sometimes the LLM needs a force flag. | Mixed — pitfall if force flag exists, mock-gap otherwise. |

**Fix path:**

1. Add `matchOrphanCheck(failureDetail, mockStateJSON, scenario) → LearnedPitfall | MockGapEntry | PolicyGapEntry | nil` to `internal/generator/pitfalls_learn.go`. Signature differs from existing M97 templates: it needs the live mock state, not just stderr.
2. The harness calls this extractor on `orphan_check` failures, passing `/mock/state` snapshot (already captured for the check itself).
3. Extractor walks the non-empty resource collections, looks up each `(service, collection)` pair in a hard-coded sub-shape table (keyed at module init):

       sub-shape table (initial seed — extend per scenario):
         aws       secretsmanager.secrets   → #1 soft-delete, force "recovery_window_in_days = 0"
         aws       kms.keys                 → #1 soft-delete, force "force_destroy = true"
         aws       iam.policies (arn:aws:iam::aws:policy/*)  → #2 auto-seed
         gcp       secretmanager.secrets    → #1 (similar to AWS)
         gcp       kms.crypto_keys          → mixed (#1 or #5)

4. For each lingering resource that matches a known sub-shape, emit the appropriate output:
   - sub-shape #1 → `AppendPitfall(...)` with the prescriptive rule.
   - sub-shape #2-#5 → write to `docs/mock-gaps.md` (T12 channel) or `docs/policy-gaps.md` (N8 channel).
   - Unrecognised lingering resource → descriptive fallback to `docs/orphan-gaps.md` for human triage.
5. Extend the M91 no-seeding ratchet: if any learned pitfall matches an orphan-check-routable sub-shape #2-#5, fail CI. Ensures we don't regress the routing.

**Tests:**
- Per sub-shape, a unit test with synthetic `(failureDetail, mockState)` → assert the right output channel + content.
- A regression with the aws-full-stack iter-2 state showing `secretsmanager.secrets: 1` → asserts a prescriptive `aws_secretsmanager_secret` pitfall is emitted with `recovery_window_in_days = 0`.

**Effort:** ~1 day. Most of the work is the sub-shape table (small, but easy to under-engineer — start with the 5 entries above, extend per scenario as they surface). The extractor wiring + tests mirror existing M97 patterns.

**Why now (user-prioritised 2026-06-01):** previously gated on "3+ scenarios show the same pattern" per `feedback_orphan_check_extractor.md`; reprioritised because the gap is the system's biggest blind spot and the design effort is mostly upfront (sub-shape table) rather than per-scenario.

---

## Recommended order

If picking ONE thing: **N1 → N2** together (~2 hr total). The
sweep proves 39/39 deterministic, and the prune cleans the pitfalls
file based on what the sweep revealed as actually-recurring vs.
fixed.

If picking the most durable thing: **N3** (T12 classifier) —
prevents future sweeps from re-polluting the pitfalls files,
making N2 a one-time job rather than a recurring cleanup.

Quick wins: N5 + N6 + N7 together (~75 min).

N8 pairs naturally with N3 — both classify "not an LLM bug" failures
into the right queue (mock-gaps for T12, policy-gaps for N8). If
doing T12 anyway, N8 is a small extension.

N9 is the highest-leverage learning-pipeline extension — closes
the system's only currently-opaque validation-layer failure mode.
The dynamic loop catches sub-shape #1 (LLM soft-delete) ~50% of
the time today; sub-shapes #2-5 are completely outside the loop's
reach until the routing lands. **User-prioritised 2026-06-01.**

---

## Open follow-ups from prior sessions (mostly closed)

Tickets are detailed below in the same order they appeared during
the sweep — but the map above is the order to *work* them.

### 1. `aws-full-stack` — IAM user policy persistence (fakeaws) — CLOSED

**Closed by:** fakeaws `c92e323` (2026-05-31 evening session).
`user_policy_attachments` + `user_inline_policies` tables added in
`repository/iam.go`; handlers in `handlers/iam.go` switched from
no-op stubs to persistence-backed implementations; round-trip tests
landed in `handlers/iam_test.go`
(`TestIAM_AttachDetachUserPolicy`, `TestIAM_PutGetDeleteUserPolicy`).

Verified live: `curl AttachUserPolicy` + `curl ListAttachedUserPolicies`
returns the attached ARN.

aws-full-stack does NOT yet close end-to-end though — destroy now
fails on `ListSSHPublicKeys` 404 and orphan-check fires on
auto-seeded managed policies. See Tickets A + B above.

### 2. GCP `sql_custom_endpoint` path duplication — CLOSED

**Closed by:** infrafactory `8566033`. `sql_custom_endpoint` flipped
to host-only in `internal/cli/generate_command.go::buildGoogleProviderBlock`
(inline comment explains v5 provider's strip-regex behaviour on
http:// endpoints). `dns_custom_endpoint` also flipped to host-only
in the same commit (closes `gcp-dns`).

### 3. fakegcp `Service Networking` endpoint missing — PARTIAL

**Partially closed by:** infrafactory `8566033` —
`service_networking_custom_endpoint` added to the provider template
so requests stop escaping to the real cloud. **Pending:** fakegcp
routes still need to land so the requests have somewhere to land:

- `GET  /v1/services/{service}/connections` (returns empty list)
- `POST /v1/services/{service}/connections` (200 + synthesised resource)
- `PATCH /v1/services/{service}/connections` (200)
- `DELETE /v1/services/{service}/connections/{name}` (200)

### 4. fakegcp DNS / SQL / NodePool plugin crashes

**Symptom:** `gcp-dns`, `gcp-gke-cluster`, `gcp-full-stack` see
"Plugin did not respond" on `google_dns_record_set`,
`google_container_node_pool`, `google_sql_database_instance`.

**Why:** fakegcp returns JSON that mismatches what the v5 provider's
parser expects. Provider crashes with nil-deref on missing fields.

**Fix path:** For each resource:
1. Run a single create against fakegcp with verbose terraform logging
   (`TF_LOG=DEBUG`) to see the exact request + response.
2. Compare response shape to the real GCP API spec for that resource.
3. Identify the missing/wrong field. Often it's an embedded sub-block
   the provider tries to dereference (e.g. `nodePool.config`,
   `dnsRecordSet.rrdatas`).

These need actual reading of provider source, not pattern-matching.

### 5. fakegcp Secret Manager `SecretVersion` 404

**Symptom:** `gcp-secret-manager` 404s on
`GET /v1/projects/.../secrets/.../versions/.../data` (or similar
read path) even right after create.

**Fix:** Inspect `fakegcp/handlers/iam.go` (Secret handlers — the
file is misnamed, secrets live there) for the SecretVersion lookup.
Likely a key-mismatch (version IDs not persisted as the provider
expects) or path-param parsing issue.

### 6. `gcp-cloud-run` — LLM hallucinates `deletion_protection` — CLOSED

**Closed by:** infrafactory `8566033`. `matchUnsupportedArgument`
template in `internal/generator/pitfalls_learn.go` handles wrapped
`Unsupported argument` diagnostics. The verbatim→prescriptive
upgrade path in `AppendPitfall` lets later prescriptive rules
REPLACE earlier raw-stderr entries for the same resource (otherwise
the descriptive dump would permanently shadow the prescriptive form
via dedup). First non-cloud-run firing: `google_redis_instance` got
a clean prescriptive rule for `deletion_protection` in the same
sweep.

### 7. Mock-state reset built into `infrafactory run` — CLOSED

**Closed by:** infrafactory `8566033`. `--reset-mocks` flag (default
true on clean runs) fans `POST /mock/reset` to every configured
mock URL (mockway/fakegcp/fakeaws). Replaces the per-script curl
fan-out the sweep scripts used to do externally.

### 8. fakeaws `aws_subnet.MapPublicIpOnLaunch` doesn't persist

**Symptom:** Provider's wait-loop polls `DescribeSubnets` after
`ModifySubnetAttribute` and waits 5 min for `MapPublicIpOnLaunch=true`,
times out. (My `ModifySubnetAttribute` is no-op; the attribute
isn't stored.)

**Fix:** Store subnet-level scalar flags in
`fakeaws/repository/ec2.go::EC2Subnet` struct, persist on
ModifySubnetAttribute, surface on DescribeSubnets.

### 9. Update `CONTRIBUTING.md` to reference `make up` — CLOSED

**Closed by:** infrafactory `8566033`.

### 10. Mirror "make up" demo in fakegcp + mockway READMEs

**Status:** `fakeaws/README.md` got a "One-shot demo (with sibling
repos)" subsection pointing at infrafactory's `make up`. fakegcp and
mockway READMEs should get the same blurb so a user landing on any
mock repo's GitHub page sees the consistent entry point.

### 11. `cloud_resource_manager_custom_endpoint` and others — PARTIAL

**Closed for `dns_custom_endpoint`:** infrafactory `8566033`. The
remaining endpoints still need a per-endpoint audit:
`cloud_resource_manager_custom_endpoint` (currently `/v1/`),
`cloud_run_v2_custom_endpoint` (currently `/v2/`),
`pubsub_custom_endpoint` (currently `/v1/`),
`storage_custom_endpoint` (currently `/storage/v1/`),
`secret_manager_custom_endpoint` (currently `/v1/`),
`redis_custom_endpoint` (currently `/v1/`).

For each: trace through the matching scenario and verify the wire
URL fakegcp receives matches the registered route. Pattern is
"if the v5 provider's lib client uses RemoveBasePathVersion or a
ReplaceAll-based path strip that doesn't fire on http:// endpoints,
the trailing path doubles."

### 12. Failure classifier — keep mock quirks OUT of pitfalls

**Status:** The auto-learning pipeline currently writes ANY recurring
failure to `pitfalls/*.yaml`, including ones that describe mock-server
gaps (`501 Not Implemented`, `Plugin did not respond`, OAuth-escape,
404 from a Describe* on a resource we just created). This violates
the "mock quirks are tickets, not pitfalls" principle — it teaches
the LLM to AVOID valid resources because the mock is incomplete.

**Fix:** Extend `internal/generator/pitfalls_learn.go` with a
`isMockActionable(detail)` predicate. Detection signals:

- `501 Not Implemented` anywhere in the body
- `Plugin did not respond`
- `OAuth ... access token` / `Request had invalid authentication credentials`
- `couldn't find resource (N retries)` (wait-loop after Describe* miss)
- `404 ... ResourceNotFoundException` on a Describe* path

When the predicate fires, instead of writing to `pitfalls/<cloud>.yaml`,
append a structured entry to `docs/mock-gaps.md` (or open a GitHub
issue against the right mock repo). Also extend the M91 no-seeding
ratchet (`TestPitfallsNoHumanSeeding`) to assert no learned entry
matches the mock-actionable predicate — turning the principle into
CI enforcement.

Effort: ~half-day. Strict superset of the existing pipeline; the
detection rules are simple substring/regex matches.

### 13. Self-healing mocks (second agent loop) — multi-session direction

**Status:** Ticket 12 is the lightweight "detect + document" version.
The heavy version is a second agent loop with read+write access to
the mock repos. When a mock-actionable failure is detected, it:

1. Reads the failing request from infrafactory's run artifacts.
2. Locates the matching handler in fakeaws / fakegcp / mockway — or
   detects that no handler exists.
3. Proposes a patch (adds a route + handler, or fixes a response
   shape) using the provider source as ground truth.
4. Runs the mock's existing tests to make sure nothing else breaks.
5. Restarts the mock + retries the failing scenario.
6. If green, opens a PR on the mock repo; if red, leaves a tracked
   ticket.

This closes the principle's loop end-to-end: mock quirks don't just
get TICKETS, they get fixed. Risk: less bounded than the current
LLM-driven pipeline because mock changes affect every scenario — needs
guardrails (per-PR scope, sibling-scenario regression run, human
approval for the patch).

Effort: multi-session. Worth scoping in its own slice before
starting. ADR-level decision (cross-repo write authority).

### 14. Audit + prune existing mock-quirk entries from pitfalls/*.yaml

**Status:** Per the principle above, almost every current entry in
`pitfalls/aws.yaml` (5/5) and many in `pitfalls/gcp.yaml` (~6/10) are
mock-quirk fingerprints written before the classifier existed. They
should be removed and their underlying gaps tracked as tickets against
the matching mock repo.

**Entries currently violating the principle** (as of 2026-05-31):

`pitfalls/aws.yaml` (all 5 entries):
- `aws_subnet` MapPublicIpOnLaunch wait-loop timeout → ticket 8.
- `aws_iam_role_policy_attachment` 404 → already fixed in fakeaws
  `348322d` (managed-ARN auto-seed), pitfall is stale.
- `aws_iam_role_policy` 404 → already fixed in fakeaws `348322d`
  (PutRolePolicy handler), pitfall is stale.
- `aws_vpc` "Failed to instantiate provider" → transient network
  (registry-download timeout), not learnable.
- `aws_db_instance` wait-loop "couldn't find resource" → fakeaws
  RDS create→read shape mismatch, new ticket.

`pitfalls/gcp.yaml`:
- KEEP: `google_compute_instance` + `google_container_cluster` VPC
  declaration rules (real GCP-side requirement).
- KEEP: `google_storage_bucket` no CMEK (real policy requirement —
  this is a Layer-1 gate).
- KEEP: `google_cloud_run_v2_service` `deletion_protection`
  (legitimate "the v5 provider doesn't accept this arg").
- PRUNE: `google_service_account` "escaping" → fixed by v5 pin +
  `iam_admin_v1` work, stale.
- PRUNE: `google_project_service` "escaping" → fixed by
  service_usage endpoint fix, stale.
- PRUNE: `google_service_networking_connection` "escaping" → fakegcp
  gap, ticket 3.
- PRUNE: `google_kms_key_ring` "escaping" → fixed by KMS handler
  (fakegcp `c7999b5`), stale.
- PRUNE: `google_container_node_pool` "Plugin did not respond" →
  ticket 4.
- PRUNE: `google_dns_record_set` "Plugin did not respond" → ticket 4.

**Fix:** Manual prune of stale entries (those that reference
already-fixed gaps), file mock-repo tickets for the rest, document
the principle in the auto-learning loop comments. Effort: ~1 hr.

Pair with ticket 12 so the principle is enforced going forward, not
just retroactively.

## Workflow / harness improvements

Things I want to land before they're forgotten in a fresh session:

### A. `make up` / `make down` shipped this session

Brings up mockway + fakegcp + fakeaws + SeaweedFS + UI in one
command. See `Makefile` head — added 2026-05-31.

### B. Add `infrafactory sweep` subcommand

The 5+ bash sweep scripts I wrote (`/tmp/sweep-*.sh`) replicate the
same pattern: iterate scenarios, reset mocks between each, log
per-scenario output, tally pass/fail at the end. This is the right
ergonomic to land in-tree.

**Spec:**
```
infrafactory sweep [--scenarios FILE.txt | --all] [--continue-on-fail] [--reset-mocks=true]
```

- Default `--all` runs every `scenarios/training/*.yaml`.
- `--scenarios FILE.txt` reads scenario paths from a file (one per line).
- `--continue-on-fail` keeps going past failures (default stop on first).
- Emits a final summary table to stdout.

Suggested location: `internal/cli/sweep_command.go`.

### C. Sub-session checkpoints

Long sessions like this one (40+ commits of fixes across 4 repos)
benefit from intermediate "save progress" markers. Convention I'd
suggest: after every ~5 mock-source fixes, run `git stash` or a
WIP commit. Avoids losing a chain of fixes to one bad change.

### D. `pitfalls/*.yaml` doesn't survive the `--reset` flow

Currently the auto-learning loop appends to pitfalls files in the
repo working tree. A `make reset-pitfalls` target would clear all
three to `pitfalls: []` for clean sweeps (useful when validating the
learning loop itself rather than relying on accumulated knowledge).

### E. Visual regression test runs page

Made the runs page screenshot viewport-only with the table masked
(2026-05-30). Should review whether other dynamic pages (compare,
diagnostics) have the same growing-content fragility.

## Commit landmarks (this session)

Across 4 repos:

- **fakeaws** `348322d` — broad EC2/IAM/KMS/Route53/DynamoDB coverage
  (~17 fixes).
- **fakegcp** `c7999b5` — Cloud KMS stubs for CMEK-required scenarios.
- **infrafactory** `7728658` — M98 after-apply-reference policy work
  (pre-staged); `bf3727e` — feedback pipeline + generator templates +
  policy exemptions + provider wiring.

## Memory pointers

- `feedback_sweep_protocol.md` — "fix at source, never seed
  pitfalls; pruning stale entries after a mock-source fix is OK."
  This is load-bearing — re-read it before making decisions about
  `pitfalls/*.yaml` editing.
- `feedback_mock_design.md` — "mocks optimise for fast feedback, not
  realism." Drives decisions like AMI auto-seed, terminated-instance
  GC, KMS hard-delete on schedule.

## How to verify the current state

```bash
make up                                        # all mocks + UI
make test                                      # Go + UI + Playwright
/tmp/infrafactory-new run scenarios/training/block-paris.yaml --config infrafactory.yaml
# Expect: terminal_reason: target_reached, completed_iterations=1
```

If you need to reproduce the full sweep:

```bash
# Use the script from this session
bash /tmp/sweep-full.sh   # if still on disk, else regenerate from this file's sweep order
```

## Session-close hygiene (lessons from 2026-05-31 + 2026-06-01 closes)

Long sessions accumulate persistent background harness tasks that
survive across sub-sessions and only die at session exit. They
don't hurt anything but they cause the harness to prompt
"Background work is running" when you try to exit. Always call
**TaskStop** on every backgrounded task before closing the session.

**Scope** (broadened after 2026-06-01 incident): the rule covers
NOT JUST `Monitor()` calls but ALSO:
- `Bash` with explicit `run_in_background: true`.
- `Bash` commands the harness auto-backgrounds because of
  long-running children (`make up`, anything `nohup`-style).

Workflow improvements proposed in [[N7]] above:

- **Convention**: every backgrounded task gets a paired `TaskStop()`
  when its purpose is done — don't leave them armed "just in case."
  Especially after a sweep finishes (or stops on the first failure
  for the bail-out pattern) — stop the monitor/wrapper that was
  tailing its stdout immediately.

- **Tooling**: `make clean-bg` (or `make clean`) target that finds
  and kills any lingering `bash /tmp/sweep-*.sh` + `tail -F
  /tmp/sweep-*-stdout.log` processes the previous session left
  running. Cheap, removes friction. (See N7 for full spec.)

Specific background tasks that survived prior-session closes
(stopped manually before exit):
- 2026-05-31: `bzlwxl814` (sweep-progress monitor), `bek3umbwi`
  (sweep 25 reset monitor), `b2s4vo2i5` (revalidation-sweep monitor).
- 2026-06-01: `b7qym91sk` (the `make up` wrapper shell that stayed
  alive the entire ~4-hour session because make spawned long-running
  child mocks).
