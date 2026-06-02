# Slices 79–83 — sibling-mock drainage + carve-out validation + N2 pruning

Status: planned (2026-06-02)
Owner: next-session claude (designed for autonomous execution)
Follows: `slices-74-78-plan.md` (AWS+Scaleway collapse + Makefile + N3 carve-out — closed 2026-06-02)

## Big picture

S74–S78 closed both major prompt-collapse arcs (GCP phase2, AWS+Scaleway phase3) and landed the canonical `make sweep-39` harness + the first targeted carve-out in the N3 mock-actionable classifier. The deterministic baseline is 37/39, with both failures routing cleanly to `docs/mock-gaps.md`.

This arc shifts focus from prompt-side ratcheting to **mock-side ratcheting** + **post-collapse validation**:

- **S79** drains the one known fakeaws gap (KMS tag persistence). Same sibling-mock pattern S77 validated.
- **S80** investigates + fixes the `aws_s3_bucket_public_access_block` 501. **Important architectural note**: S3 data-plane traffic routes to SeaweedFS (port 9090, per `infrafactory.yaml`), NOT fakeaws — so the 501 originates from SeaweedFS, which doesn't model `?publicAccessBlock`. fakeaws's `PutPublicAccessBlock` handler exists (handlers/s3.go) but is dead code in the default config. The fix is most likely an infrafactory-side **routing shim** that proxies `?publicAccessBlock` (and any other subresources SeaweedFS doesn't handle) to fakeaws while bucket/object data-plane stays on SeaweedFS. Confirm direction with a one-hour spike before scoping the PR.
- **S81** is a fresh 39-scenario sweep that exercises (a) the S79+S80 fixes — expecting 38–39/39 — and (b) the S78 N3 GCP-escape carve-out, by checking whether N13 actually learns the avoid pattern when the carve-out fires.
- **S82** is an N2 pruning audit — sweep `pitfalls/*.yaml` for entries that have been fully replaced by prompt or OPA changes between S54 and S78, and add a ratchet test pinning the remaining set.
- **S83** absorbs whatever S81 surfaces: if S81 hits 39/39, drain the next 1–2 sibling-mock gaps from `docs/mock-gaps.md`; otherwise file PRs against the regressions.

Concretely:
- **S79**: fakeaws KMS tag persistence — Persist KMS tags through TagResource / UntagResource / ListResourceTags so `aws_kms_key.tags` round-trips. Same shape as S77's rotation fix.
- **S80**: investigate + fix `aws_s3_bucket_public_access_block` 501. fakeaws already implements the four-flag handlers (handlers/s3.go since S43-T8), but they're dead code because S3 traffic routes to SeaweedFS. Likely fix is an infrafactory-side routing shim that proxies S3 subresources (`?publicAccessBlock`, `?policy`, `?versioning`, etc.) to fakeaws while data-plane stays on SeaweedFS — scope to be confirmed by a one-hour spike. **Spike-then-implement**, not pure-implement.
- **S81**: post-S79+S80 39-scenario sweep + N3 carve-out validation against learned avoid pattern.
- **S82**: N2 pruning audit + ratchet test.
- **S83**: next-most-broken sibling-mock fix (data-driven from S81).

## Slices

| Slice | Title | Effort |
|---|---|---|
| S79 | fakeaws KMS tag persistence (sibling-mock #2) | ~2-3 hr |
| S80 | `aws_s3_bucket_public_access_block` 501 — spike + routing shim (or scope-cap) | ~3-6 hr |
| S81 | Post-fix 39-scenario sweep + N3 carve-out validation | ~1-2 hr |
| S82 | N2 pruning audit of `pitfalls/*.yaml` + ratchet test | ~2 hr |
| S83 | Next sibling-mock fix (data-driven from S81) | ~2-4 hr |

**Total**: ~11–17 focused hours. One autonomous loop session.

## Standing rules

Inherit all rules from `slices-54-62-plan.md` § "Standing rules" and `slices-74-78-plan.md` § "Standing rules". Same authority to `gh pr merge --squash --admin --delete-branch`, same pitfall-pollution discipline (`git checkout pitfalls/` to discard sweep noise — never hand-edit), same per-PR scope, same mock-rebuild discipline (kill + rebuild + restart on any sibling-mock code change).

Specifically for sibling-mock PRs (S79 and likely S83; S80 is an infrafactory-side change, see slice for details):
- Land the sibling-mock change first, on its own PR in `redscaresu/fakeaws` (or matching repo). CI must be green.
- Once merged, `cd ../fakeaws && git pull` and rebuild the local binary so the infrafactory sweep picks up the change.
- Re-run the originally-failing scenario to confirm the gap is closed before crossing the slice off.
- Update `docs/mock-gaps.md` to mark the row resolved (or just trim it — the file is append-only with dedup).

For S81's carve-out validation: after the sweep, grep the resulting `pitfalls/gcp.yaml` diff for `learned_from_diff_avoid` entries naming `google_project_service` / `google_project_iam_member`. If at least one appears AND it didn't appear pre-S78, the carve-out is doing its job. If none appears, document the gap — it means no scenario hit the escape in this run; the carve-out's value is latent.

## S79 — fakeaws KMS tag persistence

### Motivation
S76 + S77 surfaced this gap behind the rotation fix. `aws_kms_key.tags` (and `aws_kms_alias` tagging) currently hard-codes an empty list in `ListResourceTags` and acks `TagResource` / `UntagResource` without persisting. The terraform-provider-aws KMS resource then loops on Update waiting for the tag set to converge to the requested value, times out after a few minutes. Same shape as the S77 `RotationEnabled` fix — add a `Tags map[string]string` to `kmsKey`, persist on Tag/Untag, return on List.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S79-T1 | Add `Tags map[string]string` to `kmsKey` in `../fakeaws/handlers/kms.go`. Initialize on `CreateKey` from the request's `Tags` array. | P0 | — |
| S79-T2 | Replace the hard-coded empty `ListResourceTags` handler with one that reads from the store. Replace the no-op `TagResource` / `UntagResource` handlers with ones that mutate the store. | P0 | S79-T1 |
| S79-T3 | Tests in `../fakeaws/handlers/kms_test.go` pinning the tag lifecycle (Create-with-tags → List → Tag → List → Untag → List) and 404 on unknown key. | P0 | S79-T2 |
| S79-T4 | Open PR in fakeaws. Once merged, rebuild + re-run a tagged scenario locally to confirm the gap is closed. Trim or mark the corresponding row in `docs/mock-gaps.md`. | P0 | S79-T3 |

### Exit criteria
- fakeaws PR merged with green CI.
- A scenario with non-empty `aws_kms_key.tags` converges in fewer iterations than before (or at all, if the rotation+tag combo was the blocker).
- `docs/mock-gaps.md` row marked resolved.

## S80 — `aws_s3_bucket_public_access_block` 501 (spike + fix)

### Motivation
The terraform-provider-aws `aws_s3_bucket_public_access_block` resource calls `PutPublicAccessBlock`, `GetPublicAccessBlock`, and `DeletePublicAccessBlock` against the bucket subresource path. The S77 close-out noted a 501 on this surface in aws-full-stack.

**Architectural correction (caught pre-execution, 2026-06-02):** S3 data-plane traffic in this project routes to **SeaweedFS** (port 9090, per `s3:` block in `infrafactory.yaml`), NOT fakeaws (port 8082). SeaweedFS doesn't model the `?publicAccessBlock` subresource and returns 501. fakeaws DOES implement the four-flag PutPublicAccessBlock / GetPublicAccessBlock / DeletePublicAccessBlock handlers (handlers/s3.go:100 onward, since S43-T8) — but those handlers are dead code in the default config because S3 never reaches fakeaws.

So the "S80 = add fakeaws handler" framing is wrong. The real options:

1. **Routing shim (likely best)**: small infrafactory-side reverse proxy at the s3 endpoint that intercepts `?publicAccessBlock` (and likely other subresources SeaweedFS doesn't handle — `?policy`, `?versioning`, `?lifecycle`, `?cors`) and forwards them to fakeaws. Bucket / object data-plane passes through to SeaweedFS unchanged. fakeaws's existing handlers become live.
2. **Upstream SeaweedFS PR**: implement the subresource there. Slower, depends on review cadence.
3. **Cap scope**: declare it a known gap and avoid `aws_s3_bucket_public_access_block` in training scenarios. Cheapest but loses coverage.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S80-T1 | **Spike (~1 hr)**: confirm the 501 source (curl `?publicAccessBlock` against both backends; check SeaweedFS docs for any flag that would enable it). Decide option 1 / 2 / 3 and document the rationale in the PR description. | P0 | — |
| S80-T2 | If option 1: implement the routing shim. Likely a small `cmd/s3-router/` binary, or extend an existing proxy. Forward subresource paths to `${fakeaws.url}/s3/...`, bucket/object paths to `${seaweedfs.url}/...`. Persist nothing — both backends already do. | P0 | S80-T1 |
| S80-T3 | If option 1: wire into `make mocks-up` so the shim launches between `infrafactory mock reset` and scenario runs. Update `infrafactory.yaml` `s3.url` to point at the shim. | P0 | S80-T2 |
| S80-T4 | Tests: a small Go test that drives `?publicAccessBlock` through the shim and confirms fakeaws's handler responds. | P0 | S80-T2 |
| S80-T5 | Re-run aws-full-stack. Confirm no 501. Update `docs/mock-gaps.md` (note: the `?publicAccessBlock` row isn't currently in the file — the S77 close-out cited it but it never made it to the canonical sink; S80's close-out should add a resolved entry retroactively for traceability). | P0 | S80-T4 |

### Exit criteria
- Either: routing shim landed and `aws_s3_bucket_public_access_block` reaches plan/apply through SeaweedFS + fakeaws without 501.
- Or: scope-capped to option 3 with rationale + a follow-up ticket filed.

## S81 — Post-fix 39-scenario sweep + N3 carve-out validation

### Motivation
Two combined goals:
1. **Sustain ratchet**: 37/39 → expected 38–39/39 once S79 (KMS tags) + S80 (S3 PAB) land. `aws-full-stack` should converge faster (the rotation + tag + PAB chain was the bottleneck).
2. **Carve-out validation**: S78 added the N3 GCP-escape carve-out (when `access_token_type_unsupported` × escape resource → route to N13 instead of mock-gaps). The carve-out's value is *latent* unless a scenario actually hits the escape. This sweep gives N13 a chance to fire, and we audit the resulting `learned_from_diff_avoid` entries.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S81-T1 | `make sweep-39`. Capture `/tmp/sweep-39/summary.tsv`. | P0 | S79, S80 |
| S81-T2 | Diff `pitfalls/gcp.yaml` against pre-sweep. Look for `learned_from_diff_avoid` entries naming `google_project_service` / `google_project_iam_member`. If present, the carve-out worked — capture the rule shape for an ADR amendment. If absent, document that no scenario hit the escape this sweep. | P0 | S81-T1 |
| S81-T3 | Triage any regressions from the 37/39 baseline. If aws-full-stack converged: ratchet expected pass count up to whatever the sweep showed. If it didn't: document the still-missing gap. | P0 | S81-T1 |
| S81-T4 | STATUS + NEXT_SESSION update. Discard sweep pollution per protocol. | P1 | S81-T3 |

### Exit criteria
- Single uninterrupted 39-scenario sweep run.
- Carve-out behavior documented (fired or didn't, with evidence).
- New baseline pass count recorded.

## S82 — N2 pruning audit + ratchet test

### Motivation
N2 (the `pitfalls/*.yaml` pruner) hasn't run as a deliberate audit since the prompt-collapse landed. Several `learned_from_diff` entries may now be **redundant** because:
- The matching prompt rule was retired between S54 and S78 (so the pitfall is the only carrier — keep it).
- The matching prompt rule wasn't retired and is still load-bearing (so the pitfall is duplicative — remove it).
- An OPA policy now enforces the same shape (so the pitfall is duplicative — remove it).

This is the inverse of the S55 ratchet: instead of preventing mock-actionable seeds from entering, we look for **functionally-superseded** entries and remove them.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S82-T1 | Read `pitfalls/aws.yaml`, `pitfalls/gcp.yaml`, `pitfalls/scaleway.yaml`. For each entry: identify the matching prompt rule (if any) and matching OPA policy (if any). Classify as: load-bearing (keep), duplicative-with-prompt (remove), duplicative-with-policy (remove), unique (keep). | P0 | — |
| S82-T2 | Remove duplicative entries. Re-run the impacted scenarios to confirm no regression. | P0 | S82-T1 |
| S82-T3 | Add a ratchet test (`internal/generator/pitfalls_no_duplication_test.go`) that fails if the pitfall body verbatim-matches a prompt-rule body OR an OPA policy `msg`. | P0 | S82-T2 |
| S82-T4 | PR. STATUS + NEXT_SESSION + ADR-0012 amendment if the dedup heuristic surfaces a new pattern. | P0 | S82-T3 |

### Exit criteria
- Audit table documented per cloud.
- 0–10 duplicative entries removed (could be zero — that's a clean result, ship the ratchet anyway).
- Ratchet test pins the dedup invariant.

## S83 — Next sibling-mock fix (data-driven from S81)

### Motivation
S81 will surface either (a) a new gap behind the S79+S80 fixes (drain it), (b) a flake worth investigating (pin or fix), or (c) nothing — in which case pick the next-highest-impact entry from `docs/mock-gaps.md` and drain that. Keeps the mock-gap queue moving.

Plausible candidates if (c):
- The five GCP `plugin did not respond` entries — bigger investigation (likely fakegcp panic on an unknown shape).
- `google_compute_instance` panic in gcp-full-stack.
- Any new mock-gap row from S81.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S83-T1 | Pick the target from S81's surfaced state. Document the rationale in the PR description. | P1 | S81 |
| S83-T2 | Implement + test in the sibling repo. Same shape as S79/S80/S77. | P0 | S83-T1 |
| S83-T3 | PR in the sibling. Rebuild + re-run the originally-failing scenario. Trim `docs/mock-gaps.md` row. | P0 | S83-T2 |
| S83-T4 | STATUS + NEXT_SESSION close-out for the S79–S83 arc. | P0 | S83-T3 |

### Exit criteria
- One more sibling-mock PR merged.
- Originally-failing scenario passes.
- Arc-close STATUS + NEXT_SESSION update documenting the arc's net deltas.

## Why this order, in one paragraph

S79 first because it's the smallest fix and proves the S77 pipeline still works after a session boundary. S80 second because it's the bigger of the two known gaps and benefits from a fresh build cycle after S79 lands. S81 next because it's the validation gate — until the sweep runs, we don't know whether S79+S80 actually moved the needle, and we don't know whether the S78 carve-out has fired yet. S82 mid-arc because the pruning audit is independent of S79+S80+S81 and benefits from a clean post-sweep state. S83 last because its content is data-driven from S81 — running it earlier would be guessing.

## Autonomous-execution loop prompt

```
/loop until all 5 slices (S79-S83) in docs/plans/slices-79-83-plan.md are complete: every slice's exit criteria met, every PR merged with green CI, STATUS + NEXT_SESSION updated.

Read docs/NEXT_SESSION.md first for the prior arc's handoff, then docs/plans/slices-79-83-plan.md for the slice definitions. The standing rules from slices-54-62-plan.md AND slices-74-78-plan.md apply (authority to merge, pitfall pollution discipline, mock rebuild discipline, STATUS+ADR+NEXT_SESSION updates, scope per PR).

Work slices in order S79 → S80 → S81 → S82 → S83. Each slice is one PR. S79 + S83 are likely sibling-mock PRs in fakeaws (land in fakeaws first, then merge from main and update docs/mock-gaps.md). S80 is an infrafactory-side routing shim, not a fakeaws change — see the plan for the architectural correction.

Authority: open + merge PRs with `gh pr merge <N> --squash --admin --delete-branch` once CI is green, in all four repos. Discard auto-learning sweep pollution in pitfalls/*.yaml with `git checkout pitfalls/` — never hand-edit.

Stop only when: (a) all 5 slices complete OR (b) you genuinely cannot proceed (regression in main blocks every scenario AND fix-forward isn't obvious — document the blocker in NEXT_SESSION + stop).

Before stopping for any reason, update docs/NEXT_SESSION.md with: which slices closed, which are blocked, what next session should do FIRST.
```

## Fresh-context checklist

The autonomous prompt assumes the agent reads, in order:
1. `AGENTS.md`
2. `docs/NEXT_SESSION.md` (READ FIRST section)
3. This file (`docs/plans/slices-79-83-plan.md`)
4. `STATUS.md`
5. `BACKLOG.md`
6. `docs/decisions/0012-dynamic-pitfalls.md` + `0015-classifier-routing.md` + `0018-n11-retirement-criteria.md`
7. `docs/plans/slices-54-62-plan.md` § "Standing rules" + the N11 7-step protocol
8. `docs/plans/slices-74-78-plan.md` § "Standing rules" + the S77 sibling-mock pattern
