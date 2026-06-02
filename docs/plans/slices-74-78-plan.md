# Slices 74–78 — AWS + Scaleway prompt-collapse parity, sustain ratchet, mock-gaps closure

Status: planned (2026-06-02)
Owner: next-session claude (designed for autonomous execution)
Follows: `slices-73-*.md` (one-off rules 9+12 retirement — closed 2026-06-02)

## Big picture

The N10→N11→N13 architecture has now retired **9 GCP phase2 rules** (CMEK + firewall + GKE + Cloud SQL + GCS + VPC + project_service + project_iam_member + the one inside S58's bundle). What remains in `prompts/gcp/phase2_generate_hcl.md` is the **destination** described in `slices-54-62-plan.md` § "Big picture": rules 1–8 (system contract) + 16 (region) + 17 (naming) — all Category C in ADR-0018.

This arc applies the same collapse to AWS + Scaleway, validates the post-collapse state with a fresh full sweep, and closes a handful of operational + mock-gap items that have accumulated.

Concretely:
- **S74** audits `prompts/aws/phase{2,3}_*.md` and retires 1–3 Category-A candidates.
- **S75** does the same for `prompts/scaleway/phase{2,3}_*.md`.
- **S76** runs the full 39-scenario deterministic sweep against the post-S73/S74/S75 prompt-collapsed state. Sustain-ratchet check.
- **S77** processes `docs/mock-gaps.md` — files one PR per high-impact gap against the matching mock repo (fakeaws / fakegcp / mockway).
- **S78** lands two operational ergonomic improvements: `make sweep-39` Makefile target + a defensive N3-classifier carve-out for `ACCESS_TOKEN_TYPE_UNSUPPORTED` on LLM-actionable resource types (the carry-over from S73's audit).

## Slices

| Slice | Title | Effort |
|---|---|---|
| S74 | AWS phase2/3 prompt audit + Category-A retirements | ~2-3 hr |
| S75 | Scaleway phase2/3 prompt audit + Category-A retirements | ~2-3 hr |
| S76 | Post-collapse 39-scenario deterministic sweep | ~1 hr |
| S77 | docs/mock-gaps.md PR triage — file 2-3 sibling-mock fixes | ~half-day |
| S78 | `make sweep-39` target + N3 classifier escape carve-out | ~1-2 hr |

**Total**: ~8–12 focused hours. One autonomous loop session.

## Standing rules

Inherit all rules from `slices-54-62-plan.md` § "Standing rules". Same authority to `gh pr merge --squash --admin --delete-branch`, same pitfall-pollution discipline, same per-PR scope.

Specifically per ADR-0018 for S74 + S75: classify each prescriptive rule into Category A (delete with no follow-up), B (replaced by `learned_from_diff` pitfall), or C (keep — system / scenario-bound / no machine-readable signal). Only retire A + B; document any C and move on.

## S74 — AWS phase2/3 prompt audit + Category-A retirements

### Motivation
S60 retired one AWS phase3 sub-bullet (RDS `deletion_protection`); S72 retired another (S3 bucket suffix). Several remaining bullets are likely Category-A — single-attribute or simple multi-attribute rules where `tofu validate` / `tofu plan` / OPA emits a clean error.

Remaining `prompts/aws/phase3_self_review.md` rule 3 sub-bullets:
- VPC + subnet ordering (Category B candidate — covered by existing pitfalls)
- IAM role + instance profile chain (mid-complexity)
- DB subnet group ordering (Category A candidate — plan error)
- Security group cycle avoidance (`aws_security_group_rule` not inline) (Category A — plan error)

Plus `prompts/aws/phase2_generate_hcl.md` rules 1–10 — re-audit per ADR-0018.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S74-T1 | Audit + classify every prescriptive rule in `prompts/aws/phase{2,3}_*.md`. Output a list with Category A/B/C per rule. | P1 | — |
| S74-T2 | Pick 1–3 Category-A candidates with the strongest machine-readable failure signal. Execute N11 7-step protocol per candidate. | P0 | S74-T1 |
| S74-T3 | Combined PR with the retirements. STATUS + NEXT_SESSION + ADR-0018 amendment if any new pattern surfaced. | P0 | S74-T2 |

### Exit criteria
- AWS audit table documented (Cat A/B/C per rule).
- 1–3 retirements landed.

## S75 — Scaleway phase2/3 prompt audit + Category-A retirements

### Motivation
Mirrors S74. S60 retired one Scaleway phase3 bullet (rule 7 — RDB + LB); S72 retired another (rule 6.d encryption_at_rest). Remaining candidates per `prompts/scaleway/phase{2,3}_*.md`:
- phase2 rule 9 ("Use private networking where required by constraints") — generic policy reminder, likely Category C
- phase3 rule 6.b (servers MUST have separate `scaleway_instance_private_nic`) — Category B (the `vpc_required.rego` policy emits a clear deny; existing `scaleway_instance_server` pitfalls cover the fix shape)
- phase3 rule 6.c (no public endpoints on databases) — Category B (`no_public_database.rego` enforces; pitfall pattern can absorb)

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S75-T1 | Audit + classify every prescriptive rule in `prompts/scaleway/phase{2,3}_*.md`. | P1 | — |
| S75-T2 | Pick 1–3 Category-A or B candidates. Execute N11 protocol per candidate. | P0 | S75-T1 |
| S75-T3 | Combined PR. STATUS + NEXT_SESSION update. | P0 | S75-T2 |

### Exit criteria
- Scaleway audit documented.
- 1–3 retirements landed.

## S76 — Post-collapse 39-scenario deterministic sweep

### Motivation
Mirrors S63 but post-S74/S75. Confirms the AWS + Scaleway retirements don't regress any of the 39 training scenarios. Builds the new baseline before S77/S78.

Reuse the sweep-script shape from S63 (`/tmp/sweep-s63.sh`) but call it via `infrafactory mock reset` (S67's CLI) instead of bare curls so the SeaweedFS cascade fires correctly.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S76-T1 | Run all 39 scenarios sequentially. Reset mocks via `infrafactory mock reset` between each. Capture per-scenario `terminal_reason` + iter count + any new `learned_from_diff*` entries. | P0 | S74, S75 |
| S76-T2 | Triage any regressions. Verify the established 39/39 baseline holds. | P0 | S76-T1 |
| S76-T3 | STATUS + NEXT_SESSION update. | P1 | S76-T2 |

### Exit criteria
- Single uninterrupted 39-scenario sweep passes (or every regression triaged).
- N10/N13 emissions audited per S55 pattern.

## S77 — docs/mock-gaps.md PR triage

### Motivation
`docs/mock-gaps.md` accumulates structured mock-server-side bug reports each time the N3 classifier routes a failure there. The file is the standing backlog for the matching sibling mock (fakeaws / fakegcp / mockway). This slice picks the 2–3 highest-impact entries and lands PRs against the sibling repo to close them.

This is the "pay down the mock-gap debt" loop that ADR-0015 envisioned. The goal isn't to drain the file in one session — it's to keep the queue moving so the inner feedback loop stays sharp.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S77-T1 | Read `docs/mock-gaps.md`. Rank entries by (a) recurrence count, (b) impact (how many scenarios block on them), (c) implementation effort. | P1 | — |
| S77-T2 | Pick the top 2–3. Implement + test against the sibling mock repo (typical pattern: missing handler route, missing field defaults, wire-shape correction). Open PR through the sibling's normal review flow. | P0 | S77-T1 |
| S77-T3 | Once landed, mark the corresponding `docs/mock-gaps.md` row as resolved. Re-run the originally-failing infrafactory scenario to confirm the gap is closed. | P0 | S77-T2 |

### Exit criteria
- 2–3 mock-gap PRs merged in the sibling repos.
- Originally-failing scenarios pass after the merge.

## S78 — `make sweep-39` + N3 classifier escape carve-out

### Motivation
Two operational items:

(a) **`make sweep-39`**: every prior arc reinvented the sweep harness as a `/tmp/sweep-*.sh` script. A canonical Makefile target makes future sweeps a one-liner and removes a class of "stale state from prior session" bugs (e.g. the duplicate-sweep concurrency mess from the S63 attempt).

(b) **N3 classifier carve-out for the GCP escape family**: from the S73 audit. If a future scenario hits the `ACCESS_TOKEN_TYPE_UNSUPPORTED` escape on `google_project_service` / `google_project_iam_member` AND the run gets stuck (3+ same-signature iters) before converging, today the failure routes to `docs/mock-gaps.md` because `IsMockActionable` matches `access_token_type_unsupported` regardless of resource. The fix: when the failing resource is one of the known LLM-actionable types (resource removal is the proper fix, not a mock-side change), N3 returns false so the lesson flows through `ExtractLearnedPitfall` instead. This is a defense-in-depth; the S73 retirement evidence suggests the LLM doesn't introduce these resources, but the carve-out catches the case if it does.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S78-T1 | Add `sweep-39` Makefile target wrapping a single-sweep script. Uses `infrafactory mock reset` between scenarios. | P1 | — |
| S78-T2 | Extend `IsMockActionable` with an LLM-actionable exclusion: when the failure detail matches `access_token_type_unsupported` AND a `google_project_service` / `google_service_networking_connection` / `google_project_iam_member` / `google_project_iam_binding` / `google_project_iam_policy` reference is in the detail, return false. Tests pinning both shapes. | P0 | — |
| S78-T3 | Combined PR. ADR-0015 amendment (carve-out rationale). | P0 | S78-T1, S78-T2 |

### Exit criteria
- `make sweep-39` works end-to-end.
- Two regression tests pin the N3 carve-out behavior.

## Why this order, in one paragraph

S74 + S75 first to land the AWS + Scaleway retirements while the GCP-collapse pattern is fresh. S76 next to validate the broader system against the post-collapse state — it's the sustain-ratchet that catches any subtle regression introduced by S74 or S75. S77 next because the mock-gap PRs against sibling repos can run in parallel with whatever GCP/AWS/Scaleway evolution comes after — they're independent. S78 last because the Makefile target benefits from observing the friction of running S76's sweep manually, and the classifier carve-out is the lowest-risk item in the arc.

## Autonomous-execution loop prompt

```
/loop until all 5 slices (S74-S78) in docs/plans/slices-74-78-plan.md are complete: every slice's exit criteria met, every PR merged with green CI, STATUS + NEXT_SESSION updated.

Read docs/NEXT_SESSION.md first for the prior arc's handoff, then docs/plans/slices-74-78-plan.md for the slice definitions. The standing rules from slices-54-62-plan.md apply (authority to merge, pitfall pollution discipline, mock rebuild discipline, STATUS+ADR+NEXT_SESSION updates, scope per PR).

Work slices in order S74 → S75 → S76 → S77 → S78. Each slice is one PR. S77 is the sibling-mock PR — land in fakeaws/fakegcp/mockway as appropriate, then merge from main and update mock-gaps.md.

Authority: open + merge PRs with `gh pr merge <N> --squash --admin --delete-branch` once CI is green, in all four repos. Discard auto-learning sweep pollution in pitfalls/*.yaml with `git checkout pitfalls/` — never hand-edit.

Stop only when: (a) all 5 slices complete OR (b) you genuinely cannot proceed (regression in main blocks every scenario AND fix-forward isn't obvious — document the blocker in NEXT_SESSION + stop).

Before stopping for any reason, update docs/NEXT_SESSION.md with: which slices closed, which are blocked, what next session should do FIRST.
```

## Fresh-context checklist

The autonomous prompt assumes the agent reads, in order:
1. `AGENTS.md`
2. `docs/NEXT_SESSION.md` (READ FIRST section)
3. This file (`docs/plans/slices-74-78-plan.md`)
4. `STATUS.md`
5. `BACKLOG.md`
6. `docs/decisions/0012-dynamic-pitfalls.md` + `0015-classifier-routing.md` + `0018-n11-retirement-criteria.md`
7. `docs/plans/slices-54-62-plan.md` § "Standing rules" + the N11 7-step protocol
