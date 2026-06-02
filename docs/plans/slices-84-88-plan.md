# Slices 84–88 — gcp-full-stack convergence + fakegcp panic triage

Status: planned (2026-06-02)
Owner: next-session claude (designed for autonomous execution)
Follows: `slices-79-83-plan.md` (sibling-mock drainage + N3 carve-out validation — closed 2026-06-02)

## Big picture

S79–S83 landed 38/39 deterministic, with one persistent failure: `gcp-full-stack` `repair_budget_exhausted` on `google_service_networking_connection` `ACCESS_TOKEN_TYPE_UNSUPPORTED`. The S78 N3 carve-out routed the failure into `pitfalls/gcp.yaml` correctly (validated organically in S81), but the underlying escape itself was not fixed — `service_networking_custom_endpoint` IS already injected by `internal/cli/generate_command.go:274`, so the question is *why the override doesn't redirect the request*.

This arc tackles two goals in parallel-friendly order:

1. **gcp-full-stack convergence** (S84-S85) — investigate the provider-config override pattern, land the fix. Outcome: 39/39 deterministic.
2. **fakegcp panic triage** (S86-S87) — drain the 4-5 `plugin did not respond` entries in `docs/mock-gaps.md` pointing at fakegcp panics. Outcome: sibling-mock arc progress, fewer scenarios at risk of mid-run crashes.

Then a post-fix 39-scenario sweep validates both (S88).

Concretely:
- **S84**: read iterations/1..5 of the last gcp-full-stack run, identify what's overriding `service_networking_custom_endpoint` (per-resource provider alias? v5 SDK preflight path? fakegcp's `Projects.GetProject` returning 401?). Timeboxed: 2 hours of investigation, then either proceed to S85 with a fix plan or file findings and skip to S86.
- **S85**: Land the gcp-full-stack fix. Scope depends on S84:
  - If LLM-side (per-resource provider block): add a pitfall against the pattern.
  - If infrafactory-side (provider-block injection bug): fix `generate_command.go`.
  - If fakegcp-side (preflight 401): fix the relevant fakegcp handler.
- **S86**: Triage the 4-5 fakegcp `plugin did not respond` mock-gaps entries (`google_kms_crypto_key_iam_member`, `google_container_node_pool`, `google_compute_instance`, `google_sql_database_instance`). Identify the panic source (request shape, mismatch, missing route).
- **S87**: Fix the highest-impact panic. Likely a single fakegcp PR.
- **S88**: Post-fix 39-scenario sweep + arc close-out.

## Slices

| Slice | Title | Effort |
|---|---|---|
| S84 | gcp-full-stack provider-config investigation (timeboxed) | ~2 hr |
| S85 | gcp-full-stack fix (scope from S84) | ~1-4 hr |
| S86 | fakegcp `plugin did not respond` triage | ~2 hr |
| S87 | Fix highest-impact fakegcp panic | ~2-4 hr |
| S88 | Post-fix 39-scenario sweep + arc close-out | ~1-2 hr |

**Total**: ~8–14 focused hours. One autonomous loop session.

## Standing rules

Inherit all rules from `slices-54-62-plan.md` § "Standing rules", `slices-74-78-plan.md` § "Standing rules", and `slices-79-83-plan.md` § "Standing rules". Same authority to `gh pr merge --squash --admin --delete-branch`, same pitfall-pollution discipline (`git checkout pitfalls/` to discard sweep noise — never hand-edit), same per-PR scope, same mock-rebuild discipline.

Specifically for this arc:
- **S84 timebox is firm.** If the gcp-full-stack root cause isn't clear after 2 hours of investigation, file findings in a `docs/investigations/gcp-full-stack-2026-06-02.md` artifact and skip to S86. Don't block the arc on a depth-of-investigation problem; the fakegcp work in S86-S87 is independent and ships value regardless.
- **S85 is conditional.** Its content depends entirely on S84's findings. The plan can't predict the fix shape; it commits to landing whatever S84 surfaces.
- **S87 picks one panic.** Don't try to drain all 4-5 in this arc — the rule-of-three from `feedback_orphan_check_extractor.md` applies: fix one, observe the pattern, generalize only if it recurs.

## S84 — gcp-full-stack provider-config investigation (timeboxed)

### Motivation
S81's only failure. The full error: `Failed to find Service Networking Connection, err: Failed to retrieve project, pid: infrafactory-test, err: googleapi: Error 401: ... "reason": "ACCESS_TOKEN_TYPE_UNSUPPORTED" ... "method": "google.cloudresourcemanager.v1.Projects.GetProject", "service": "cloudresourcemanager.googleapis.com"`.

