# Slices 89–93 — fakeaws Secrets Manager soft-delete + AWS phase2 audit

Status: planned (2026-06-03)
Owner: next-session claude (designed for autonomous execution)
Follows: `slices-84-88-plan.md` (gcp-full-stack convergence + panic gate — closed 2026-06-03)

## Big picture

S84–S88 left 38/39 deterministic with one persistent failure: `aws-full-stack` `stuck` on `orphan_check :: aws_secretsmanager_secret LLMSoftDelete`. The classifier already labels the subshape correctly. This is a small, well-shaped sibling-mock fix in fakeaws (same pattern as S77's KMS rotation immediate-delete).

In parallel, AWS phase2 has never been audited under the ADR-0018 N11 retirement framework. GCP phase2 collapsed from 17 → 9 rules across S54–S73; AWS phase3 had two Category-A retirements in S74. AWS phase2 is the last unaudited prompt — 10 numbered rules in `prompts/aws/phase2_generate_hcl.md`.

This arc tackles both:

1. **fakeaws Secrets Manager soft-delete** (S89-S90) — single sibling-mock PR + post-fix sweep targeting 39/39.
2. **AWS phase2 audit** (S91-S92) — classify every rule per ADR-0018, retire Category-A candidates.
3. **Arc close-out + reflection** (S93) — sweep, document, and explicitly evaluate whether the 5-slice scaffold is still right for fix-driven arcs (noted as an open question at S88 close-out).

## Slices

| Slice | Title | Effort |
|---|---|---|
| S89 | fakeaws Secrets Manager `DeleteSecret` immediate-hard-delete | ~30 min |
| S90 | Post-S89 39-scenario sweep + 39/39 confirmation | ~1 hr |
| S91 | AWS phase2 audit per ADR-0018 (Cat A/B/C per rule) | ~1 hr |
| S92 | Retire AWS phase2 Category-A candidates (0-2 expected) | ~1-2 hr |
| S93 | Post-retirement sweep + arc close-out + scaffold-question reflection | ~1-2 hr |

**Total**: ~5–7 focused hours. Smallest arc since S54.

## Standing rules

Inherit all rules from `slices-54-62-plan.md`, `slices-74-78-plan.md`, `slices-79-83-plan.md`, and `slices-84-88-plan.md`. Same merge authority, pitfall pollution discipline, mock rebuild discipline, scope per PR.

Specifically for this arc:

- **S89 is a fakeaws PR.** Land it in `../fakeaws` first, merge fakeaws-main, then rebuild + restart fakeaws locally so the sweep picks it up.
- **AWS phase2 audit calibration.** Most of the 10 rules look Category C on a first read (system contract — provider pinning, file organisation, variable defaults). Realistic outcome: 0-2 retirements. Don't force retirements — Category C is a valid landing.
- **S93 reflection is mandatory.** The S88 close-out flagged that the 5-slice scaffold is starting to feel heavy for arcs where most work is 1-2 substantive fixes + 2-3 documentation slices. S93 either commits to keeping the scaffold (with rationale) or proposes a lighter shape for the next arc.

## S89 — fakeaws Secrets Manager `DeleteSecret` immediate-hard-delete

### Motivation
`aws-full-stack` (S88 sweep) hit `orphan_check :: aws_secretsmanager_secret LLMSoftDelete` — the only failure in the 38/39 sweep. fakeaws's `DeleteSecret` handler currently mirrors real AWS by leaving the secret in `PendingDeletion` state with a recovery window; the orphan_check sees the entry in `/mock/state` after destroy and flags it as a leftover.

Same fix shape as S77's KMS rotation (also a "real AWS uses a pending-deletion window but for test purposes immediate hard-delete works"): drop the secret from the store immediately on `DeleteSecret`. Optional: honor `force_delete_without_recovery: true` request flag if the terraform-provider-aws sends it; otherwise immediate-delete unconditionally for the mock.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S89-T1 | Locate fakeaws Secrets Manager handler. Read current `DeleteSecret` behavior. | P1 | — |
| S89-T2 | Either (a) immediate hard-delete unconditionally, or (b) honor `ForceDeleteWithoutRecovery: true` from the request body (the v5 provider sends this for `force_destroy: true` aws_secretsmanager_secret resources). Pick (a) unless the request shape already drives (b). | P0 | S89-T1 |
| S89-T3 | Regression test pinning Create → Delete → ListSecrets returns empty. Plus 404 on subsequent DescribeSecret. | P0 | S89-T2 |
| S89-T4 | fakeaws PR. Once merged, `make fakeaws-restart` + re-run aws-full-stack locally to confirm orphan_check passes. Trim or note the `LLMSoftDelete` row in `docs/mock-gaps.md` as resolved (file is git-untracked, regenerates on next sweep). | P0 | S89-T3 |

### Exit criteria
- fakeaws PR merged with green CI.
- aws-full-stack converges target_reached on a single end-to-end run.

## S90 — Post-S89 39-scenario sweep + 39/39 confirmation

### Motivation
Validate S89 against the full scenario set. Pre-S89 baseline: 38/39 (aws-full-stack was the one). Target: 39/39.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S90-T1 | `make mocks-restart` to pick up the post-S89 fakeaws binary. | P0 | S89 |
| S90-T2 | `make sweep-39`. Capture `/tmp/sweep-39/summary.tsv` + `panics.log`. | P0 | S90-T1 |
| S90-T3 | Triage. If 39/39: arc deltas updated. If 38/39 with a new shape: that becomes the next-arc target. | P0 | S90-T2 |
| S90-T4 | STATUS + NEXT_SESSION update. Discard sweep pollution per protocol. | P1 | S90-T3 |

### Exit criteria
- Single uninterrupted sweep + panic gate clean.
- New baseline pass count documented.

## S91 — AWS phase2 audit per ADR-0018

### Motivation
GCP phase2 collapsed from 17 → 9 between S54 and S73. AWS phase3 had two Category-A retirements in S74. AWS phase2 has never been audited. The file is short (`prompts/aws/phase2_generate_hcl.md`, ~70 lines, 10 numbered rules) so the audit is itself short.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S91-T1 | Read every numbered rule. For each: classify as Category A (delete, no follow-up), B (replaced by `learned_from_diff` pitfall), or C (keep — system contract / scenario-bound / no machine-readable signal). | P0 | — |
| S91-T2 | Document the table (rule → category → rationale) in a slice-internal note or PR description; doesn't need its own file. | P1 | S91-T1 |
| S91-T3 | Identify 0-2 retirement candidates (Cat A + B). If none: that's the arc's outcome — document and skip S92. | P0 | S91-T2 |

### Exit criteria
- Every AWS phase2 rule classified.
- Retirement plan (or "no candidates") documented.

## S92 — Retire AWS phase2 Category-A candidates (0-2 expected)

### Motivation
Execute the N11 7-step protocol on whatever S91 surfaced. The protocol: pick rule → delete from prompt → blank corresponding pitfall (or confirm none exists) → re-run impacted scenarios → assess organic re-learning → either confirm retirement or restore.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S92-T1 | For each retirement candidate from S91: execute the N11 protocol (delete rule from prompt, run impacted scenarios, check whether the failure organic-learns into a pitfall). | P0 | S91 |
| S92-T2 | If a retirement breaks an impacted scenario: restore the rule. Document why it's Category C in retrospect. | P0 | S92-T1 |
| S92-T3 | Combined PR with the retirements + audit table. Per ADR-0018 amendment if a new pattern surfaced. | P0 | S92-T2 |

### Exit criteria
- 0-2 retirements landed (or "no candidates" formally documented).
- No regression in impacted scenarios.

## S93 — Post-retirement sweep + arc close-out + scaffold reflection

### Motivation
Final sweep validates S89+S92 together. The scaffold reflection: was the 5-slice template still worth it for an arc where most substantive work was 1 mock fix + 0-2 prompt edits + 3 documentation slices?

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S93-T1 | `make sweep-39` post-S92. Verify no regression from S91/S92 prompt edits. | P0 | S92 |
| S93-T2 | STATUS + NEXT_SESSION update. ARCHIVE per-slice narrative. | P0 | S93-T1 |
| S93-T3 | **Scaffold reflection**: explicit written assessment in NEXT_SESSION. Either: "5-slice scaffold still right because X" OR "next arc should be Y-shape (e.g. 1-3 slices, drop the post-arc-sweep slice when it's deterministic)" — propose a concrete lighter shape. | P0 | S93-T2 |

### Exit criteria
- Single uninterrupted sweep.
- Arc close-out with explicit scaffold decision for the next arc.

## Why this order, in one paragraph

S89 first because it's the smallest fix with the biggest visible payoff (39/39). S90 immediately after to validate before changing anything else. S91+S92 second because the audit is independent of the mock fix and benefits from a clean post-S90 baseline. S93 last because the post-retirement sweep needs both deltas in place. The scaffold reflection lands in S93 specifically — by then we'll have lived through another 5-slice arc and have fresh evidence about whether the template is working.

## Autonomous-execution loop prompt

```
/loop until all 5 slices (S89-S93) in docs/plans/slices-89-93-plan.md are complete: every slice's exit criteria met, every PR merged with green CI, STATUS + NEXT_SESSION updated.

Read docs/NEXT_SESSION.md first for the prior arc's handoff, then docs/plans/slices-89-93-plan.md for the slice definitions. The standing rules from slices-54-62-plan.md, slices-74-78-plan.md, slices-79-83-plan.md, AND slices-84-88-plan.md apply (authority to merge, pitfall pollution discipline, mock rebuild discipline, STATUS+ADR+NEXT_SESSION updates, scope per PR).

Work slices in order S89 → S90 → S91 → S92 → S93. S89 is a sibling-mock PR in fakeaws — land in fakeaws first, then rebuild + verify, then move on. S92 may close as "no retirement candidates" if S91 finds AWS phase2 is all Category C — that's a valid outcome.

Authority: open + merge PRs with `gh pr merge <N> --squash --admin --delete-branch` once CI is green, in all four repos. Discard auto-learning sweep pollution in pitfalls/*.yaml with `git checkout pitfalls/` — never hand-edit.

S93 MUST include a written assessment of whether the 5-slice scaffold is still right for fix-driven arcs — either confirm with rationale OR propose a concrete lighter shape for the next arc.

Stop only when: (a) all 5 slices complete OR (b) you genuinely cannot proceed (regression in main blocks every scenario AND fix-forward isn't obvious — document the blocker in NEXT_SESSION + stop).

Before stopping for any reason, update docs/NEXT_SESSION.md with: which slices closed, which are blocked, what next session should do FIRST.
```

## Fresh-context checklist

The autonomous prompt assumes the agent reads, in order:
1. `AGENTS.md`
2. `docs/NEXT_SESSION.md` (READ FIRST section)
3. This file (`docs/plans/slices-89-93-plan.md`)
4. `STATUS.md`
5. `docs/status/ARCHIVE.md` § "2026-06-03 S84–S88" (for the aws-full-stack context that motivates S89)
6. `docs/decisions/0018-n11-retirement-criteria.md` (the N11 protocol that S92 executes)
7. `prompts/aws/phase2_generate_hcl.md` (the 10 rules S91 audits)
