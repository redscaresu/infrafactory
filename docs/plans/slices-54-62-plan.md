# Slices 54–62 — sustain + prompt-collapse arc

Status: planned (2026-06-02)
Owner: next-session claude (designed for autonomous execution)
Replaces: `slices-54-58-plan.md` (now folded in; that file may be deleted)

## Big picture (the destination — item 7 from the prior session's close-out)

The 2026-06-02 loop session proved the N10→N11 architectural shift end-to-end. The CMEK rule retirement showed the auto-correction feedback channel CAN carry prescriptive knowledge that the prompt previously stated. The natural question: **how far can the prompt collapse?**

Goal: shrink the GCP / AWS / Scaleway phase-2 + phase-3 prompts from "playbook of every gotcha across every resource" to **"system contract + scenario intent only"** — every prescriptive rule either retired (with or without `learned_from_diff` pitfall replacement) or documented as load-bearing.

Concretely:
- **7 GCP retirements possible**: phase2 rules 9 (avoid project_service), 10 (VPC), 11 (firewall), 12 (avoid project_iam_member), 13 (GKE), 14 (SQL), 15 (GCS). Rules 1-8 (system), 16 (region/scenario-bound), 17 (naming) stay.
- **AWS retirements**: phase2 is already short (rules 1-10 are mostly system/structural); phase3 has 10+ pitfall-driven rules. Per-rule audit needed.
- **Scaleway retirements**: phase2 rule 9 (private networking) + phase3 rule 7 (RDB private_network ip_net/enable_ipam) are obvious candidates. Per-rule audit.
- **+ N13 deletion-as-fix**: lets the system retire its OWN "do NOT use" rules (avoid-patterns). Phase 2 of N10. Currently the only way to retire avoid-rules is hand-edit.

If all candidates retire: the GCP phase2 prompt collapses from ~17 rules to ~7 (system/contract only). Similar for AWS + Scaleway.

## Slices (sequenced for autonomous execution)

| Slice | Title | Effort |
|---|---|---|
| S54 | Full 39-scenario deterministic sweep + sustain ratchet | ~1 hr |
| S55 | N10 first-production-entries audit | ~1 hr |
| S56 | N11 retire GCP phase2 rule 11 (firewall network-vs-subnetwork) | ~1-2 hr |
| S57 | N11 retire GCP phase2 rule 13 (GKE single-node-pool) | ~1-2 hr |
| S58 | N11 retire GCP phase2 rules 14 + 15 (SQL teardown · GCS test setup) | ~2 hr |
| S59 | N11 retire GCP phase2 rule 10 (VPC + subnetwork) | ~1-2 hr |
| S60 | AWS + Scaleway prompt-rule retirement audit + 2-3 retirements | ~3-4 hr |
| S61 | N13 — N10 phase 2 deletion-as-fix extractor | ~half-day |
| S62 | ADR-0018 — N11 retirement criteria + close-out | ~30 min |

**Total**: ~14-18 focused hours. Designed for one autonomous Claude loop session running across two sittings (or one long sitting if context window permits).

## Standing rules for autonomous execution

These apply to every slice. The next-session prompt at the bottom of this doc invokes them.

