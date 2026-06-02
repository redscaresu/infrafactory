# Slices 63–67 — post-collapse validation + flake triage + harness ratchet

Status: planned (2026-06-02)
Owner: next-session claude (designed for autonomous execution)
Follows: `slices-54-62-plan.md` (sustain + prompt-collapse arc — closed)

## Big picture

The S54–S62 arc collapsed GCP phase2 from 17 → 11 prescriptive rules and landed
N10 + N13 as the auto-derivation backbone. This arc closes the validation loop:
re-prove 39/39 deterministic with the new prompt-collapsed state, exercise N13 in
production for the first time, fix two carried-forward LLM/mock flakes, and harden
the sweep harness against the SeaweedFS state-leak that surfaced in S54.

Concretely:
- Confirm no regression from the six prompt retirements (S56–S60).
- N13 production validation — first organic `learned_from_diff_avoid` emission +
  S55-style audit.
- Two specific flakes that S57/S59 surfaced get root-caused: `deletion_policy`
  hallucination on `google_cloud_run_v2_service`, and `google_apikeys_key` mock
  gap on gcp-full-stack.
- An `infrafactory mock reset` CLI command + permanent `cmd/n10extract` so the
  sweep harness + retirement workflow stop relying on ad-hoc helpers.

## Slices

| Slice | Title | Effort |
|---|---|---|
| S63 | Post-collapse deterministic 39-scenario sweep | ~1 hr |
| S64 | N13 first-production audit | ~1 hr |
| S65 | gcp-cloud-run `deletion_policy` hallucination triage | ~1-2 hr |
| S66 | gcp-full-stack `google_apikeys_key` mock gap | ~half-day |
| S67 | Sweep harness sustain ratchet + N10 tool permanence | ~2 hr |

**Total**: ~6–10 focused hours. One autonomous loop session.

## Standing rules

All standing rules from `slices-54-62-plan.md` apply — re-read that file's
"Standing rules for autonomous execution" before starting. Specifically:

- **Authority to merge** PRs with `gh pr merge <N> --squash --admin --delete-branch`
  once CI is green (in all four repos: `infrafactory`, `../mockway`, `../fakegcp`,
  `../fakeaws`).
- **Pitfall pollution discipline**: never hand-edit `pitfalls/*.yaml`. Discard
  sweep noise with `git checkout pitfalls/`. The N10/N13 extractors are the only
  legitimate authors.
- **Mock + binary rebuild discipline**: after any sibling-mock merge, restart the
  mock; after any `internal/` / `prompts/` change, `make build`.
- **STATUS + ADR + NEXT_SESSION updates** are MANDATORY before merging any PR
  that touches `internal/` or `cmd/infrafactory/`.
- **Per-PR scope**: one slice per branch; PR title format `S<N>: <title>`.

## S63 — Post-collapse deterministic 39-scenario sweep

### Motivation
Six GCP prompt rules retired (S56–S59), three cross-cloud rules retired (S60),
N13 extractor wired (S61). The S54 sweep ran against the pre-retirement
baseline. S63 re-runs the same 39 scenarios against the new collapsed-prompt
state to confirm no regression. Also builds the baseline that S64+ audits.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S63-T1 | Reset all mocks (including SeaweedFS — see S67 for the systematic fix). Run all 39 training scenarios sequentially. Capture per-scenario `terminal_reason` + iteration count + any new `learned_from_diff` / `learned_from_diff_avoid` entries. | P0 | — |
| S63-T2 | Triage any regressions. If a scenario that passed in S54's sweep now fails, root-cause: prompt-retirement side-effect? LLM non-determinism? Sibling-mock change? File a follow-up if non-blocking. | P0 | S63-T1 |
| S63-T3 | STATUS + NEXT_SESSION update with post-sweep state + comparison table vs S54. | P1 | S63-T2 |

### Exit criteria
- Single uninterrupted 39-scenario sweep passes (or every regression triaged + filed).
- New baseline captured for S64.

## S64 — N13 first-production audit

### Motivation
N13 (`ExtractPrescriptiveAvoid`) shipped in S61 with synthetic tests only. The
S63 sweep is the first real exercise — any `learned_from_diff_avoid` entries it
produces are the first production output. S55 audited N10's first entries the
same way; S64 mirrors that for N13.