Known facts:
- `internal/cli/generate_command.go:274` injects `service_networking_custom_endpoint = "%[1]s/"`.
- `internal/cli/generate_command.go:229` injects `resource_manager_v3_custom_endpoint = "%[1]s/"`.
- `internal/cli/generate_command.go:210` sets `user_project_override = false`.
- The carve-out from S78 routes this failure into `pitfalls/gcp.yaml` (validated in S81) — but the carve-out is a routing fix, not a root-cause fix.

Open questions:
1. Is the LLM-generated HCL defining a per-resource `provider` block or alias that overrides the global config?
2. Is the v5 SDK calling a different endpoint than the overrides target (e.g. a v3 vs v1 mismatch)?
3. Is fakegcp's `Projects.GetProject` returning 401 itself for some reason (the comment at lines 201-216 says `user_project_override = false` should prevent this — but it must not)?

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S84-T1 | Read `.infrafactory/runs/gcp-full-stack/<last-failed>/iterations/{1..5}/generated/*.tf`. Look for: per-resource `provider` blocks/aliases, custom provider sources, any HCL that overrides the global provider config. | P0 | — |
| S84-T2 | If T1 finds an override: document the pattern + propose a pitfall. Skip to S85. | P0 | S84-T1 |
| S84-T3 | If T1 finds no override: instrument the request path. `curl` `cloudresourcemanager.googleapis.com/v1/projects/infrafactory-test` against fakegcp's preflight handler. Check what it returns + why. | P0 | S84-T2 |
| S84-T4 | If T3 surfaces a fakegcp handler bug or missing route: document the fix + proceed to S85. | P0 | S84-T3 |
| S84-T5 | Timebox: if at 2 hours of investigation the root cause is still unclear, file findings in `docs/investigations/gcp-full-stack-2026-06-02.md` (or update the existing) and skip to S86. | P1 | — |

### Exit criteria
- Either a documented fix path → proceed to S85.
- Or a documented dead-end → skip to S86 with the investigation artifact committed.

## S85 — gcp-full-stack fix (scope from S84)

