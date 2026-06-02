# Status Archive

Historical snapshots and older session notes can be moved here to keep `STATUS.md` concise.

## 2026-02-22 (initial OSS docs baseline)

- Added contributor-facing docs: `README.md`, `CONTRIBUTING.md`.
- Added architecture summary: `docs/architecture.md`.
- Added ADR scaffold: `docs/decisions/README.md`, `docs/decisions/0001-foundations.md`.
- Replaced dated status snapshot with milestone roadmap in `ROADMAP.md`.
- Added structured decision workflow and doc sync rules to `AGENTS.md` and `CODEX.md`.

## 2026-06-02 S54–S62 sustain + prompt-collapse arc — close-out notes

Per-slice close-out details for the S54–S62 arc, archived from NEXT_SESSION.md after the arc closed. See `docs/plans/slices-54-62-plan.md` for the plan + standing rules, and ADR-0012 (with amendments) + ADR-0018 for the durable architectural decisions captured during the arc.

## 2026-06-02 S61 N13 deletion-as-fix close-out

S61 added `ExtractPrescriptiveAvoid` — the dual of N10's addition-as-fix extractor. When the LLM clears a failure by REMOVING HCL rather than adding it, the system now emits a "do NOT use" pitfall with `source: learned_from_diff_avoid`.

Two attribution paths:
- **Attribute-level**: a removed attribute whose name appears verbatim in the failure detail. Covers the `deletion_policy = "DELETE"` hallucination case from S59's gcp-cloud-run flakiness.
- **Resource-level**: ALL instances of a top-level resource type were removed AND that type name appears in the failure detail. Covers `google_project_service` / `google_project_iam_member` escape patterns that PR #18 + #23 had to hand-retire from prompts.

Strict attribution rules: returns nil if the deletion can't be tied to the failure (unrelated LLM refactors); partial-removal skips resource-type emission (ambiguous). Four unit tests pin attribute removal, resource removal, unrelated-removal-returns-nil, and partial-removal-no-emit. Both N10 and N13 extractors run together per cleared-failure iter pair — either or both can emit.

ADR-0012 amended with N13 design rationale. M91 + S55-T3 ratchets extended to whitelist + size-check `learned_from_diff_avoid`.

**Validation against recorded runs**: deferred to next sweep — the synthetic tests cover the mechanism; the real validation comes when a sweep produces an avoid entry organically. The S59 gcp-cloud-run \"deletion_policy\" hallucination is the most likely trigger — if it recurs and the LLM self-corrects by dropping the attribute (versus introducing yet another wrong one), N13 will record the rule and prevent future occurrences.

## 2026-06-02 S60 N11 retirement close-out