If the audit produces a `google_project_iam_member` (or `google_project_service`)
avoid entry, the rule 12 retirement (NEXT_SESSION carry-over item) lands in this
PR.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S64-T1 | For every `source: learned_from_diff_avoid` entry post-S63, hand-verify: (a) removed attribute/resource actually correlated with the failure clearance, (b) rule wording is actionable (specific identifier, not generic), (c) under the 600-byte cap. | P0 | S63 |
| S64-T2 | False-positive regression test: any entry that fails T1 gets a pinning test in `prescriptive_extractor_test.go` + a heuristic tightening. | P0 | S64-T1 |
| S64-T3 | Conditional: if a `google_project_iam_member` or `google_project_service` avoid entry landed organically, retire GCP phase2 rule 12 (or rule 9) via the 7-step protocol in the same PR. ADR-0018 Category B. | P1 | S64-T1 |

### Exit criteria
- Every avoid entry hand-verified.
- False positives have regression tests.
- Conditional rule retirement if an organic entry covers it.

## S65 — gcp-cloud-run `deletion_policy` hallucination triage

### Motivation
S59's validation hit gcp-cloud-run getting stuck after 2 iters because the LLM
hallucinated `deletion_policy = "DELETE"` on `google_cloud_run_v2_service` — an
attribute that doesn't exist. `tofu validate` reports the error clearly; the LLM
should self-correct in iter 2 but stuck-detection (same failure signature
twice) killed the loop before it could.

This isn't a one-off — it's a real fragility in the dynamic loop. Three possible
fixes:
1. **Rely on N13** to capture the avoid pattern from a successful run. Won't help
   if stuck-detection kills the run before success.
2. **Raise the stuck-detection threshold** from 2→3 iters for failures whose
   detail explicitly says "Unsupported argument" — those are highly machine-
   readable and a 3rd iter is likely to succeed.
3. **Add a phase2 prompt rule** about Cloud Run unsupported args (Category C —
   no machine-readable signal that solves it pre-iter).

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S65-T1 | Reproduce gcp-cloud-run stuck a few times to confirm consistency. If only 1-in-5, escalate to "investigate LLM consistency"; if 4-in-5, treat as systemic. | P0 | — |
| S65-T2 | If systemic: implement option (2) — value-aware stuck-detection. Specifically: when `feedback.IsStuck` would fire AND the failure detail contains "Unsupported argument", give one more iteration. | P0 | S65-T1 |
| S65-T3 | Re-run gcp-cloud-run 5× to confirm convergence. | P0 | S65-T2 |
| S65-T4 | PR + STATUS + ADR amendment to 0012 (or new ADR if the stuck-detection change is meaty). | P0 | S65-T3 |

### Exit criteria
- gcp-cloud-run converges deterministically OR the flakiness is well-understood + accepted as LLM-side.

## S66 — gcp-full-stack `google_apikeys_key` mock gap

### Motivation
S57's gcp-full-stack repro hit `repair_budget_exhausted` on iter 5 because the
LLM introduced `google_apikeys_key` — a resource that hits `apikeys.googleapis.com`,
not implemented by fakegcp. The LLM produces this non-deterministically.

Per `feedback_mock_design.md`, the fix is at-source: implement the resource in
fakegcp. Alternative: tighten the scenario architecture plan so the LLM doesn't
reach for it. Pick the lower-cost path that doesn't break the inner feedback
loop.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S66-T1 | Audit `apikeys.googleapis.com` API surface for the minimal handler set needed (Create + Get + Delete of a Key). 1-2 hours. | P1 | — |
| S66-T2 | Decision: implement in fakegcp (preferred) OR add a `Do NOT use` note to gcp-full-stack's architecture plan / phase1 prompt. | P0 | S66-T1 |
| S66-T3 | If fakegcp: implement + tests + PR through normal flow. If scenario plan: pin the change. | P0 | S66-T2 |
| S66-T4 | Re-run gcp-full-stack 5× to confirm stability. | P0 | S66-T3 |

### Exit criteria
- gcp-full-stack converges deterministically across 5 consecutive runs.