### Motivation
Lands whatever S84 surfaces. Conditional content.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S85-T1 | If LLM-side: add a pitfall in `pitfalls/gcp.yaml` (commit it as `source: learned` with a hand-written rule). Per protocol this is allowed if the pattern is observed and documented — the protocol forbids hand-editing for sweep-noise reasons, not for deliberate seed-vs-architecture decisions. (Distinct from M91's `TestPitfallsNoHumanSeeding` — that test rejects `source: seed` and `source: static`; `source: learned` is allowed.) | P0 | S84 |
| S85-T2 | If infrafactory-side: edit `internal/cli/generate_command.go` to fix the provider injection. Add regression test that exercises the override path. | P0 | S84 |
| S85-T3 | If fakegcp-side: file a fakegcp PR with the handler fix + regression test. | P0 | S84 |
| S85-T4 | Re-run `gcp-full-stack` end-to-end to confirm convergence. | P0 | S85-T1, T2, or T3 |
| S85-T5 | Combined PR per the chosen scope. STATUS + NEXT_SESSION update. | P0 | S85-T4 |

### Exit criteria
- gcp-full-stack converges in ≤ 4 iterations on a fresh run.
- The fix is durable across the next sweep (validated in S88).

## S86 — fakegcp `plugin did not respond` triage

### Motivation
`docs/mock-gaps.md` carries 4-5 `plugin did not respond` entries against fakegcp on specific resource types: `google_kms_crypto_key_iam_member`, `google_container_node_pool`, `google_compute_instance`, `google_sql_database_instance`. These indicate fakegcp panics during request handling — the OpenTofu provider's Go gRPC layer can't get a response back. Without a panic stack trace it's not clear which handler is at fault.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S86-T1 | For each `plugin did not respond` entry: re-run the originating scenario in isolation, capture fakegcp stderr/log output, look for panic stack traces. | P0 | — |
| S86-T2 | Identify the common shape (or unique shapes) of request that triggers the panic. Likely: missing field, unexpected enum, parent-resource lookup failing. | P0 | S86-T1 |
| S86-T3 | Rank the panics by (a) scenario impact, (b) fix effort. Pick one for S87. | P0 | S86-T2 |
| S86-T4 | Document findings in `docs/investigations/fakegcp-panics-2026-06-02.md`. | P1 | S86-T2 |

### Exit criteria
- Each of the 4-5 entries has a documented panic source (or "not reproducible — flake").
- One target picked for S87.

## S87 — Fix the highest-impact fakegcp panic

### Motivation
S86 picks the target; S87 implements + tests + ships the fix in fakegcp.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S87-T1 | Implement the fix in `../fakegcp`. | P0 | S86-T3 |
| S87-T2 | Add a regression test in `../fakegcp` covering the panic shape (handler-level test, not e2e). | P0 | S87-T1 |
| S87-T3 | Open fakegcp PR. Once merged, rebuild + re-run the originally-failing scenario to confirm the panic is gone. | P0 | S87-T2 |
| S87-T4 | Update `docs/mock-gaps.md` (or note the entry as resolved). | P1 | S87-T3 |

### Exit criteria
- fakegcp PR merged with green CI.
- Originally-failing scenario converges (or progresses further than before).

## S88 — Post-fix 39-scenario sweep + arc close-out

### Motivation
Mirrors S81 but post-S85+S87. Confirms both fixes hold and surfaces what's next.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S88-T1 | `make sweep-39`. Capture `/tmp/sweep-39/summary.tsv`. | P0 | S85, S87 |
| S88-T2 | Verify gcp-full-stack converges + the S87-fixed scenario passes. Triage any regressions. | P0 | S88-T1 |
| S88-T3 | Discard sweep pollution per protocol. Document the new baseline pass count. | P0 | S88-T2 |
| S88-T4 | STATUS + NEXT_SESSION update with arc close-out. Per-slice narrative in `docs/status/ARCHIVE.md`. | P0 | S88-T3 |

### Exit criteria
- Single uninterrupted 39-scenario sweep.
- New deterministic baseline documented (target: 39/39).
- Arc close-out lands.

## Why this order, in one paragraph

S84 first because it's the highest-leverage open question and might surface a pattern that affects other scenarios. S85 immediately after to capitalize on S84's investigation context. S86 next because fakegcp work is independent of gcp-full-stack — if S85 succeeds, S86 starts fresh; if S85 surfaced a deeper architectural issue, S86 still ships value. S87 picks one panic per the rule-of-three. S88 last because it validates both deltas in one sweep.

## Autonomous-execution loop prompt

```
/loop until all 5 slices (S84-S88) in docs/plans/slices-84-88-plan.md are complete: every slice's exit criteria met, every PR merged with green CI, STATUS + NEXT_SESSION updated.

Read docs/NEXT_SESSION.md first for the prior arc's handoff, then docs/plans/slices-84-88-plan.md for the slice definitions. The standing rules from slices-54-62-plan.md, slices-74-78-plan.md, AND slices-79-83-plan.md apply (authority to merge, pitfall pollution discipline, mock rebuild discipline, STATUS+ADR+NEXT_SESSION updates, scope per PR).

Work slices in order S84 → S85 → S86 → S87 → S88. Each slice is one PR (or in S84/S86, one investigation artifact + skip-or-proceed decision). S87 is a sibling-mock PR in fakegcp — land in fakegcp first, then rebuild + verify, then merge documentation in main.

Authority: open + merge PRs with `gh pr merge <N> --squash --admin --delete-branch` once CI is green, in all four repos. Discard auto-learning sweep pollution in pitfalls/*.yaml with `git checkout pitfalls/` — never hand-edit (except S85-T1, which IS a deliberate pitfall add).

S84 timebox is firm: 2 hours of investigation, then either proceed to S85 with a fix plan or file findings and skip to S86. Don't let depth-of-investigation block the arc.

Stop only when: (a) all 5 slices complete OR (b) you genuinely cannot proceed (regression in main blocks every scenario AND fix-forward isn't obvious — document the blocker in NEXT_SESSION + stop).

Before stopping for any reason, update docs/NEXT_SESSION.md with: which slices closed, which are blocked, what next session should do FIRST.
```

## Fresh-context checklist

The autonomous prompt assumes the agent reads, in order:
1. `AGENTS.md`
2. `docs/NEXT_SESSION.md` (READ FIRST section)
3. This file (`docs/plans/slices-84-88-plan.md`)
4. `STATUS.md`
5. `docs/status/ARCHIVE.md` § "2026-06-02 S79–S83" (for the carve-out validation context that feeds S84)
6. `docs/decisions/0012-dynamic-pitfalls.md` + `0015-classifier-routing.md` + `0018-n11-retirement-criteria.md`
7. `internal/cli/generate_command.go` lines 196-280 (the GCP provider-block injection that S84 must investigate)