S60 generalized the N11 retirement protocol across clouds. Retired:
- **AWS phase3 rule 3 sub-bullet**: \`deletion_protection = false\` on RDS instances. Re-ran `aws-rds` → target_reached iter 1. LLM produced `skip_final_snapshot = true` (provider default `deletion_protection = false` is implicit-fine).
- **Scaleway phase3 rule 7** (two sub-rules in one): RDB `private_network` requiring `ip_net` OR `enable_ipam = true`, AND LB `assign_flexible_ip` conflict with `ip_ids`. Re-ran `mysql-ha-paris` → target_reached iter 2 (RDB `private_network { pn_id = ... enable_ipam = true }` correctly produced); `lb-paris` → target_reached iter 1 (`ip_ids = [scaleway_lb_ip.main.id]` with no `assign_flexible_ip`).

Per protocol step 5 (both): rules redundant — delete with no follow-up. **Seventh, eighth, ninth N11 retirements** (CMEK + firewall + GKE + SQL + GCS + VPC across GCP, then AWS-RDS + SCW-RDB + SCW-LB).

The protocol now generalizes across all three cloud providers — strong evidence the N10→N11 architectural shift is cloud-agnostic.

## 2026-06-02 S59 N11 retirement close-out

S59 retired GCP phase2 rule 10 (VPC + subnetwork) — the highest-stakes GCP retirement (affects nearly every networked scenario).

- Step 3: deleted phase2 rule 10 + phase3 rule 5.
- Step 5: re-ran three VPC-heavy scenarios:
  - `gcp-vm-network` → target_reached iter 1. Generated HCL has explicit `google_compute_network` + `google_compute_subnetwork` with `auto_create_subnetworks = false`, and `network_interface { subnetwork = google_compute_subnetwork.main.id }` on the instance.
  - `gcp-iam` → target_reached iter 1. Same VPC + service-account wiring pattern.
  - `gcp-cloud-run` → **stuck** after 2 iters. BUT the failure was `deletion_policy = "DELETE"` on `google_cloud_run_v2_service` (LLM hallucinated an attribute — \"An argument named deletion_policy is not expected here\"). This is unrelated to VPC. The two-iter stuck-detection killed iteration before the auto-correction loop could resolve. Captured as follow-up.
- Step 5 path: the **two existing same-resource pitfalls** (`google_compute_instance` and `google_container_cluster`, both `source: learned`) already encode the VPC pattern — they were auto-learned in a prior session and carry the rule. Rule 10 is redundant given those pitfalls.

**Sixth N11 retirement** (CMEK + firewall + GKE + SQL + GCS + VPC). The highest-stakes single-rule retirement succeeded — confirms the protocol's "auto-learned pitfall replaces prompt rule" path works for cross-resource patterns affecting many scenarios.

**Follow-ups captured**:
- `gcp-cloud-run` flakiness on `deletion_policy` hallucination — the existing `google_cloud_run_v2_service` `deletion_protection` pitfall may be subtly mis-applied by the LLM. Worth a focused review.

## 2026-06-02 S58 N11 retirements close-out

S58 retired GCP phase2 rules 14 (Cloud SQL teardown + private IP) AND 15 (GCS test setup) in one PR via the 7-step protocol:

- Step 3 (delete prompt rules): removed both phase2 rules + the three matching phase3 checkpoints (rules 8 + 9 + 12).
- Step 5 (re-run):
  - `gcp-cloud-sql` → target_reached iter 2. Verified `deletion_protection = false`, `ipv4_enabled = false`, `private_network = google_compute_network.main.id`. Iter 1 failure was unrelated CMEK self-correction.
  - `gcp-storage` → target_reached iter 1. Verified `force_destroy = true` + `uniform_bucket_level_access = true`.
- Step 5 exit path: **rules redundant — delete with no follow-up** for both.

**Fourth + fifth N11 retirements** (CMEK + firewall + GKE + SQL + GCS). The protocol now generalizes to multi-failure-mode rules (Cloud SQL: destroy + region + policy; GCS: destroy + uniqueness + access mode). Strong evidence the auto-correction channel carries any rule with at least one machine-readable failure path per attribute.

## 2026-06-02 S57 N11 retirement close-out

S57 retired GCP phase2 rule 13 (GKE single-node-pool strategy) via the 7-step protocol:

- Step 1-2 (sweep + inspect pitfalls): no `learned_from_diff` for `google_container_cluster` or `google_container_node_pool` existed.
- Step 3 (delete prompt rule): removed rule 13 from `prompts/gcp/phase2_generate_hcl.md` and the matching phase3 self-review checkpoint (rule 10).
- Step 5 (re-run):
  - `gcp-gke-cluster` → target_reached on iter 4. The GKE config in iter 4 was correct (`remove_default_node_pool = true` + `initial_node_count = 1` + separate `google_container_node_pool`, no inline `node_config`). The 3 prior failed iters were CMEK self-correction on `google_storage_bucket.tfstate` (unrelated to rule 13).
  - `gcp-full-stack` → repair_budget_exhausted on iter 5. BUT iter 5's failure was `google_apikeys_key` mock gap (`apikeys.googleapis.com` not implemented by fakegcp), and the GKE shape in iter 5's HCL was IDENTICAL to gke-cluster's correct shape. The LLM non-deterministically introduced `google_apikeys_key` — a separate bug, NOT a regression caused by rule 13 retirement.
- Step 5 exit path hit: **"rule was redundant — delete with no follow-up."** Steps 6-7 skipped.

**Third N11 retirement** (CMEK + firewall + GKE). The protocol now generalizes to multi-attribute rules with cross-resource patterns (cluster + separate node_pool).

**Follow-up captured**: gcp-full-stack flakiness on `google_apikeys_key` is N12 territory — either implement the resource in fakegcp or improve the scenario architecture plan so the LLM doesn't reach for it.

## 2026-06-02 S56 N11 retirement close-out

S56 retired GCP phase2 rule 11 (firewall `network` vs `subnetwork` attribute) via the 7-step protocol:

- Step 1-2 (sweep + inspect pitfalls): no `learned_from_diff` for `google_compute_firewall` existed — the prompt rule kept the LLM correct, so no failure was ever recorded for N10 to learn from.
- Step 3 (delete prompt rule): removed rule 11 from `prompts/gcp/phase2_generate_hcl.md` and the matching phase3 self-review checkpoint (rule 6).
- Step 4 (blank matching pitfall): N/A — no matching entry.
- Step 5 (re-run): `gcp-vm-network` (the only firewall-dense scenario) → target_reached on iter 1. LLM produced correct `network = google_compute_network.main.id` with no `subnetwork =`, narrow `source_ranges`.
- Step 5 exit path hit: **"rule was redundant — delete with no follow-up."** Steps 6-7 skipped.

**Second N11 retirement** (CMEK was first). Confirms the 7-step protocol generalizes to single-attribute correction rules where `tofu validate` provides strong machine-readable feedback. The architectural shift holds: well-typed validator errors flow through the dynamic auto-correction channel without any prompt or pitfall scaffolding.

## 2026-06-02 S55 N10 audit close-out

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

---

## 2026-05-30 → 2026-06-02 pre-arc session narrative (archived)

Below is the accumulated prior-session content from NEXT_SESSION.md prior to the S62 close-out — the auto-learning sweep narratives, the N1–N14 ticket dispatch, the orphan / classifier / policy-pitfall design notes. Kept verbatim for traceability; superseded by ADR-0015/0016/0017/0018 + the closed S54–S62 slice plan.

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

---

## STATUS.md — historical "Current phase" + "Recent updates" (archived 2026-06-02)

When STATUS.md was trimmed for the S63 prep, the rolling narrative under § Current phase (S55–S62 entries) and the full § Recent updates section (S33–S53 close-outs + auto-learning design notes) moved here. Kept verbatim for traceability.

### § Current phase (pre-trim, entries beyond S62)

- **S58 N11 retirements — GCP phase2 rules 14 (Cloud SQL teardown + private IP) + 15 (GCS test setup) both retired as redundant**. One PR landing both. Followed the 7-step protocol: deleted both phase2 rules and the three matching phase3 checkpoints (rules 8 + 9 + 12). Re-ran `gcp-cloud-sql` → target_reached iter 2 (LLM produced `deletion_protection = false`, `ipv4_enabled = false`, `private_network = google_compute_network.main.id` correctly; iter 1's failure was unrelated CMEK self-correction); `gcp-storage` → target_reached iter 1 (LLM produced `force_destroy = true` + `uniform_bucket_level_access = true` correctly). Per protocol step 5 exit path: rules were redundant — delete with no follow-up. Fourth and fifth N11 retirements (CMEK + firewall + GKE + SQL + GCS).
- **S57 N11 retirement — GCP phase2 rule 13 (GKE single-node-pool) retired as redundant**. Followed the 7-step protocol against rule 13 (phase2) + rule 10 (phase3): re-ran `gcp-gke-cluster` (4 iters → target_reached, GKE shape correct: `remove_default_node_pool = true` + `initial_node_count = 1` + separate `google_container_node_pool` with no inline `node_config`) and `gcp-full-stack` (repair_budget_exhausted on iter 5, but the failure was a `google_apikeys_key` mock gap unrelated to GKE — the GKE shape in iter 5 was correct, identical to gke-cluster). Per protocol step 5 exit path: "rule was redundant — delete with no follow-up." gcp-full-stack flakiness captured as a separate follow-up (LLM non-deterministically introduces `google_apikeys_key` which fakegcp doesn't support — likely N12 territory). Third N11 retirement (CMEK + firewall + GKE).
- **S56 N11 retirement — GCP phase2 rule 11 (firewall network-vs-subnetwork) retired as redundant**. Followed the 7-step protocol: deleted the rule from `prompts/gcp/phase2_generate_hcl.md` + the equivalent phase3 review checkpoint (rule 6) + re-ran `gcp-vm-network` (the firewall-dense scenario). Result: target_reached on iter 1, LLM produced correct firewall HCL (`network = google_compute_network.main.id`, no `subnetwork = ...` attribute, narrow `source_ranges`) without any prompt or pitfall guidance. Per protocol step 5: "rule was redundant — delete with no follow-up." Second N11 retirement total (CMEK was first). Confirms the protocol generalizes beyond CMEK — single-attribute correction rules with strong machine-readable validation feedback (here, `tofu validate` rejecting `subnetwork` as "Unsupported argument") can retire without pitfall replacement.
- **S55 N10 audit — 3 wiring/quality fixes landed**: (1) `iterationHistory` was only appended on failed iterations, so a "1 fail → 1 pass" run had len=1 and the N10 extractor's `len > 1` guard skipped — only 3+ iter scenarios ever fired. Fix: also append the passing iteration with empty failures. Production validation: `gcp-storage` now fires `prescriptive_pitfall_learned` on iter 1→2; previously silent. (2) `trimSnippet` cut at the last newline, leaving unbalanced blocks like `depends_on = [` mid-list. Fix: prefer cutting after a column-0 `}` (top-level resource close). New test `TestExtractPrescriptiveFix_SnippetTrimAtBlockBoundary` pins the boundary preference. (3) S55-T3 ratchet `TestPitfallsLearnedFromDiffSnippetCap` asserts on-disk learned_from_diff rules stay under `snippetMaxBytes + 400` bytes; whitelisted `learned_from_diff` alongside `learned` in M91. Production audit produced clean entries for `google_storage_bucket`, `google_sql_database_instance`, and `scaleway_domain_record` — all under 600-byte snippet cap, all type-attributed correctly (storage via direct address; SQL via type-hint fallback; domain record via direct address).
- **S54 sweep result — 39/39 deterministic baseline** (one regression triaged + retried). Full 39-scenario sweep ran with mocks reset between scenarios: 38 passed first attempt + `aws-full-stack` failed with `BucketAlreadyExists` on S3 across all 5 iterations. Root cause: pre-existing SeaweedFS state from a manual session before sweep start — once cleared, aws-full-stack passes iter 1. fakeaws's `/mock/reset` does not cascade to SeaweedFS (the s3 carve-out only cascades via `cloudMockStateRouter.Reset` inside `infrafactory run`), so any sweep harness that calls bare curls to `/mock/reset` must also drop SeaweedFS buckets independently. Captured in NEXT_SESSION as a sustain-ratchet improvement; current run-time `auto_reset` chain is correct.
- **N10 dedupe — learned → learned_from_diff upgrade**: the S54 sweep emitted a `prescriptive_pitfall_learned` event for `google_storage_bucket` (from gcp-full-stack iter 1→2) but the entry did NOT land in `pitfalls/gcp.yaml`. Root cause: the existing same-resource `source: learned` symptom-only entry (from gcp-storage) shared 3+ significant words with the N10 candidate, so `isDuplicate` silently dropped the new entry. The verbatim→prescriptive upgrade path covered only `isVerbatimFallback` predecessors. Fix: parallel upgrade path — when a `PrescriptiveSource` candidate sees any same-resource non-prescriptive entry, REPLACE in place. Two new unit tests pin both the upgrade and the prescriptive-vs-prescriptive dedupe (latter still skips duplicates to prevent per-iter-pair bloat).
- Slices 1-32 complete. 12 Scaleway training scenarios pass `infrafactory run`.
- Slices 33-39 fully complete (cross-repo e2e infrastructure, oscillation pitfall learning, http_probe diagnostics, pitfalls API + UI, run compare API + UI, real-time scenario validation).
- **Slices 33-42 (GCP critical path) now complete**: S40 (visual regression Playwright suite), S41 (fakegcp test parity to mockway level — 881 lines repo tests + 17 FK violation handler tests + 6 cascade delete tests + admin /mock/state tests + double-apply idempotency harness + 5 misconfigured TF examples), S42 (multi-cloud UI with cloud badge + dynamic Layer 3 label + mock-provider dispatch), S36 (cross-repo TestE2E_GCPDoubleApplyIdempotency + GCP scenarios surfaced in UI Playwright), M38 (Google provider auto-injection). 17 ticket batch closed 2026-05-23.
- **Slices 43-48 (fakeaws) complete**: 9 AWS services landed across 5 wire formats (IAM, S3, EC2, RDS, DynamoDB, EKS, SQS, Route53, Secrets Manager); aggregate handler coverage 82.4%; S48-T4 codex review loop closed at pass 17 (passes 1-15 each fixed ≥1 BLOCKING; passes 16-17 returned consecutive NOTHING_TO_IMPROVE). Pass-by-pass history archived under `../fakeaws/docs/review-passes/passN.md`.
- **2026-06-01 sweep close-out — realistic pass rate 39/39**: closed every outstanding training scenario via 18 named tickets across infrafactory + fakeaws + fakegcp + mockway. ADR-0014 captures the provider-endpoint-flag discipline learned this session (v1/v3 distinct flags, host-only default, dual-prefix mock routes). 22 infrafactory commits, 7 fakeaws, 6 fakegcp, 1 mockway. See `docs/NEXT_SESSION.md` for the per-ticket breakdown and remaining N1-N7 polish candidates.
- **2026-06-01 N1 sweep + policy fix**: the first full deterministic sweep after the 39/39 one-off claim landed at **36/39** (web-app-paris, compute-lb-multi-paris, aws-full-stack failed). Investigation showed 2 of 3 failures were a real bug in `policies/scaleway/vpc_required.rego` (count-based `scaleway_instance_server` + matching count-based NIC was falsely flagged because the policy compared planned `[N]`-indexed addresses literally against configuration's symbolic references). Fixed + regression-tested. Expected post-fix deterministic rate: **38/39**. The remaining `aws-full-stack` failure is genuine LLM non-determinism on the Secrets Manager soft-delete pattern (closes ~50% of the time; candidate for the N3 orphan_check extractor if it recurs).
- **2026-06-01 N3 / T12 classifier landed**: `IsMockActionable` predicate + `AppendMockGap` writer + ratchet test `TestPitfallsNoMockActionableSeeds` enforce the long-stated "mock quirks are tickets, not pitfalls" principle. Failures matching 501 / Plugin-did-not-respond / OAuth-escape / Describe*-404 patterns now route to `docs/mock-gaps.md` instead of `pitfalls/<cloud>.yaml`. 5 stale gcp entries pruned in the same commit (caught by the new ratchet). ADR-0015 captures the classifier-routing pattern. Closes the spec drafted as Ticket 12 in the original NEXT_SESSION.md.
- **2026-06-01 N2 prune of stale verbatim mock-quirk pitfalls**: 11 entries removed from pitfalls/aws.yaml + pitfalls/gcp.yaml (2 aws + 9 gcp) whose underlying mock-server bugs were fixed earlier in the session (T-C, T-D-2, T-E, T-E-2, the recovery commits). All entries either had verbatim stderr dumps for the fixed bug or descriptive "escaping" / "not implemented" rules that no longer apply. Kept entries are real provider/cloud constraints (e.g. google_compute_instance VPC requirement, google_storage_bucket CMEK, aws_db_instance encryption) or tracked-but-unfixed (aws_subnet MapPublicIpOnLaunch / T8). Pre-prune AWS=6/GCP=13 → Post-prune AWS=4/GCP=4. Companion to N3's classifier-driven prune (5 entries earlier this session).
- **2026-06-01 N8 policy_pitfall_conflict detector landed**: `DetectPolicyConflict` extracts backticked keywords from same-resource prescriptive pitfalls, compares to the LLM's iteration HCL, and emits a `PolicyGap` to `docs/policy-gaps.md` when all keywords match AND the policy still fired (motivating case: 2026-06-01 deterministic sweep, web-app-paris + compute-lb-multi-paris). The auto-learning loop now routes three distinct non-LLM failure categories to dedicated channels — N3 → mock-gaps, N8 → policy-gaps, N9 → mixed (sub-shape #1 LLM, #2-5 mock). ADR-0017 documents the detection contract.
- **2026-06-01 N9 orphan_check extractor landed**: `ClassifyOrphans` reads the live `/mock/state` JSON after a stuck-on-orphan termination, walks lingering resource collections, and classifies each across 5 sub-shapes (LLM soft-delete, mock auto-seed, mock CASCADE, mock-vs-provider divergence, provider soft-delete). Sub-shape #1 emits a prescriptive `aws_secretsmanager_secret`/`aws_kms_key`/`google_secret_manager_secret` pitfall; sub-shapes #2-5 route to `docs/mock-gaps.md`. Seeded with 6 known (cloud, service, collection) entries from this session's sweeps; extensible. ADR-0016 captures the sub-shape table. Closes the system's only previously-opaque validation-layer failure mode.
- **2026-06-02 end-to-end sweep validation surfaced 3 classifier/policy bugs (all fixed in one PR)**: (1) N3 `IsMockActionable` was wired only into the self-correction path, not the stuck/budget path, so 4 GCP scenarios re-learned pruned OAuth-escape pitfalls. (2) N8 `DetectPolicyConflict` regex hunted for `policy=X.Y` in `f.Detail`, but `Policy` is a structured `FailureSummary`/`feedback.Failure` field — every real policy failure was missed. (3) PR #8's `vpc_required.rego` count fix matched only the singleton `X.id` reference shape; count-based NIC expressions store references as `["X", "count.index"]` (bare ref, no `.id`), so 4 Scaleway scenarios still hit the rego false positive. All three bugs validated against the recorded sweep plan (0 failures on real `compute-lb-multi-paris` plan post-fix). See ADR-0015 + ADR-0017 amendments.
- **2026-06-02 GCP auth-pipeline discipline (ADR-0014 rule 4)**: gcp-cloud-run, gcp-cloud-sql, gcp-gke-cluster, gcp-storage all hit `401 ACCESS_TOKEN_TYPE_UNSUPPORTED` against what *looked* like `cloudresourcemanager.googleapis.com` despite every `*_custom_endpoint` being correctly set and fakegcp serving the route. Root cause: the v5 SDK's auth pipeline bypasses the `access_token` short-circuit when `user_project_override` defaults to true OR the parent process has any of `GOOGLE_APPLICATION_CREDENTIALS`, `GOOGLE_CREDENTIALS`, `GOOGLE_OAUTH_ACCESS_TOKEN`, `GOOGLE_CLOUD_KEYFILE_JSON`, `CLOUDSDK_*`, or `GCLOUD_*` env vars set — the SDK probes the metadata server / token endpoint before issuing the actual API call. Fix: (a) `user_project_override = false` in the Google provider template (`generate_command.go::buildGoogleProviderBlock`), (b) `stripGCPAuthEnv` strips the env-var families at the tofu subprocess boundary (`internal/cli/exec_runner.go`). Initial draft also added a `credentials` JSON stub but the v5 provider rejects HCL setting both `credentials` and `access_token` with a fatal `Invalid Attribute Combination` error; attribute removed in a follow-up commit.
- **2026-06-02 GCP prompts: omit `google_project_service` + `google_service_networking_connection`**: end-to-end re-validation post auth-pipeline fixes showed those two resources still trigger the preflight escape regardless of provider config. They're meta-resources (API enablement, Private Service Access setup) that the LLM was previously instructed to include by the GCP prompts — but for the fakegcp validation target both are unnecessary (every API is implicitly served; private endpoints synthesised). Updated `prompts/gcp/phase1`, `phase2`, `phase3` to instruct the LLM to omit them. Same fix-at-source discipline as `feedback_mock_design.md`.
- **2026-06-02 N10 diff-based prescriptive-pitfall extractor landed**: `ExtractPrescriptiveFix` in `internal/generator/prescriptive_extractor.go` walks adjacent failing→passing iteration pairs after a run reaches `target_reached`, diffs the failing iteration's HCL against the passing iteration's HCL, scopes the diff to the failing resource + new sibling resources referenced from its new attributes, and emits a `LearnedPitfall{Source: PrescriptiveSource}` containing the minimal HCL snippet that resolved the failure. Closes the symptom-only gap in `ExtractLearnedPitfall`: today the system learns WHAT failed; with N10 it also learns the HCL pattern that FIXED it. Wired into `run_command.go` after the existing stuck/budget classifier hooks. Conservative-by-design — returns nil when the failing block didn't change, ignoring whitespace-only diffs and addresses missing from either iteration. Unblocks N11 (retire prescriptive prompt rules 9–16). See `docs/NEXT_SESSION.md` § N10 for the design rationale.
- **2026-06-02 N10 production-validation fixes**: the first real sweep against N10 (`gcp-cloud-sql` iter 1→iter 2 `target_reached`) emitted zero `learned_from_diff` entries because two extractor bugs blocked attribution. (1) State-side OPA `deny_state` rules emit details like `Cloud SQL instance NAME missing diskEncryptionConfiguration.kmsKeyName` — no terraform address — so `firstResourceAddress` returned empty and the extractor returned nil. Added `inferResourceTypeFromDetail` hint table (`Cloud SQL instance` → `google_sql_database_instance`, `storage bucket` → `google_storage_bucket`, etc.) + `changedResourcesOfType` fallback that scopes the diff to resources of the inferred type whose body changed (abstains on ambiguity — exactly-one-match rule). (2) The LLM commonly renames resources between iterations (e.g. `.postgres` → `.pg`); the strict-address lookup paired no bodies and returned nil. Fix: when the address is only in passing (rename or pure-add), treat the entire passing body as the addition. Validated against the recorded sweep — extractor now emits the CMEK fix snippet for SQL.
- **2026-06-02 fakegcp gap-clearing batch (PRs #4-#9 against fakegcp)**: Closed five distinct v5-provider-deref / shape gaps surfaced by re-validating the 4 originally-failing GCP scenarios after N10 landed. (#4 KMS cryptoKeyVersions list/get/:destroy routes + KMS IAM policy round-trip — unblocks CMEK destroy + google_kms_crypto_key_iam_member. #5 NodePool server-side defaults — management, upgradeSettings, maxPodsConstraint, podIpv4CidrSize, config.{metadata,labels,tags,oauthScopes,shieldedInstanceConfig,workloadMetadataConfig}, networkConfig. #6 SQL + Compute server-side defaults — settings.{activationPolicy,backupConfiguration,ipConfiguration,maintenanceWindow,...} + networkInterfaces[].fingerprint, metadata.fingerprint, tags.fingerprint, scheduling.*, shieldedInstanceConfig, disks[].kind/mode/boot. #7 IAM auditConfigs + ServiceAccount oauth2ClientId/disabled/etag — unblocks google_project_iam_member's post-Apply read consistency check + iam_member's principal-resolution deref. #8 NodePool int64,string-tagged fields emit as JSON strings; #9 reverts diskSizeGb back to JSON number (PR #8 over-corrected — only maxPodsPerNode is int64,string; diskSizeGb is plain int64). Each PR has 1-7 round-trip tests pinning the wire shape.
- **2026-06-02 google provider: disable global IAM batching**: gcp-full-stack's google_project_iam_member.* escaped to real cloudresourcemanager.googleapis.com with ACCESS_TOKEN_TYPE_UNSUPPORTED even though every other IAM / ResourceManager call landed cleanly on fakegcp. Root cause: the v5 provider's BatchingConfig wrapper aggregates iam_member writes and constructs its OWN cloudresourcemanager client — that client does not honor cloud_resource_manager_custom_endpoint. Fix: `batching { enable_batching = false }` in the Google provider block. Each IAM mutation now goes through the normal client path that respects the endpoint override. (PR #23)
- **2026-06-02 N11 first prompt-rule retirement — CMEK rule (rule 16 + phase3 rule 13)**: All 9 originally-failing GCP scenarios now pass deterministically post N12 fakegcp gap-clearing + N10 attribution fixes. Executed the 7-step N11 validation protocol on the CMEK rule (`encryption.default_kms_key_name` / `encryption_key_name` / `disk_encryption_key.kms_key_self_link`). After deleting the rule from `prompts/gcp/phase2_generate_hcl.md` + `phase3_self_review.md` AND removing the matching `google_sql_database_instance` `learned_from_diff` pitfall, `gcp-cloud-sql` still closed `target_reached` on iter 2 (iter 1 failed on missing CMEK, iter 2 self-corrected from the failure-detail feedback alone). Per the protocol — "If step 5 doesn't show a regression, the prompt rule was redundant — delete with no follow-up." Validates that the LLM's self-correction loop carries CMEK without prompt or static pitfall support; one fewer hand-written prescriptive rule. First production proof of the N10→N11 architectural shift: prescriptive guidance moves from hand-written prose to a living artifact (in this case, the auto-feedback channel itself proved sufficient).
- ADRs 0009 (incremental deployment) and 0010 (Layer 3, supersedes ADR-0003) are implemented in code and docs.
- 22 implementation contracts codified in CONCEPT.md § "Implementation Contracts (Slices 22-29)".

### § Recent updates (pre-trim)

## Recent updates
- **Slices 33-42 closeout (2026-05-23, 17 tickets)**:
  - **S41** (fakegcp test parity): T1 testutil snapshot/restore helpers + `test-e2e` Make target; T2 repository_test.go 881 lines, 27 funcs, 41.6% pkg coverage over 15 named tables; T3 fk_violation_test.go 17 HTTP-layer 404 tests; T4 cascade_delete_test.go 6 tests; T5 admin /mock/state + /mock/state/{service} tests; T6 scripts/e2e.sh 163-line double-apply harness; T7 5 misconfigured TF examples (incl. fix of misnamed pre-existing `instance_missing_network`). Commits: fakegcp@45e5402..556da4b.
  - **S42** (multi-cloud UI): T2 cloud-pill badge, dynamic "Layer 3 (Real {Cloud})" label, layer3-status backend dispatches required env vars by cloud (SCW_*/GOOGLE_*/AWS_*); T3 `serverState.mockStateForCloud` dispatcher, `FakegcpState` wired through ServerConfig with mockway fallback for unconfigured fakegcp, run-mode exposes `mock_provider`; T5 ui/e2e/multi-cloud.spec.ts (4 tests).
  - **S40** (visual regression): T1+T2 ui/e2e/visual.spec.ts captures 7 baselines with `toHaveScreenshot` (maxDiffPixelRatio 0.02, threshold 0.2) and masks volatile chrome (session id, start time, textarea body); T3+T4 ui/e2e/spot-checks.spec.ts adds functional spot-checks across pages + error-state coverage (unknown SPA route, missing scenario, /api/scenarios/missing + /layer3-status both 404).
  - **S36-T11**: TestE2E_GCPDoubleApplyIdempotency in internal/e2e — runs `infrafactory run --no-destroy` twice on gcp-pubsub.yaml; asserts mock state counts + per-resource identities unchanged (catches silent delete-recreate churn).
  - **S36-T12**: covered by multi-cloud.spec.ts (asserts ≥1 gcp-* training scenario in the GCP sidebar group).
  - **M38**: `ensureGoogleProviderWiring` + `validateGoogleProviderWiring` in internal/cli/generate_command.go mirror the Scaleway helpers; 4 new tests.
  - **Pre-existing bug surfaced (resolved 2026-05-25 by M74)**: fakegcp's working examples previously used `cloud_sql_custom_endpoint` which was rejected by google provider v7. Fixed at fakegcp@b617d2f — all 6 provider blocks now use the correct `sql_custom_endpoint` name; documented in fakegcp/CHANGELOG.md.
- **API breaking change** (review-1 → review-3 hardening): `POST /api/scenarios/validate` now strictly requires `Content-Type: application/json` (previously accepted empty Content-Type as JSON) and rejects unknown JSON fields (previously silently dropped). External clients should set the header explicitly and limit the body to `{"yaml": "..."}`. Internal UI usage already complies.
- **fakegcp Reset bug fixed** (review-4): `repository.Repository.Reset()` now removes the `*.snapshot` baseline file alongside the table truncation. Previously, `infrafactory run --clean` against fakegcp would silently retain the previous run's snapshot baseline, so a follow-up Restore could resurrect it. Mirrors mockway's contract.
- **S42-T1 + S42-T4 complete (sidebar grouped by cloud + API cloud field)**:
  - `GET /api/scenarios` now returns `cloud` per scenario item (extracted from the loaded YAML during the directory walk).
  - Sidebar layout regroups scenarios client-side by cloud: SCALEWAY first, then GCP, then OTHER for unknowns. Stable test ids: `sidebar-cloud-{cloud}`, `sidebar-cloud-label`, `sidebar-scenario-{path}`.
  - Layer3-status per-cloud credential checks (S42-T4 second half) deferred — current `/api/scenarios/{path}/layer3-status` already adapts at runtime, but exposing a richer per-cloud schema lands with S42-T2 (scenario-page badge + dynamic credentials section).
- **S38-T3 + S39-T4 complete (Playwright e2e for compare and validation)**:
  - `ui/e2e/compare.spec.ts` (4 tests): sidebar nav → /compare, &lt;2-runs disables Compare button, two-run compare renders file list with status badges, switching files updates the diff pane. Uses `findComparableScenario` precondition probe → tests skip cleanly on a fresh checkout with no `.infrafactory/runs/` data.
  - `ui/e2e/validation.spec.ts` (4 tests): seeded scenario shows valid after debounce, `cloud: scaleway → aws` flips to errors, garbage YAML surfaces `yaml syntax`, restoring a valid edit returns to green. Each test restores original textarea content in `finally`.
  - Playwright suite: 24 passed, 7 skipped. Slices 37-39 fully closed for the UI side.
- **S41-T1 partial (fakegcp test infrastructure)**:
  - In the sibling `../fakegcp/` working tree: added `test-race`, `test-short`, `test-coverage` Makefile targets. `test-coverage` writes `coverage.out` and emits an HTML report; current handlers package coverage is **63.7%** of statements. Existing `testutil/testutil.go` is the helper set; further coverage targets land with S41-T2 (repository unit tests).
  - Not committed in fakegcp because that repo has no git history yet — `S41-T0` (git init + first commit + push) is the gate, owned by the user (avoiding unauthorised remote pushes).
- **S36-T0 verified done (fakegcp prerequisites)**:
  - Confirmed `../fakegcp/AGENTS.md` already covers the required Architecture, Testing, and Conventions sections (plus extras like "GCP API Conventions", "API Fidelity Principles", "Checklist for New Handlers", "Safe Workflow"). No update needed.
  - Confirmed `google_compute_forwarding_rule` handler exists in `../fakegcp/handlers/loadbalancer.go` with Create/Get/List/Delete plus idempotency on the Terraform provider's read-then-apply path. CRUD is sufficient since Terraform's apply loop owns the idempotency contract.
  - Real fakegcp git initialization + push remains as **S41-T0** (separate ticket, in the sibling repo).
- **S37-T4 complete (Playwright e2e for pitfalls)**:
  - 5 new Playwright tests in `ui/e2e/pitfalls.spec.ts` cover page load via sidebar, default-tab selection, tab switch updating row count, edit + save with reload-and-restore for idempotent re-runs, and add/delete row dance. Total Playwright suite now 23 passing (up from 18).
  - Bundled into commit 2392eb5 (named for S39-T2/T3) due to the same sub-agent worktree merge race; tracked here for clarity.
- **S39-T2 + S39-T3 complete (debounced validation + inline errors)**:
  - Scenario page now validates the YAML textarea on every change with a 500ms debounce. New `api.validateScenarioYAML(yaml)` calls the existing `POST /api/scenarios/validate` endpoint. Race protection via a `validationVersion` counter so an in-flight request from an earlier keystroke can't overwrite a later result.
  - Inline UI shows "Validating…" while the request is in flight, "Valid scenario." in green when valid, and a red bulleted list `path: message` for each violation. Stable test ids: `scenario-yaml`, `scenario-validation`, `scenario-validation-{checking,valid,errors}`.
- **S38-T2 complete (/compare UI page)**:
  - New SvelteKit route at `/compare` with scenario + run-1 + run-2 selectors, a Compare button, a left-side file list with status badges (`added`/`removed`/`modified`/`unchanged`), and a right-pane unified-diff viewer. Uses the existing `GET /api/runs/{scenario}/compare` endpoint via the new `api.compareRuns()` helper. Default selection picks the two most recent runs of the chosen scenario.
  - Sidebar gains a "Compare" entry between Live and Pitfalls. Stable test ids: `compare-section`, `compare-scenario`, `compare-run1`, `compare-run2`, `compare-run`, `compare-files`, `compare-file-<name>`, `compare-status-<name>`, `compare-diff`, `compare-error`.
- **S37-T3 complete (/pitfalls UI page)**:
  - Added `/pitfalls` SvelteKit route with provider tabs (default = first alphabetically), an inline-editable table per provider (Resource, Rule, Source, Discovered From), `+ Add`/Delete row controls, and per-provider Save calling the existing `PUT /api/pitfalls/{provider}`. Source badges: `learned` → sky accent, `static`/`seed`/unknown → neutral slate. Sidebar gains a "Pitfalls" entry between Live and Diagnostics.
  - Helpers extracted to `ui/src/lib/pitfalls-{view,api}.js` so the fetch and view logic stay testable. 15 new node:test cases (8 view + 7 api). `npm test` 55 → 70+ passes; `npm run build` clean. Stable test ids documented in the agent report so S37-T4 Playwright tests can drive the page deterministically.
- **S36-T10 complete (GCP training scenarios)**:
  - Added `scenarios/training/gcp-{vm-network,gke-cluster,cloud-sql,full-stack}.yaml`. All four validate against the GCP-widened schema (regression test in `scenario_gcp_test.go::TestLoadGCPTrainingScenarios`).
  - Live `infrafactory run` parity against fakegcp is gated by S41-T1 (fakegcp test infrastructure in the sibling repo) and S36-T11 (cross-repo e2e); the scenario fixtures themselves are static and ship now.
- **S36-T5 complete (GCP topology unit tests)**:
  - Added 6 more `TestDeriveTopologyGCP*` cases on top of the 5 from S36-T4: multiple forwarding rules, port_range/ports-array variants, database-only without compute, MySQL port, GKE edge, and malformed JSON. Now 11 GCP-specific tests + 1 dispatch detection test (4 sub-cases) = 12 total, matching the Scaleway 10+ floor.
- **S36-T9 complete (real_probe.go GCP resource patterns)**:
  - `probeTargetResourceTypes` now returns mixed Scaleway + GCP resource type lists per probe target. `findHostForResourceType` already skips past types absent from live state, so a Scaleway scenario keeps resolving the same way and a GCP scenario picks up `google_compute_global_address`/`google_compute_forwarding_rule` (load_balancer), `google_sql_database_instance` (database), `google_redis_instance` (redis), `google_compute_instance` (compute), `google_container_cluster` (kubernetes).
  - `pickHost` learns six new GCP-specific attribute pattern lists (e.g. `network_interface.0.access_config.0.nat_ip` for compute, `endpoint` for GKE).
- **S36-T3 + S36-T4 complete (GCP prompts + topology derivation)**:
  - **S36-T3**: Added `prompts/gcp/phase{1,2,3}_*.md` mirroring the Scaleway prompts' placeholders ({{.ScenarioYAML}}, {{.Constraints}}, {{.ResolvedMappings}}, {{.Overrides}}, {{.ArchitecturePlan}}, {{.AcceptanceCriteria}}, {{.GeneratedFiles}}, {{.FeedbackJSON}}, {{.ProviderSchema}}, {{.Layer3Guidance}}, {{.Pitfalls}}). Content swaps Scaleway idioms for `hashicorp/google` source, `google_project_service` API enablement gating, no-default-VPC enforcement, fully-qualified `serviceAccount:` IAM principals, GKE separate-node-pool pattern, Cloud SQL `deletion_protection=false` for tests, GCS `force_destroy=true`, CMEK encryption, and the allowed-region list. Phase 3's self-review checklist tracks the four `policies/gcp/*.rego` constraints.
  - **S36-T4**: `DeriveTopology` auto-detects cloud from raw state shape (top-level `compute` → GCP, `instance` → Scaleway) and dispatches. `deriveTopologyGCP` covers `google_compute_instance` (compute connectivity), `google_compute_forwarding_rule` (http_probe + LB-level fallback diagnostic), `google_sql_database_instance` (compute → database connectivity, default-deny public), `google_container_cluster` (compute → kubernetes). Public ingress recognised only when an `INGRESS` firewall with `0.0.0.0/0` ALLOW exists. 6 unit tests cover dispatch detection (4 sub-cases), happy-path probe, no-backend probe, compute→database, public-ingress firewall, and default-deny.
- **S36-T8 complete (multi-cloud mock client)**:
  - Renamed `mockwayStateClient` → `mockStateClient` (and `newMockwayStateClient` → `newMockStateClient`) since `/mock/state`, `/mock/reset`, `/mock/snapshot`, `/mock/restore` are identical between mockway and fakegcp. Same client serves either backend; callers pick the URL by cloud. Added a doc comment recording the multi-cloud contract.
- **S36-T1 complete (prompts reorganization)**:
  - Moved `prompts/phase{1,2,3}_*.md` → `prompts/scaleway/`. Added `resolvePromptTemplatePath(promptsDir, cloud, fileName)` in `internal/generator/prompts.go` — prefers the cloud-specific subdirectory when `req.Cloud` is set and a matching template exists, falls back to the legacy flat layout (preserves existing test fixtures that write phase files directly into a temp `promptsDir`). Both `claude_adapter` and `openrouter_adapter` now route phase template loads through this resolver.
  - 4 new sub-tests cover cloud-specific exists, cloud-specific missing fallback, empty-cloud legacy, and the all-missing legacy-path return.
- **S39-T1 complete (POST /api/scenarios/validate)**:
  - Added `validateScenarioHandler` returning `{"valid":..., "errors":[{"path","message"}]}` for any input. Schema-invalid YAML returns `200` with `valid: false` and per-violation entries; YAML syntax errors return `200` with a `yaml syntax: ...` message; empty body → 400; wrong method → 405; wrong content-type → 415.
  - Added `scenario.ValidateBytes(payload, schemaPath, sourceLabel)` that operates on bytes (no temp file) and shares `parseAndValidate` with `LoadWithSchema`. `Violation` now has `json:"path"`/`json:"message"` tags.
  - Routed at exact path `/api/scenarios/validate` so it sits cleanly next to the existing `/api/scenarios/{path}` PUT handler. 7 new tests cover valid, schema-invalid, missing-required-field, syntax-error, empty-body, wrong-method, wrong-content-type.
- **S37-T2 complete (PUT /api/pitfalls/{provider})**:
  - Refactored `handlers_pitfalls.go` into a single `pitfallsHandler` dispatcher: `GET /api/pitfalls` lists, `PUT /api/pitfalls/{provider}` writes the provider's YAML file atomically (.tmp + rename). Body is `{"pitfalls": [...]}`. Validation rejects empty resource/rule (422), unknown fields (400), traversal in provider name (400), missing pitfalls dir (424), and non-PUT methods (405).
  - Default `source: static` when omitted on a write. 4 new tests cover write success, validation, traversal rejection, and method rejection.
- **S38-T1 complete (GET /api/runs/{scenario}/compare)**:
  - Added `internal/api/handlers_runs_compare.go` returning file-level diffs between two runs of a scenario. Each entry has `filename`, `status` (`added`/`removed`/`modified`/`unchanged`), and `unified_diff` text (3 lines context). Empty diff for unchanged files. Validates run IDs (no path traversal), 400 on missing query params, 404 when either run's generated/ snapshot is absent.
  - Added direct dep `github.com/pmezard/go-difflib`.
  - 4 unit tests cover all four file statuses, validation, missing-run, and traversal cases.
- **S36-T2 complete (scenario schema GCP enum)**:
  - Widened `scenario.schema.json` `cloud` enum from `[scaleway]` to `[scaleway, gcp]`. Added a `storage` resource type (`purpose`, `size`) for GCS buckets. Existing compute/networking/database/kubernetes/redis/iam shapes are generic enough; no per-cloud branching.
  - Added `internal/scenario/scenario_gcp_test.go` covering a full GCP scenario validates and `cloud: aws` is rejected.
- **S36-T6 + S36-T7 complete (GCP pitfalls + OPA policies)**:
  - `pitfalls/gcp.yaml` seeded with 8 entries covering VPC/subnetwork prerequisites, required-API enablement, IAM principal format, GKE node pool strategy, Cloud SQL deletion protection / name reservation, GCS bucket naming, and firewall scoping.
  - `policies/gcp/{no_public_sql,vpc_required,region_restriction,encryption}.rego` — OPA bundle mirroring the Scaleway shape (`import rego.v1`, `deny contains msg if {...}`). `region_restriction.rego` reads `data.region_allowlist` with a `["us-central1","europe-west1","europe-west4"]` default. Wiring into `infrafactory.yaml`'s `policy_paths` and `constraint_policies` is deferred to a later S36 ticket.
- **Slice 37-T1 complete (GET /api/pitfalls)**:
  - Added `internal/api/handlers_pitfalls.go` — scans the configured pitfalls dir for `*.yaml`/`*.yml`, parses each as `generator.PitfallsFile`, and returns providers grouped alphabetically with deterministic entries (`resource`, `rule`, `source`, `discovered_from`).
  - Empty/missing directory → 200 with empty providers array. Malformed YAML → 500 with parse-error message. Non-GET → 405. 5 unit tests cover these branches.
- **Slice 35 complete (S35-T2, S35-T3)**:
  - `EvaluateTopology` now appends the diagnostic to http_probe failure detail with `": "`, e.g. `http probe "load_balancer:80" expected true got false: no backend attached`. Pre-computed-topology callers leave diagnostics nil and behave unchanged.
  - Plumbing is internal: `EvaluateTopology` captures `DeriveTopology`'s second return into a local map and an unexported `httpProbeDiagnostic` helper does the exact-key → LB-fallback → empty lookup. No exported signatures changed.
  - 5 new tests in `internal/harness/topology_evaluate_test.go`: exact-key diagnostic, LB-fallback diagnostic, no-diagnostic-keeps-bare-message, healthy-probe-no-detail, pre-computed-topology-no-diagnostic.
  - These changes were bundled into commit `eb6c7b9` (named for S34-T3) due to a sub-agent worktree merge race during the pre-commit hook; tracked here for clarity.
- **Slice 34-T3 complete (oscillation learning tests)**:
  - Added `TestRunCommandLearnsPitfallFromOscillation` driving the run loop with an alternating-error static harness for 4 iterations (A, B, A, B). Asserts `repair_budget_exhausted` and that the K8s detail (extractable) is appended to `pitfalls/scaleway.yaml` while the generic detail (B) is correctly skipped.
  - Added `TestRunCommandSkipsOscillationLearningWhenNoOscillation` confirming sustained linear failure → `stuck` terminal reason → no pitfalls file written.
  - Closes Slice 34.
- **Slice 35-T1 complete (http_probe diagnostics)**:
  - `DeriveTopology` now returns `(jsonBytes, diagnostics map[string]string, error)`. Diagnostics are keyed per http_probe entry (e.g. `load_balancer:80`) plus an `load_balancer` LB-level fallback for cases where the probe key doesn't exist (no frontend on requested port). Strings are short, lowercase, and factual ("no backend attached", "no public ip on lb", "frontends on port 443"). Existing JSON output and consumers are unchanged; only `internal/harness/topology.go:28` needed a compile-fix to ignore the new return.
  - 6 new diagnostic tests cover no-backend, no-public-IP, both, no-frontend-on-port, frontend-on-different-port, and a healthy-probe sanity check.
  - S35-T2 will surface these into evaluation failure messages.
- **Slice 34-T2 complete (oscillation pitfall wiring)**:
  - Run loop now accumulates `feedback.IterationResult` per iteration. When the run terminates with `repair_budget_exhausted` or `stuck`, it calls `feedback.DetectOscillation` and feeds each oscillating signature's detail through `generator.ExtractLearnedPitfall` + `generator.AppendPitfall`. Successful learns log `oscillation_pitfall_learned`; append errors log `oscillation_pitfall_append`.
- **Slice 34-T1 complete (oscillation detection)**:
  - Added `DetectOscillation(history []IterationResult) []FailureSignature` in `internal/feedback/oscillation.go`. Returns signatures that follow the pattern present-at-N, absent-at-N+1, present-at-N+2, capturing alternating-fix loops the model gets into. Result is deterministically sorted; gaps longer than one iteration intentionally don't count.
  - 7 unit tests covering: short history, simple oscillation, sustained failure, distinct failures, multi-signature oscillation, longer gap (negative), embedded oscillation in longer history.
  - Detector is unwired so far (S34-T2 will plumb it into the run loop and pitfall extraction).
- **Slice 33 complete (cross-repo e2e tests)**:
  - **S33-T3**: Added `TestE2E_FullStackParis` exercising compute + VPC + RDB + Kubernetes + Redis + container registry + IAM in one run. Uses two-stage flow (`--no-destroy` then incremental destroy) to assert resources land in mockway state and that the destroy pass also reaches `target_reached`. Runtime ~7.5s gated.
- **Slice 33-T2 complete (e2e for web-app-paris)**:
  - Added `TestE2E_WebAppParis` in `internal/e2e/web_app_paris_test.go` that runs the canonical web-app-paris training scenario against a freshly-started mockway with pre-baked HCL via the stub generator. Verified `target_reached` and all topology + policy criteria pass (4.5s wall time).
  - Extended `WriteConfig` to render absolute repo paths for `policies/`, `mappings.yaml`, `prompts/`, and `pitfalls/` so policy criteria evaluate end-to-end.
  - Added `RepoRoot` helper for fixture path resolution.
- **Slice 33-T1 complete (cross-repo e2e infrastructure)**:
  - Added `internal/e2e` package with `StartMockway`, `MockwayInstance` (Reset/FetchState/Stop), and `RunInfrafactory` helpers that drive the CLI in-process.
  - Added `cli.WithRuntimeDependencies(RuntimeDependencies)` exported `RootOption` so external test packages can inject stub generators without subprocess overhead.
  - Plumbed `rootConfig` through `newGenerateCmd`, `newValidateCmd`, `newTestCmd`, `newRunCmd`, and `newMockCmd` so `WithRuntimeDependencies` deps reach `withRuntimeWithOptions`.
  - Added `TestStartMockwayInfrastructure` (env-gated by `INFRAFACTORY_ENABLE_E2E=1`) and `TestRunInfrafactoryDrivesValidate` (default) to verify the helpers.
  - Verified with `make test` (Go unit + UI unit + 18 Playwright e2e) and `bash scripts/check_all.sh`.
- **UI hardening and Playwright e2e tests**:
  - Fixed scenario navigation bug: sidebar clicks now reload data using `afterNavigate` instead of `onMount`.
  - Added iteration progress banner to Live page: pulsing indicator during runs, pass/fail badge on completion.
  - Redesigned Live page with iteration timeline showing per-iteration stages, failures, and retry reasons.
  - Added Playwright e2e test suite (18 tests) covering navigation, scenario pages, and Live page iteration timeline.
  - Added `make test` target (Go unit + UI unit + Playwright e2e), plus `make ui-test` and `make ui-test-e2e`.
  - Replaced Mermaid diagrams with ASCII art in README.
- **Slice 30 complete (Layer 3 production readiness)**:
  - Added `tofu plan -state=terraform-live.tfstate` stage to sandbox deploy harness for `plan-live.txt` artifact capture (Contract #8).
  - Added auto-destroy of real Scaleway resources on failed runs without `--no-destroy` (Contract #14 billing protection).
  - Added post-generation validation that `scaleway_account_project` resource exists when Layer 3 enabled (Contract #12 self-managed project lifecycle).
  - Removed `sandbox_project_id` config field and `SCW_DEFAULT_PROJECT_ID` passthrough — project lifecycle is fully HCL-managed per ADR-0010.
  - Verified holdout checks execute Layer 3 dual-apply with sandbox deploy + real probes (Contract #10).
  - Closed `S9-T8` governance ticket (superseded by Slices 26-30).
  - Updated AGENTS.md with Scaleway bootstrap documentation and project management workflow.
  - All tests pass: `go test -tags noui ./...` green across all 8 packages.
- **M36 maintenance hardening complete**:
  - Switched UI run-error WebSocket escaping to JSON-backed escaping so control characters cannot corrupt payloads.
  - Made state-policy input flattening reject a top-level `state` collision instead of silently shadowing it.
  - Reused runstore path validation for iteration artifacts to block traversal paths.
  - Truncated large Mockway state error payloads before embedding them in returned errors.
  - Added focused regression coverage for each fix.
- **M35 review remediation complete**:
  - Fixed `test --no-destroy` CLI exposure and added regression coverage for cleanup suppression.
  - Restricted `baseline_state.json` persistence to incremental runs only and aligned the clean-run test.
  - Updated CONCEPT.md real-probe config docs to match the shipped `validation.real_probes` contract.
  - Changed `POST /api/runs/{scenario}/start` clean/no_destroy conflicts from `400` to `422`.
  - Hardened real probes for invalid ports and defensive empty-DNS responses.
  - Added destroy stderr propagation plus a regression proving Layer 3 cleanup still runs after probe failure.
- **Slices 26-29 complete**:
  - Added Layer 3 deploy/destroy harnesses with `terraform-live.tfstate`.
  - Added Layer 3 credential checks, prompt guidance, real probes, and opt-in real-tool Layer 3 smoke/incremental E2E coverage.
  - Added scenario-page Layer 3 toggle/readiness state and Live page Layer 3 progress/probe display.
  - Updated run/test output contracts, command goldens, README config/runtime/UI guidance, and tracking docs.
- **Slice 25 complete (S25-T1..S25-T5)**:
  - Added scenario-page `Keep state` and `Force clean` controls mapped to `no_destroy` / `clean`.
  - Added scenario-page run-mode detection using the backend `run-mode` endpoint.
  - Added Live page run-mode display plus collapsible `Plan Diff` and `Baseline State` panels backed by run artifacts.
  - Added frontend helper coverage for run-option normalization, run-mode summaries, plan/baseline URLs, and baseline JSON formatting.
  - Updated README Web UI guidance for incremental UI workflow and artifact visibility.
  - Verified with `cd ui && npm test && npm run build` and `bash scripts/check_all.sh`.
- **Slice 25 backend complete (S25-T1, S25-T2)**:
  - Persisted `plan.txt` and `baseline_state.json` as run-root artifacts in the runstore, with new runstore read/write helpers and regression coverage.
  - Added `GET /api/runs/{scenario}/{run_id}/plan` and `GET /api/runs/{scenario}/{run_id}/baseline`.
  - Extended `POST /api/runs/{scenario}/start` to accept `clean` and `no_destroy` JSON flags, with deterministic mutual-exclusion validation.
  - Added `GET /api/scenarios/{path}/run-mode`, using the same detection contract as the CLI: mockway resources, `terraform.tfstate`, and previous successful run.
  - Updated the UI run starter to pass through `clean` and `no_destroy` flags to the CLI run command.
  - Verified with focused API/UI tests, `go test ./internal/api ./internal/cli ./internal/runstore`, and `bash scripts/check_all.sh`.
- **Slice 24 complete (S24-T1, S24-T2)**:
  - Added `scenarios/training/incremental-project-paris.yaml` as the canonical evolving scenario fixture for incremental work.
  - Added `TestRunCommandRealToolIncrementalMockwayE2E`, an opt-in real-tool E2E that evolves one scenario through webserver → PostgreSQL → Redis while preserving state with `--no-destroy`.
  - Extended the same E2E to cover forced `--clean`, reseeding after clean destroy, final destroy without `--no-destroy`, and post-destroy fallback to clean auto-detection.
  - Patched the sibling `../mockway` repo so source-built mockway now supports the provider calls surfaced by the E2E: `POST /rdb/v1/regions/{region}/instances/{id}/upgrade` and `GET /redis/v1/zones/{zone}/clusters/{id}/certificate`.
  - Updated README with an explicit incremental operator workflow.
  - Verified with `cd ../mockway && go test ./...`, `INFRAFACTORY_ENABLE_REALTOOL_INCREMENTAL=1 go test ./internal/cli -run TestRunCommandRealToolIncrementalMockwayE2E -count=1`, and a final `bash scripts/check_all.sh`.
- **Slice 23 complete (S23-T1, S23-T2, S23-T3)**:
  - Added `run --clean` and `run --no-destroy` flags with mutual-exclusion validation.
  - Added incremental auto-detection using mockway state, `output/<scenario>/terraform.tfstate`, and the latest successful run for the scenario.
  - Added mockway snapshot-at-run-start and restore-per-iteration behavior for incremental runs; clean runs continue using reset.
  - Skipped destruction and holdouts under `--no-destroy`, and surfaced run mode in CLI output plus structured logs.
  - Extended `run.json` metadata with `incremental` and `previous_run_id`.
  - Preserved `terraform.tfstate` and `.terraform/` during incremental generated-file writes.
  - Updated README CLI/operator guidance and refreshed run command goldens.
  - Verified with `go test ./internal/cli ./internal/harness ./internal/runstore`, `go test -tags noui ./...`, and `bash scripts/check_all.sh`.
- **Slice 22 complete (S22-T1, S22-T2)**:
  - Added Mockway snapshot/restore support in the sibling `../mockway` repo using SQLite-backed state copies and DB reopen-on-restore behavior.
  - Added unauthenticated admin endpoints `POST /mock/snapshot` and `POST /mock/restore`.
  - Updated `POST /mock/reset` to clear any stored snapshot baseline.
  - Added focused repository and HTTP lifecycle tests covering snapshot success, restore success, restore-without-snapshot, and reset-clears-snapshot behavior.
  - Verified with `cd ../mockway && GOCACHE=/tmp/mockway-gocache GOMODCACHE=/tmp/mockway-gomodcache go test ./...`, `GOCACHE=/tmp/mockway-gocache GOMODCACHE=/tmp/mockway-gomodcache go test -tags noui ./...`, and `bash scripts/check_all.sh`.
- **Run viewer fallback and run-date follow-up**:
  - Diagnosed the blank IaC viewer against the real local runstore: the affected runs had no persisted `generated/` snapshot directories even though `output/<scenario>/` still contained IaC.
  - Added a run-viewer fallback so older runs without stored snapshots show the current scenario output with an explicit warning instead of an empty preview.
  - Added a `Started` column to `/runs`.
  - Added frontend regression coverage for run-date formatting and re-verified the frontend build/tests.
- **Run diff view and full artifact download follow-up**:
  - Added `GET /api/runs/{scenario}/{run_id}/artifacts.zip` to download the full run artifact directory, not only IaC files.
  - Upgraded the run detail page with snapshot-to-snapshot diffing and a second download action for the full archive.
  - Replaced the lightweight IaC tokenizer with a richer stateful highlighter that understands attributes, functions, interpolation, heredocs, and block comments.
  - Added backend regression tests for full-artifact archive contents and frontend helper tests for diff generation, archive URLs, snapshot option selection, and richer highlighting.
  - Re-ran Go tests after one transient WebSocket timeout in `internal/api`; the package passed on immediate rerun and the final full hygiene pass was green.
- **Per-iteration IaC snapshots, highlighting, and bundle download**:
  - Persisted immutable IaC snapshots under `.infrafactory/runs/<scenario>/<run_id>/iterations/<n>/generated/` in addition to the run-final generated set.
  - Added run API endpoints for iteration listing and iteration-scoped generated-file reads plus a zip bundle download endpoint for run IaC history.
  - Upgraded the run detail page with snapshot selection, syntax-highlighted IaC preview, and a download button for the run bundle.
  - Added runstore/API/CLI regression tests for iteration snapshot persistence and bundle generation, plus frontend helper tests for highlighting and bundle URLs.
  - Fixed a Makefile `.PHONY` parse bug so `make ui-build` works again.
  - Built embedded UI assets, launched the app against a synthetic runstore, and verified:
    - `/api/runs/demo-scenario/run-123/iterations` returned the iteration list
    - `/api/runs/demo-scenario/run-123/iterations/2/files/main.tf` returned the iteration snapshot IaC
    - `/api/runs/demo-scenario/run-123/bundle.zip` contained both final and per-iteration IaC paths
    - `/runs/demo-scenario/run-123` served the run detail page with `200`
- **Per-run IaC viewer and regression test follow-up**:
  - Persisted generated IaC under runstore paths (`.infrafactory/runs/<scenario>/<run_id>/generated/...`) instead of relying only on mutable scenario output directories.
  - Added run-scoped IaC API endpoints under `/api/runs/{scenario}/{run_id}/files` and `/api/runs/{scenario}/{run_id}/files/{path...}`.
  - Upgraded the run detail page into a per-run IaC viewer with file list, code preview, and a Live link.
  - Added UI mode/backend version markers in the sidebar and Run History row actions for `IaC` and `Live`.
  - Added frontend logic tests for Live selection, Run History filtering, and failure hint derivation.
  - Added backend regression tests for generated-file persistence/read/list/traversal, diagnostics success paths, and UI starter async lifecycle.
  - Launched a temporary live server against a synthetic runstore and verified:
    - `/api/diagnostics` returned ready Claude runtime status
    - `/api/runs/demo-scenario/run-123/files` returned the stored IaC file list
    - `/api/runs/demo-scenario/run-123/files/main.tf` returned stored IaC content
    - `/runs`, `/runs/demo-scenario/run-123`, and `/diagnostics` served the embedded app shell with `200`
  - Review pass 1 found durable doc gaps in README/BACKLOG/ROADMAP/plan notes and those were updated.
  - Review pass 2 found no further improvements.
  - Review pass 3 found no further improvements.
  - Final doc sync pass updated `SESSION_START.md`, `CONCEPT.md`, and `internal/runstore/doc.go` so fresh sessions and architecture docs match the shipped UI and run-scoped IaC history.
- **Web UI diagnostics and run-ops follow-up**:
  - Added `GET /api/diagnostics` with backend generator readiness checks for `claude-code` and `openrouter`.
  - Added `/diagnostics` UI page plus sidebar access, and linked Live failure hints to diagnostics.
  - Improved `/live` recovery to prefer the latest run for the selected scenario before falling back globally.
  - Added Run History filtering by scenario/run ID/terminal reason and status.
  - Added regression coverage for diagnostics contract, incomplete run directories breaking `/api/runs`, and UI starter context/preflight failures previously observed during manual testing.
- **Web UI run UX follow-up**:
  - Added UI run preflight checks so `POST /api/runs/{scenario}/start` fails immediately with a clear message when `claude` is missing from `PATH` or `OPENROUTER_API_KEY` is unset.
  - Updated the frontend API client to unwrap JSON `{error: ...}` responses into readable UI errors.
  - Reworked the Live page to show run failure cards with stage/check/command/detail instead of only raw iteration JSON.
  - Added focused `uiRunStarter` preflight tests and re-verified with `bash scripts/check_all.sh`.
- **Web UI DX hardening follow-up (post Slice 21)**:
  - Fixed frontend dependency conflict by aligning Vite to v6 (`npm install` now succeeds).
  - Added Docker Compose UI dev services: `infrafactory-api` and `infrafactory-ui` (profile `ui`).
  - Added Make targets: `ui-stack-up`, `ui-stack-logs`, `ui-stack-down`.
  - Fixed embedded asset handoff: `make ui-build` now copies built assets to `cmd/infrafactory/ui/build` for `go:embed`.
  - Updated `scripts/check_all.sh` to gate untagged tests on embedded asset path existence.
  - Updated README Web UI instructions for local and Docker workflows.
- **Slice 21 complete (SUi-1..SUi-8)**:
  - Added backend API package surfaces for scenarios, runs, output, config, and run start.
  - Added runstore capabilities: `ListScenarios`, `ReadIterationArtifact`, `RunMetadata.TerminalReason`.
  - Added WebSocket hub/client/sink and `/api/ws` streaming endpoint.
  - Added UI run starter wiring from `infrafactory ui` to real run execution with single-run guard and conflict handling.
  - Added SvelteKit dashboard routes for scenario browsing/editing, run history/detail, output viewing, and live log stream.
  - Added/expanded API and runstore test coverage for success/error/traversal/concurrency contracts.
  - Added GoReleaser pre-build hook for UI (`make ui-build`) and README Web UI documentation.
  - Added `github.com/coder/websocket` dependency.
- **UI follow-up hardening (Live run visibility fix)**:
  - Fixed a real Live page regression where runs launched from the scenario page could remain visually idle because `run.json` was only written at terminal completion.
  - `internal/cli/run_command.go` now persists initial run metadata with `status: running` before generator execution starts, so `/api/runs/{scenario}/{run_id}` becomes visible immediately.
  - `ui/src/routes/live/+page.svelte` now synthesizes console lines from polled run metadata and iteration artifacts when websocket delivery is absent, instead of staying stuck on `No active run.` or an empty waiting state.
  - `ui/src/lib/ws.ts` now bypasses the Vite websocket proxy in dev mode and connects directly to the backend websocket origin (`:4173` by default, override with `VITE_UI_API_ORIGIN`) to avoid repeated proxy `ECONNRESET` failures.
  - `internal/api/server.go` now explicitly allows localhost websocket origins so the cross-origin dev UI (`:5173`) can subscribe to `/api/ws` on the backend (`:4173`).
  - `internal/api/server.go` now gives upgraded websocket clients a connection-scoped lifetime instead of tying read/write pumps to the HTTP request context, which was causing live log streams to die before later run events arrived.
  - `internal/cli/ui_command.go` now resolves `agent.claude.command` to an absolute binary path during UI preflight and injects that resolved path into the async run runtime, avoiding later `PATH` drift for UI-triggered Claude runs.
  - `infrafactory.yaml` now pins the local Claude binary to `/opt/homebrew/bin/claude` on this machine so UI-triggered runs do not depend on shell PATH setup.
  - `internal/runstore/runstore.go` now explicitly skips directories missing `run.json` before metadata decode, preserving `/runs` even when the runstore contains older partial directories.
  - Added `GET /api/runs/{scenario}/{run_id}/log` backed by persisted `app.log`, and `/live` now replays those log lines before appending websocket frames. The page only falls back to synthesized metadata/artifact lines when both replay and websocket data are absent.
  - Fresh-context docs were updated so future agents do not need to rediscover these contracts:
    - `SESSION_START.md`
    - `README.md`
    - `CONCEPT.md`
    - `docs/plans/web-ui-plan.md` (marked as historical and aligned with current gates/runtime notes)
  - Added focused regression tests:
    - `TestRunCommandPersistsRunningMetadataBeforeCompletion`
    - frontend tests for `synthesizeLiveConsoleLines(...)`
    - frontend tests for websocket URL origin selection
    - backend websocket test for `Origin: http://127.0.0.1:5173`
    - HTTP-level start-run + websocket broadcast integration test
    - UI-run preflight test for resolved absolute Claude path
    - run-history regression test for historical incomplete directory names
    - run-log handler test for `app.log` replay
    - frontend replay/live merge helper test
  - Verified with:
    - `go test -timeout=20s -run 'TestRunCommandPersistsRunningMetadataBeforeCompletion' ./internal/cli`
    - `go test -timeout=60s ./internal/api`
    - `cd ui && npm test && npm run build`
    - `bash scripts/check_all.sh`
    - real local runtime reproduction: started backend on `127.0.0.1:4186`, connected a websocket client with `Origin: http://127.0.0.1:5173`, triggered `POST /api/runs/web-app-paris/start`, and observed streamed `run_start`, `iteration_start`, and `stage_start` frames.
    - real local UI-triggered run on `127.0.0.1:4189` with the pinned absolute Claude path reached `generator/claude: phase "plan_architecture" start` without reproducing `exec: "claude": executable file not found in $PATH`.

- **SUi-1 complete (Slice 21A)**:
  - Added `infrafactory ui` command with default bind `127.0.0.1:4173`.
  - Added root options pattern: `NewRootCmd(opts ...RootOption)` and `WithUIAssets(fs.FS)`.
  - Added build-tag embed split:
    - `cmd/infrafactory/embed.go` (`!noui`) embeds `ui/build`.
    - `cmd/infrafactory/embed_dev.go` (`noui`) provides nil assets.
  - Added `internal/api` skeleton:
    - `GET /api/config` allowlisted response only.
    - `GET /api/*` fallback returns `501 not implemented`.
    - SPA handler serves static files and `index.html` fallback when assets are embedded.
    - Non-API requests return deterministic 404 JSON message in API-only (`noui`) mode.
  - Added placeholder SvelteKit scaffold in `ui/`.
  - Added tests for server/spa/config handlers and root command wiring.
  - Updated `scripts/check_all.sh` to run `go test -tags noui ./...` when `ui/build/` is absent.
  - Added ADR-0008 documenting `ui` command contract and `noui` API-only behavior.
- **Slice 21 execution plan retained**: Full implementation plan remains in `docs/plans/web-ui-plan.md` as the historical/design reference for the shipped Web UI slice.
- **Slice 20 complete (S20-T1..S20-T6)**: 6 new scenarios exercising untested parameter combos:
  - `mysql-ha-paris`: mysql engine, medium DB, HA=true, private networking.
  - `compute-lb-multi-paris`: large compute (count=3), multi-backend LB (80/http + 443/tcp).
  - `k8s-medium-override-paris`: medium K8s with node_type/node_count overrides.
  - `private-lb-db-paris`: private LB, large PostgreSQL with node_type/engine_version overrides.
  - `public-registry-iam-paris`: is_public=true registry, IAM with policy=false.
  - `redis-xlarge-session-paris`: xlarge Redis with node_type override, xlarge compute.
  - Prompt fixes: LB backend/frontend zone pitfall, compute type mapping enforcement, phase1 exact-mapping enforcement.
  - Mockway fix: expanded server type catalog (GP1-L, GP1-XL, DEV1-L).
  - All 12 scenarios (6 existing + 6 new) pass on first iteration.
- **S19-T1 complete (round 4)**: Referential integrity and validation strictness:
  - **Delete cascades removed** — `DeleteLB`, `DeleteCluster`, `DeleteRDBInstance` now return 409 Conflict when dependents exist (per AGENTS.md contract). Exception: `lb_private_networks` cascade since the Scaleway provider doesn't detach them before LB delete.
  - **init_endpoints strict validation** — `BuildRDBEndpointsFromInit` rejects `private_network` with missing ID instead of silently falling back to public endpoint.
  - All 6 scenarios still pass on first iteration.
- **S19-T1 complete (round 2)**: Additional reliability fixes from extended review:
  - **IAM defaults not applied** — JSON Schema declared `default: true` for application/api_key/policy but Go's json.Unmarshal doesn't apply schema defaults. Fixed with `applyIAMDefaults()` that checks the raw YAML for omitted fields.
  - **LB/Frontend/Backend updates didn't persist** — same pattern as RDB update bug. Added `repo.UpdateLB()`, `repo.UpdateFrontend()`, `repo.UpdateBackend()` methods.
  - **LB list routes leaked cross-LB data** — `ListFrontends`/`ListBackends` ignored `lb_id` URL param. Added `ListFrontendsByLB`/`ListBackendsByLB` repo methods.
  - **RDB certificate endpoint didn't check instance existence** — returned 200 for nonexistent IDs. Now 404s.
  - **K8s companion test coverage** — added `scaleway_k8s_pool` auto-include test.
  - **Scenario test coverage** — added fixtures + tests for `iam`, `registry`, `redis`, `kubernetes` resource types and IAM default behavior.
  - All 6 scenarios still pass on first iteration.
- **S19-T1 complete (round 1)**: 3 bugs fixed (RDB update persistence, Redis missing fields).
- Completed Slice 18 (`S18-T1`..`S18-T5`): all 5 new scenarios pass on first iteration.

---

## NEXT_SESSION.md per-slice close-outs (archived 2026-06-02 at S74 prep)

Trimmed from NEXT_SESSION.md when the S63–S73 retirement chain was summarised into a single READ FIRST pointer. Per-slice details kept verbatim for traceability.

## READ FIRST — 2026-06-02

**S73 standalone retirement — GCP phase2 rules 9 + 12 (`google_project_service` family + `google_project_iam_member` family) retired Category A.** Audit prompted by "why aren't these in pitfalls?" — the real blocker was that the prompt rule was too effective (prevented the LLM from ever introducing the resource, so N13 had no failure to attribute). Executed N11 7-step protocol; 6 scenarios re-ran cleanly across two validation rounds with zero residual references. GCP phase2 collapsed 11 → 9 rules.

**S63–S67 arc CLOSED.** Five PRs merged (#37–#41). The post-collapse arc validated 39/39 deterministic + tightened N13 attribution + verified two flakes are non-reproducible + added `infrafactory mock reset` CLI for sweep harness ergonomics.

**Closed this session**:
- S63: 39/39 deterministic post-collapse sweep — no regression from the six N11 retirements.
- S64: N13 case-insensitive attribution. `attributeAppearsInDetail` matches removed attribute names in literal + case-insensitive snake_case + camelCase forms. Closes the aws_subnet `MapPublicIpOnLaunch` false-positive finding from S63. ADR-0012 amended.
- S65: gcp-cloud-run `deletion_policy` flake no longer reproducible (5/5 clean runs).
- S66: gcp-full-stack `google_apikeys_key` flake no longer reproducible (5/5 clean runs, zero apikeys mentions).
- S67: `infrafactory mock reset` CLI command added. `cloudMockStateRouter.ResetAll` fans out across mockway + fakegcp + fakeaws + s3 (SeaweedFS) cascade. Closes the S54 SeaweedFS state-leak gap — sweep harnesses no longer need a bare-curl carve-out.

## S68–S72 arc — CLOSED

- ✅ **S68**: N3 classifier coverage gap — `IsMockActionable` now recognizes `waiting for state to become` + `empty result` shapes.
- ✅ **S69**: M96 closed as superseded. ADR-0012 amended with the four-layer extractor model.
- ✅ **S70**: Permanent `cmd/n10extract` CLI — drives N10/N13 against a recorded run dir, emits a candidate pitfall YAML.
- ✅ **S71**: M98 already-fixed audit. All four affected policies already carry `after_unknown` branches. Added regression ratchet. M98 closed.
- ✅ **S72**: Two Category-A retirements (AWS S3 suffix + Scaleway encryption_at_rest). 12 total N11 retirements since the arc began.

## Suggested next-arc directions

None of these are blocking; pick what you need:

1. **Another full 39-scenario sweep** post-S68–S72 changes to confirm the broader picture is still 39/39 deterministic with N3 classifier + new ratchets in place. ~1 hr.
2. **More N11 retirements** in AWS phase3 / Scaleway phase3 — each remaining sub-bullet is a candidate per ADR-0018. ~30 min per rule.
3. **N10 organic emission audit** — sweep + audit any `learned_from_diff` or `learned_from_diff_avoid` entries that surface. The earlier S55-style audit pattern.
4. **Mock-server gap closure** — `docs/mock-gaps.md` is the standing backlog for fakegcp/fakeaws/mockway. Pick the highest-impact entry.
5. **Permanent `make clean-bg`** or similar session-hygiene target — the M53 era flagged session cleanup as friction; a tiny Makefile target would help.

## Next arc: S68–S72

Plan file: `docs/plans/slices-68-72-plan.md`. Five slices, ~6–10 focused hours, designed for one autonomous loop session.

- **S68**: N3 classifier coverage gap — add the two patterns from S63's audit (`aws_kms_key` rotation timeout, `aws_route53_record` empty-result). ~30 min.
- **S69**: Close M96 (descriptive vs prescriptive) as superseded by N10/N13 — audit the legacy extractor's call sites. ~1 hr.
- **S70**: Promote the throwaway `cmd/n10extract` helper to a permanent CLI command. ~1-2 hr.
- **S71**: M98 — fix OPA known-after-apply false-fire on `vpc_required.rego` + `encryption.rego` (false-flagging correct HCL because `planned_values` shows `null` for known-after-apply references). ~half-day.
- **S72**: ADR-0018 sustaining audit — AWS + Scaleway phase3 retirements. Expect 1–2 more Category-A per cloud. ~2-3 hr.

Autonomous-execution loop prompt at the bottom of the plan file.


**S63 audit findings carried into S64**:
1. `aws_subnet` `learned_from_diff` is a false positive — the LLM's iter-pair diff captured ADDED attrs (`cidr_block`, `availability_zone`) but the actual fix was REMOVAL of `map_public_ip_on_launch`. N13 should have caught it but the failure detail uses camelCase (`MapPublicIpOnLaunch`) while the HCL attribute is snake_case (`map_public_ip_on_launch`) — N13's `strings.Contains(failureDetail, attr)` attribution failed. **Fix in S64**: case-insensitive (or snake↔camel) attribute matching.
2. Two mock-actionable failures (`aws_kms_key` rotation timeout, `aws_route53_record` empty-result) bypassed the N3 classifier and landed as `learned` pitfalls. The N3 `IsMockActionable` predicate needs to recognize "rotation update timeout" + "empty result" as signals. **Investigate in S64-T2 territory; may spawn a small N3-classifier patch slice.**
3. No `learned_from_diff_avoid` entries — N13 in production confirms the heuristic doesn't false-positive (good) but also confirms the gcp-cloud-run flake from S59 wasn't deterministic.

Plan file with full slice definitions: `docs/plans/slices-63-67-plan.md`.

## What's already done (S54–S62 close-out)

| Slice | PR | Outcome |
|---|---|---|
| S54 | #26 | 39/39 deterministic sweep baseline + N10 dedupe fix |
| S55 | #27 | N10 wiring + trim + ratchet fixes |
| S56 | #28 | Retire GCP rule 11 (firewall) — Category A |
| S57 | #29 | Retire GCP rule 13 (GKE) — Category A |
| S58 | #30 | Retire GCP rules 14 + 15 (Cloud SQL + GCS) — Category A |
| S59 | #31 | Retire GCP rule 10 (VPC) — Category B |
| S60 | #32 | Retire AWS RDS + Scaleway RDB/LB — Category A |
| S61 | #33 | N13 deletion-as-fix extractor |
| S62 | #34 | ADR-0018 retirement-criteria framework |

**Architectural milestones**:
- GCP phase2 prompt collapsed from 17 prescriptive rules to 11 (9 if hand-applied retirements count as Category C).
- N10 → N11 → N13 sequence end-to-end: addition-as-fix via `learned_from_diff`, removal-as-fix via `learned_from_diff_avoid`, both fed by real run diffs.
- 7-step retirement protocol generalized across GCP + AWS + Scaleway.

## Open follow-ups carried into the S63–S67 arc

- **gcp-cloud-run `deletion_policy` hallucination** (from S59 step 5) — S65 handles.
- **gcp-full-stack `google_apikeys_key` mock gap** (from S57) — S66 handles.
- **SeaweedFS state-leak in bare-curl sweep harness** (from S54) — S67-T1 handles via `infrafactory mock reset` CLI command.
- **Permanent `cmd/n10extract`** — S67-T3 (optional).

## Important context still relevant

- **ADR-0012** (with two amendments) captures N10 + N13 design.
- **ADR-0018** codifies N11 retirement criteria — read before any further retirement.
- **`feedback_sweep_protocol.md`**: never hand-edit `pitfalls/*.yaml`. Discard sweep pollution with `git checkout pitfalls/`. The N10/N13 extractors are the only legitimate authors.
- **`feedback_mock_design.md`**: mocks optimize for fast feedback, not realism. Mock gaps get fixed at source.
- **Standing rules for autonomous execution** are in `docs/plans/slices-54-62-plan.md` § "Standing rules". S63–S67 inherits them all.

## Older context

- **Per-slice close-out notes for S54–S61** + **prior-session narratives back to 2026-05-30** live in `docs/status/ARCHIVE.md` (S54-S62 section).
- **`STATUS.md`** has the rolling "current phase" log; older entries move to `docs/status/ARCHIVE.md` per the existing update policy.

---

## STATUS.md per-slice close-outs S63-S73 (archived 2026-06-02 at S74 prep)

Trimmed when the S54-S73 GCP collapse was rolled up into a single "GCP phase2 prompt-collapse complete" milestone.

## Current phase

- **S73 — GCP phase2 rules 9 + 12 retired (project_service + project_iam_member family).** Two Category-A retirements based on a clarifying audit ("why are these not part of pitfalls?"): the real blocker to N13 self-learning was that the prompt rule was too effective — it prevented the LLM from ever introducing the resource, so no failure recorded for N13 to attribute. Executed N11 7-step protocol: deleted both rules + phase3 equivalents (rule 4 + rule 11), then re-ran 6 GCP scenarios (gcp-cloud-sql, gcp-cloud-run, gcp-storage, gcp-full-stack, gcp-iam, all twice). Every scenario passed `target_reached` with zero `google_project_service` / `google_project_iam_*` references in any generated HCL. The LLM doesn't reach for these resources even without the prompt rule — rule was redundant. Per ADR-0018 Category A: delete with no follow-up. GCP phase2 collapsed from 11 to 9 prescriptive rules (rules 1-8 system + rules 16+17 scenario-bound). N13's `learned_from_diff_avoid` could still capture these patterns IF a future scenario triggers them, but production behavior shows the LLM doesn't need the prompt rule to avoid them.
- **S72 — AWS + Scaleway phase3 retirements landed. S68–S72 arc CLOSED.** Two Category-A retirements per ADR-0018:
  - AWS phase3 rule 3 sub-bullet on `S3 bucket names include an account-synthetic or run-scoped suffix`. Re-ran `aws-s3` → target_reached iter 1 (LLM produced `bucket = var.bucket_name` correctly — the suffix is in the variable default).
  - Scaleway phase3 rule 6.d on `Encryption at rest if required`. Re-ran `mysql-ha-paris` → target_reached iter 1 (LLM produced `encryption_at_rest = true` correctly).
  - Both rules' violations had strong machine-readable signals (S3 `BucketAlreadyExists` from the runtime; OPA `encryption_at_rest not enabled` from the policy), so the auto-correction loop carries them. Eleventh and twelfth N11 retirements since the arc began (CMEK + firewall + GKE + SQL + GCS + VPC + AWS-RDS + SCW-RDB + SCW-LB + S3-suffix + SCW-encryption).
  The S68–S72 arc is closed with 5 PRs (#43 S68, #44 S69, #45 S70, #46 S71, this S72 PR).
- **S71 — M98 OPA known-after-apply audit closed.** Discovery: all four affected policies (`gcp/vpc_required.rego`, `gcp/encryption.rego`, `aws/vpc_required.rego`, `aws/encryption.rego`) already carry `after_unknown.X == true` branches — landed between 2026-05-23 and S60. The remaining 9 policies either inspect literal fields (region, plain bool, block presence) or use the `configuration` view (Scaleway `vpc_required`), so they're not affected. Empirical confirmation: the S63 39/39 sweep passed with no policy false-fire. S71 adds `TestOPAPoliciesM98KnownAfterApplyBranches` as a guard against accidental regression on the four fixes. BACKLOG M98 row updated to `done`.
- **S70 — Permanent `cmd/n10extract` CLI landed.** Throwaway tool from the 2026-06-02 loop session promoted to a permanent command. Takes `--failed-dir` + `--passing-dir` + `--failure-detail` + `--failure-resource` + `--cloud` + `--scenario` + `--mode {fix,avoid}` and emits a candidate `LearnedPitfall` snippet as pitfalls-file YAML on stdout. `--run-dir <path>` shorthand auto-discovers the iter pair from `.infrafactory/runs/<scenario>/<run-id>/iterations/N/generated/` directories (picks last-failing + last-passing). Three unit tests pin the auto-discovery; smoke-tested against a real gcp-cloud-sql run dir (extracted the expected CMEK+private_network snippet). `make build` now also produces `bin/n10extract`. Stable forced-extract path for future N11 retirements (per ADR-0018) when the organic learn loop hasn't fired for the target pattern.
- **S69 — M96 closed as superseded.** Audit found `ExtractLearnedPitfall` is not superseded by N10/N13 — they're layered, not competing. N10/N13 fire only on `target_reached` and produce prescriptive rules from real iter-pair diffs; M97 templates inside `ExtractLearnedPitfall` cover the still-stuck-runs niche where the auto-correction loop hasn't converged yet; the descriptive fallback is the last-resort base case when neither matches. M96's original question (path 1 vs path 2) was answered architecturally by the N10→N11→N13 sequence over Slices 54-67, not by changing `ExtractLearnedPitfall`. No code change; BACKLOG row marked done with the audit rationale, ADR-0012 amended with the four-layer extractor model.
- **S68 — N3 classifier coverage gap closed.** Added two patterns to `IsMockActionable` from the S63 sweep audit: (a) `waiting for state to become '...'` (provider polling on a mock-side field that doesn't persist after Update — AWS KMS rotation, EC2 Subnet MapPublicIpOnLaunch, similar); (b) `empty result` (mock acks Create but Read returns 0 rows — aws_route53_record). The pre-existing `aws_subnet` `MapPublicIpOnLaunch` stale entry that the ratchet caught is removed from `pitfalls/aws.yaml`. Three new positive-case tests in `TestIsMockActionable_FivePositiveSignalClasses`. M91 ratchet was already source-aware and now enforces the new signals at CI time.
- **S67 — `infrafactory mock reset` CLI command landed. S63–S67 arc CLOSED.** New subcommand resets every configured mock backend (mockway + fakegcp + fakeaws + s3 cascade via `cloudMockStateRouter.ResetAll`) in one call. Closes the S54 SeaweedFS state-leak gap: sweep harnesses no longer need a bare-curl carve-out to drop SeaweedFS buckets. Two unit tests pin the fan-out (with + without s3 configured). Smoke-tested against the live stack. With this, the entire S63–S67 arc is closed (5 PRs: #37 S63, #38 S64, #39 S65, #40 S66, this S67 PR).
- **S66 — gcp-full-stack `google_apikeys_key` flake no longer reproducible.** Ran the scenario 5× consecutively (S66-T4): 4× target_reached iter 1 (164-266s), 1× target_reached iter 2 (414s). Zero `google_apikeys_key` mentions across any of the 5 logs — the LLM never reached for the unsupported resource. The S57 mock-gap was non-deterministic LLM behavior. Closing without a code change. Safety net: if the LLM reaches for `google_apikeys_key` in a future run, the apply will fail clearly (the resource isn't implemented by fakegcp); N13 should catch the removal organically once the LLM self-corrects.
- **S65 — gcp-cloud-run `deletion_policy` flake no longer reproducible.** Ran the scenario 5× consecutively (S65-T1): 4× target_reached iter 1 (24-32s), 1× target_reached iter 2 (53s). No `deletion_policy` hallucination in any run. The S59 stuck-pattern was non-deterministic LLM behavior, not a systemic gap. Closing without a code change. Safety net: S64's case-insensitive N13 attribution + the existing `google_cloud_run_v2_service` `deletion_protection` learned pitfall mean any future recurrence would: (a) feed back through the dynamic correction loop, (b) self-learn into pitfalls via N13's `learned_from_diff_avoid` shape if the LLM removes the offending attr to clear it.
- **S64 — N13 case-insensitive attribution CLOSED.** Closes finding (1) from S63's audit. `ExtractPrescriptiveAvoid` now matches removed attribute names against the failure detail in three forms — literal, case-insensitive snake_case, and camelCase (`map_public_ip_on_launch` → `MapPublicIpOnLaunch`). The AWS provider echoes JSON-side field names verbatim in many timeout errors, so the original strict-substring check missed legitimate deletion-as-fix patterns. New regression test pins the aws_subnet `MapPublicIpOnLaunch` shape; existing four N13 tests remain green. ADR-0012 amended. S63 audit findings (2) and (3) carried into S65/S66 (the two flake-triage slices) since both flakes also didn't recur in S63 — those slices become "verify reproducibility + close if stable" rather than fix-with-code.
- **S63 — 39/39 deterministic sweep CLOSED.** Post-collapse re-validation across all 39 training scenarios: every scenario passed (`target_reached`) under the prompt-collapsed state from S54–S62. No regression from the six N11 retirements. Three audit findings carried into S64: (a) `aws_subnet` learned_from_diff false positive — N10 captured added attrs while the actual fix was a REMOVAL of `map_public_ip_on_launch` (N13 case but the failure detail used the camelCase `MapPublicIpOnLaunch` while the HCL attribute is `map_public_ip_on_launch`, so attribution missed); (b) two mock-actionable failures (`aws_kms_key` rotation timeout, `aws_route53_record` empty-result) bypassed the N3 classifier and landed in pitfalls as `learned`; (c) N13 didn't fire organically — the gcp-cloud-run `deletion_policy` flake from S59 didn't recur this sweep. Pitfall pollution discarded per protocol; the legitimate entries will re-emerge.
- **S54–S62 sustain + prompt-collapse arc CLOSED.** Nine PRs merged (#26–#34). GCP phase2 prompt collapsed from 17 → 11 prescriptive rules. ADR-0018 codifies the three-category N11 retirement framework. The N10 → N11 → N13 sequence (addition + removal auto-derivation) is end-to-end across GCP + AWS + Scaleway.
- Older milestones (S1–S53) are in `docs/status/ARCHIVE.md`.