1. **Authority to merge PRs**: open + merge with `gh pr merge <N> --squash --admin --delete-branch` once CI is green, in all three repos (`infrafactory`, `../mockway`, `../fakegcp`, `../fakeaws`). Wait for the doc-hygiene + test + gitleaks checks to pass; skip the `build-binary` check which is gated on tags only.
2. **Pitfall pollution discipline**: after any failed sweep, discard auto-learning noise with `git checkout pitfalls/`. **Never hand-edit `pitfalls/*.yaml`.** If you need a `learned_from_diff` entry for N11 step 2, drive the legitimate extractor against a recorded run dir (a small `cmd/n10extract/main.go` was used + removed in the prior session — same pattern).
3. **Mock + binary rebuild discipline**: after any merge to a sibling repo, rebuild + restart that mock (`pkill -f <mock-bin>; cd ../<mock> && go build && ./<mock> --port <port> &`). After any infrafactory commit touching `internal/`, `prompts/`, or `pitfalls/`, run `make build`.
4. **STATUS + ADR + NEXT_SESSION updates** are MANDATORY before merging any PR that changes code under `internal/` or `cmd/infrafactory/`. The `check-doc-hygiene` CI job enforces this. If unsure whether an ADR is needed, add one — it's cheaper than a CI re-run.
5. **Per-PR scope**: one slice per branch. The PR title format is `S<N>: <title>`. The PR body must reference the slice plan + capture the protocol outcome (especially for N11 retirements — which exit path of the 7-step protocol did you hit?).
6. **The N11 7-step protocol** for every retirement:
   1. Sweep with N10 active.
   2. Inspect `pitfalls/<cloud>.yaml` for a `learned_from_diff` entry covering the rule's pattern. If absent, force one via recorded-run replay.
   3. Delete the prompt rule locally.
   4. Blank the matching pitfall entry locally.
   5. Re-run the scenario(s) that exercise the rule.
      - If passes: **rule was redundant** (auto-feedback carries it). Skip steps 6+7; commit prompt-rule deletion only.
      - If fails: go to step 6.
   6. Restore the pitfall (NOT the prompt rule). Re-run. Should pass.
   7. Commit prompt-rule deletion. If step 6 also fails, restore the prompt rule + file a follow-up against the N10 extractor.
7. **Don't stop unless goal reached or genuinely blocked**. The loop instructions are explicit. If a sweep fails on something not in scope (e.g. a regression in an unrelated scenario), triage briefly + continue to the next slice. Capture the regression in `docs/NEXT_SESSION.md` so it doesn't get lost.

## S54 — Full 39-scenario deterministic sweep + sustain ratchet

### Motivation
PRs #23 (provider batching) + #22 (N10 fix) + #24 (CMEK retirement) all merged this session. The confirmation sweep covered only 4 GCP scenarios. AWS (12) and Scaleway (16) need re-validation. A clean 39-scenario sweep proves no regression and gives N10 a chance to populate fresh `learned_from_diff` entries broadly.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S54-T1 | Run all 39 training scenarios sequentially against current main. Reset mocks between scenarios. Capture per-scenario `terminal_reason` + iteration count + any new `learned_from_diff` entries. | P0 | — |
| S54-T2 | For any scenario that regresses: triage (mock gap → fakegcp/fakeaws/mockway PR; LLM gap → continue). Re-run until 39/39 holds in a single uninterrupted sweep. | P0 | S54-T1 |
| S54-T3 | Update `STATUS.md` + `docs/scenario-failure-matrix.md` with the post-sweep state. | P1 | S54-T2 |

### Exit criteria
- Single uninterrupted 39-scenario sweep passes.
- All regressions triaged + fixed (or filed for follow-up if non-blocking).
- STATUS reflects post-sweep state.

## S55 — N10 first-production-entries audit

### Motivation
PR #22 added the type-hint fallback to N10 attribution. The "exactly-one-match" abstention rule should prevent false positives, but the heuristic is only as good as the hint table. The S54 sweep produces the first batch of real entries; each one is a claim that the iter-pair diff was the *fix*. Wrong attribution = misleading future iterations.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S55-T1 | For every `source: learned_from_diff` entry in `pitfalls/<cloud>.yaml` post-S54, read both iter dirs and verify: (a) attributed type matches failure intent, (b) snippet captures actual fix not unrelated additions, (c) trim cap doesn't cut mid-block. | P0 | S54 |
| S55-T2 | For each false-positive: extend `prescriptive_extractor_test.go` with a regression test pinning the failure shape, tighten heuristic, re-run affected scenario to verify. | P0 | S55-T1 |
| S55-T3 | CI ratchet: fail if any `learned_from_diff` snippet exceeds 600 bytes. | P2 | — |

### Exit criteria
- Every entry hand-verified.
- False positives have regression tests.
- Trim-cap ratchet in place.

## S56 — N11 retire GCP phase2 rule 11 (firewall network-vs-subnetwork)