## S67 — Sweep harness sustain ratchet + N10 tool permanence

### Motivation
Two operational carry-overs:

- **SeaweedFS state-leak (S54)**: the sweep harness used bare-curl `/mock/reset`
  which doesn't cascade to SeaweedFS, so pre-sweep S3 buckets blocked
  aws-full-stack. Only `cloudMockStateRouter.Reset` cascades (via
  `resetS3Backend` at `internal/cli/s3_state.go:82`).
- **N10 forced-extract helper**: `cmd/n10extract` was written + removed in the
  2026-06-02 loop session for N11 step 2 fallback. If forced extraction recurs
  (likely), promote it to a permanent CLI command.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S67-T1 | Add `infrafactory mock reset` CLI command wrapping `cloudMockStateRouter.Reset`. Takes a `--cloud` flag (defaults to all). Cascades SeaweedFS via the existing s3 carve-out. | P0 | — |
| S67-T2 | Update sweep harness pattern (the inline scripts under `/tmp/sweep-*.sh` and the docs in `feedback_sweep_protocol.md`) to recommend `infrafactory mock reset` over bare curls. | P1 | S67-T1 |
| S67-T3 | Optional: land `cmd/n10extract/main.go` as a permanent command that reads `--run-dir` + `--scenario` flags and emits a candidate `LearnedPitfall` snippet for triage. Useful for N11 step 2 retirements where the organic learn loop hasn't fired yet. | P2 | — |
| S67-T4 | STATUS + NEXT_SESSION update. | P0 | S67-T1, S67-T2 |

### Exit criteria
- `infrafactory mock reset` exists + tested (unit + smoke).
- Sweep harness no longer needs SeaweedFS-specific carve-out code.

## Why this order, in one paragraph

S63 first because every later slice depends on its outcome (regressions block S64;
the deletion_policy hallucination might be auto-fixed by N13 organic emission,
which moots S65; the apikeys gap might or might not surface depending on LLM
non-determinism, which scopes S66). S64 next while the S63 entries are fresh.
S65 + S66 are the two carried-forward flakes — independent, parallelizable but
sequenced for clean PR scope. S67 last because it's operational hardening that
benefits from all the friction observed in S63–S66.

## Autonomous-execution loop prompt

Paste this into a fresh Claude session to start the arc:

```
/loop until all 5 slices (S63-S67) in docs/plans/slices-63-67-plan.md are complete: every slice's exit criteria met, every PR merged with green CI, STATUS + NEXT_SESSION updated.

Read docs/NEXT_SESSION.md first for the prior arc's handoff, then docs/plans/slices-63-67-plan.md for the slice definitions. The standing rules from slices-54-62-plan.md apply (authority to merge, pitfall pollution discipline, mock rebuild discipline, STATUS+ADR+NEXT_SESSION updates, scope per PR).

Work slices in order S63 → S64 → S65 → S66 → S67. Each slice is one PR (S64 may be a no-op PR if no organic learned_from_diff_avoid entries surfaced — capture that outcome in NEXT_SESSION).

Authority: open + merge PRs with `gh pr merge <N> --squash --admin --delete-branch` once CI is green, in all four repos. Discard auto-learning sweep pollution in pitfalls/*.yaml with `git checkout pitfalls/` — never hand-edit.

Stop only when: (a) all 5 slices complete OR (b) you genuinely cannot proceed (regression in main blocks every scenario AND fix-forward isn't obvious — document the blocker in NEXT_SESSION + stop).

Before stopping for any reason, update docs/NEXT_SESSION.md with: which slices closed, which are blocked, what next session should do FIRST.
```

## Fresh-context checklist

The autonomous prompt assumes the agent reads, in order:
1. `AGENTS.md`
2. `docs/NEXT_SESSION.md` (the §"READ FIRST" pointer)
3. This file (`docs/plans/slices-63-67-plan.md`)
4. `STATUS.md`
5. `docs/decisions/0012-dynamic-pitfalls.md` + `0018-n11-retirement-criteria.md`
   (the auto-learning + retirement ADRs)
6. `docs/plans/slices-54-62-plan.md` for the prior arc's standing rules