### Motivation
Single-attribute correction. `tofu validate` rejects `subnetwork` argument on `google_compute_firewall` with an "argument not expected" error — strong machine-readable feedback the LLM can self-correct from. If this retires (which it likely does via step 5's "redundant" exit), that confirms **validation-error-driven self-correction works**, and S57/S58 can attack higher-stakes rules with confidence.

Affected scenarios: most networked-multi-tier ones (gcp-full-stack, gcp-load-balancer, etc.).

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S56-T1 | Identify which scenario reliably exercises `google_compute_firewall`. Likely `gcp-full-stack` or `gcp-load-balancer`. | P1 | S54 |
| S56-T2 | Step 1-2 of N11: check `pitfalls/gcp.yaml` post-S54 for a `learned_from_diff` covering firewall correction. If absent, force one via recorded-run replay. | P1 | S56-T1 |
| S56-T3 | Step 3-4: delete rule 11 from `prompts/gcp/phase2_generate_hcl.md`. Re-number subsequent rules. Blank matching pitfall. | P1 | S56-T2 |
| S56-T4 | Step 5: re-run the chosen scenario. Capture outcome. | P0 | S56-T3 |
| S56-T5 | Step 6-7 if needed (restore pitfall, re-run, confirm). Then PR + STATUS + NEXT_SESSION update. | P0 | S56-T4 |

### Exit criteria
- Rule 11 retired (with or without pitfall replacement) OR documented as load-bearing.
- 7-step protocol outcome captured.

## S57 — N11 retire GCP phase2 rule 13 (GKE single-node-pool)

### Motivation
Multi-attribute rule (`remove_default_node_pool = true` + `initial_node_count = 1` + separate `google_container_node_pool`). Failure mode is `tofu plan` error ("default node pool conflict") — similarly machine-readable.

Sequenced after S56 because S56's outcome confirms whether validation-error-self-correction generalises. If S56 reveals the LLM doesn't reliably self-correct from validation errors, S57 needs the pitfall replacement path (step 6).

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S57-T1 | Inspect `pitfalls/gcp.yaml` for `google_container_cluster` or `google_container_node_pool` `learned_from_diff` entry covering single-pool pattern. Force one via recorded replay if absent. | P1 | S54, S56 |
| S57-T2 | Execute N11 steps 3-7 against rule 13. Re-run gcp-gke-cluster + gcp-full-stack. | P0 | S57-T1 |
| S57-T3 | PR + STATUS + NEXT_SESSION update. | P0 | S57-T2 |

### Exit criteria
- Rule 13 retired or documented as load-bearing.
- Two retirements complete → if both pass via step 5, that's the criterion for codifying in S62.

## S58 — N11 retire GCP phase2 rules 14 + 15 (SQL teardown · GCS test setup)

### Motivation
Compound rules with multiple attributes each:
- Rule 14: `deletion_protection = false` + run-scoped name suffix + `ipv4_enabled = false` for SQL.
- Rule 15: `force_destroy = true` + `uniform_bucket_level_access = true` for GCS.

Both have multiple failure modes (destroy-orphan check, region-restriction, no-public-sql policy). If they retire, that's strong evidence the protocol generalises beyond "single-attribute or simple multi-attribute" rules.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S58-T1 | N11 protocol on rule 14. Affected scenarios: gcp-cloud-sql, gcp-full-stack. | P1 | S57 |
| S58-T2 | N11 protocol on rule 15. Affected scenarios: gcp-storage, gcp-full-stack, gcp-gke-cluster (CMEK bucket). | P1 | S58-T1 |
| S58-T3 | Combined PR or two PRs depending on outcome. STATUS + NEXT_SESSION updates. | P0 | S58-T2 |

### Exit criteria
- Both rules retired or documented as load-bearing.

## S59 — N11 retire GCP phase2 rule 10 (VPC + subnetwork)

### Motivation
The most-referenced GCP rule. Affects every networked scenario (compute_instance, container_cluster, sql with private_network, etc.). High-stakes retirement — if it succeeds, the GCP prompt collapses dramatically. If it fails, the retirement criteria from S56-S58 needs revisiting.

Sequenced last among GCP retirements because it's the hardest case.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S59-T1 | N11 protocol on rule 10. Affected scenarios: nearly all GCP scenarios. | P1 | S58 |
| S59-T2 | PR + STATUS + NEXT_SESSION update. | P0 | S59-T1 |

### Exit criteria
- Rule 10 retired or documented as load-bearing.
- GCP phase2 prompt down from 17 rules to 6-10 (depending on which retirements stick).

## S60 — AWS + Scaleway prompt-rule retirement audit + 2-3 retirements

### Motivation
Validate that the N11 architectural shift generalises beyond GCP. AWS phase2 is short (mostly system rules); AWS phase3 has more prescriptive content. Scaleway phase2 + phase3 have several prescriptive rules.

Per-cloud audit: inspect which rules describe prescriptive resource-specific behaviour vs system-level constraints. Retire 1-2 per cloud as proof-of-concept.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S60-T1 | Audit `prompts/aws/phase{2,3}_*.md` — list each prescriptive rule with its failure-mode classification (validation error · plan error · policy violation · destroy failure). | P1 | S54 |
| S60-T2 | Same for `prompts/scaleway/phase{2,3}_*.md`. | P1 | S54 |
| S60-T3 | Pick the 2 most likely AWS candidates (by failure-mode classification — prefer validation/plan errors) + execute N11 protocol on each. | P0 | S60-T1, S55 |
| S60-T4 | Pick the 2 most likely Scaleway candidates + execute N11. The `scaleway_rdb_instance private_network ip_net/enable_ipam` rule (phase3 rule 7) is the obvious first target. | P0 | S60-T2, S55 |
| S60-T5 | Aggregate PR. STATUS + NEXT_SESSION update. | P0 | S60-T4 |

### Exit criteria
- AWS + Scaleway audits documented.
- 2-3 retirements landed across the two clouds.

## S61 — N13: N10 phase 2 deletion-as-fix extractor

### Motivation
GCP phase2 rules 9 + 12 are AVOID patterns ("do NOT use X"). The current N10 only learns from additions. N13 extends N10 to learn from removals: a sweep that ran with `google_project_iam_member` → failed → ran without it → passed = "do NOT use `google_project_iam_member`" as a `learned_from_diff_avoid` pitfall.

If N13 lands, rules 9 + 12 become retirement candidates (the system would carry them from pitfalls). The current loop session prompt-retired them via PR #23 / #24; with N13, the system could derive them.

Sequenced before S62 because S62's ADR depends on whether the system can self-derive avoid-patterns or not.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S61-T1 | Extend `ExtractPrescriptiveFix` to compute REMOVAL diff (what's in iter[N-1] but not iter[N]). Add `LearnedPitfall.Source = "learned_from_diff_avoid"`. | P1 | S55 |
| S61-T2 | Heuristic: only emit when (a) removed attribute appears in failure detail OR (b) a top-level resource of the failing type was removed entirely. | P1 | S61-T1 |
| S61-T3 | Wire into `run_command.go` alongside addition-as-fix. Tests: 3-4 synthetic cases (project_service removal, deletion_protection removal, ipv4_enabled toggle, project_iam_member removal). | P1 | S61-T2 |
| S61-T4 | Validation against recorded runs: pre-PR #23 gcp-full-stack run dir + post-PR #23 dir. Extractor should emit `learned_from_diff_avoid` for `google_project_iam_member`. | P0 | S61-T3 |
| S61-T5 | PR + ADR-0012 amendment + STATUS + NEXT_SESSION update. | P0 | S61-T4 |

### Exit criteria
- `ExtractPrescriptiveFix` handles both addition + removal patterns.
- 3+ removal-as-fix test cases.
- Recorded-run validation produces expected `learned_from_diff_avoid` entries.

## S62 — ADR-0018 + close-out

### Motivation
Multiple retirements landed (CMEK + S56-S60 batch). The pattern across them is the basis for ADR-0018 ("N11 retirement criteria"): which prompt rules retire cleanly, which need pitfall replacement, which are load-bearing.

Also produces the final STATUS + NEXT_SESSION close-out for the arc.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S62-T1 | ADR-0018 — codify N11 retirement criteria. Categories: (a) "redundant — delete with no follow-up" (auto-feedback carries it), (b) "replaced by learned_from_diff pitfall", (c) "load-bearing — keep". Reference each retirement's exit path. | P0 | S56-S60 |
| S62-T2 | Final STATUS entry summarising the prompt-collapse arc: pre-arc rule counts vs post-arc, per-cloud. | P0 | S62-T1 |
| S62-T3 | Replace the "2026-06-02 loop session close-out" section in `docs/NEXT_SESSION.md` with the new close-out. Archive the old one inline below. | P0 | S62-T2 |
| S62-T4 | (Optional) Update README to mention the auto-derivation property if it's now stable enough to be a feature claim. | P2 | S62-T2 |

### Exit criteria
- ADR-0018 merged.
- NEXT_SESSION updated to point at next-arc work.

## Why this order, in one paragraph

S54 establishes the regression-free baseline (without it, every later claim is shaky). S55 audits the first batch of N10 entries before more land. S56 + S57 + S58 + S59 are the GCP retirement arc — easy-to-hard sequencing within one cloud lets the protocol stabilise. S60 broadens to AWS + Scaleway to prove the shift generalises. S61 extends N10 to handle the avoid-patterns the GCP prompts still have. S62 codifies the lessons + closes the arc. Total ~14-18 focused hours, designed to run autonomously with the loop prompt at the bottom of this doc.

## Autonomous-execution loop prompt

Paste this into a fresh Claude session to start the arc with no further intervention:

```
/loop until all 9 sustain-arc slices (S54-S62) in docs/plans/slices-54-62-plan.md are complete: every slice's exit criteria met, every PR merged with green CI, STATUS + NEXT_SESSION updated.

Read docs/NEXT_SESSION.md § "2026-06-02 loop session close-out — READ FIRST" for the prior session's full handoff, then docs/plans/slices-54-62-plan.md for the slice definitions + standing rules + N11 7-step protocol.

Work slices in order S54 → S55 → S56 → S57 → S58 → S59 → S60 → S61 → S62. Each slice may take multiple iterations / PRs. The standing rules at the top of slices-54-62-plan.md govern every PR (authority to merge, pitfall pollution discipline, mock rebuild discipline, STATUS+ADR+NEXT_SESSION updates, scope per PR).

Authority: open + merge PRs with `gh pr merge <N> --squash --admin --delete-branch` once CI is green, in all four repos (infrafactory, ../mockway, ../fakegcp, ../fakeaws). Discard auto-learning sweep pollution in pitfalls/*.yaml with `git checkout pitfalls/` — never hand-edit. For N11 step 2's `learned_from_diff` entries when none exist, drive the legitimate extractor against a recorded run dir (cmd/n10extract pattern from the prior session — write the tiny tool, use it, delete it).

Stop only when: (a) all 9 slices complete (every exit criterion met) OR (b) you genuinely cannot proceed (e.g., a regression in main blocks every scenario AND fix-forward isn't obvious — in that case document the blocker in NEXT_SESSION + stop). Otherwise keep going — don't ask "should I continue?", the answer is yes.

If a sibling-mock handler isn't implemented and a sweep needs it, implement it (with tests + PR through the normal review flow).

Before stopping for any reason, update docs/NEXT_SESSION.md with: which slices closed, which are blocked, what next session should do FIRST.
```

## Fresh-context checklist (for the agent running the prompt above)

The autonomous prompt assumes the agent reads:
1. `AGENTS.md` (always — entry point)
2. `docs/NEXT_SESSION.md` (always — § "2026-06-02 loop session close-out — READ FIRST")
3. This file (`docs/plans/slices-54-62-plan.md`) — slice definitions + standing rules + the N11 protocol
4. `STATUS.md` for current state
5. `docs/decisions/0012-dynamic-pitfalls.md` + `0014-provider-endpoint-flag-discipline.md` + `0015-classifier-routing.md` + `0017-policy-pitfall-conflict.md` (the auto-learning ADRs)

Operational caveats are in `AGENTS.md` § "Operational caveats" — re-read if anything is surprising.

For per-cloud mock fidelity (when adding handlers): each mock's own `README.md` documents the API-compatibility contract; the `examples/working/<svc>/` smoke harness + `coverage_matrix.yaml` are the per-bundle rule.
