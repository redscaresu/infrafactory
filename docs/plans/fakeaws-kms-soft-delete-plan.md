# Arc: fakeaws KMS soft-delete

Status: planned (2026-06-05)
Owner: next-session claude (designed for autonomous execution)
Follows: `sustain-under-renamed-vocab-plan.md` (closed 2026-06-05 with S105)
Shape: goal-named variable-length arc per AGENTS.md (1 slice, ~30-60 min)

## Big picture

S105 sweep 3 surfaced an organic mock-gap: `aws-secrets-manager` scenario fails during destroy because `terraform-provider-aws` polls `kms.DescribeKey` expecting `KeyState = "PendingDeletion"`, but fakeaws hard-deletes the key on `ScheduleKeyDeletion` and returns 404 NotFoundException on the subsequent DescribeKey.

The comment at `../fakeaws/handlers/kms.go:144` claims the provider treats 404 as "deletion complete" — the live failure proves that's wrong. Structurally identical to **S89 (Secrets Manager soft-delete)**: same fix pattern.

## Standing rules

Inherit from prior arcs. Specifically:

- **Fix at source**: this IS a mock-side fix; the entry in `docs/mock-gaps.md` will clear once the fix lands and the scenario re-runs.
- **Mandatory close-out per Option C**: folded into the single slice.

## Slice

| Slice | Title | Effort |
|---|---|---|
| S106 | fakeaws KMS soft-delete + orphan-check filter + close-out | ~30-60 min |

## S106 — fakeaws KMS soft-delete

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S106-T1 | Modify `handlers/kms.go::kmsScheduleKeyDeletion`: don't `delete(kmsStore.keys, in.KeyId)`. Instead set `k.KeyState = "PendingDeletion"` + `k.DeletionDate = computed` and persist. | P0 | — |
| S106-T2 | Modify `kmsKeyMetadata` (or wherever KeyState is serialized) to emit the persisted KeyState rather than a hardcoded "Enabled". | P0 | T1 |
| S106-T3 | Modify `gatherKMSStateReal` (or equivalent orphan-check state-export path) to filter out keys with `KeyState = "PendingDeletion"` so destroy completes cleanly under orphan_check. Mirror the S89 fix for Secrets Manager. | P0 | T1 |
| S106-T4 | Add `/mock/reset` purge for PendingDeletion keys so re-runs start clean. | P1 | T1 |
| S106-T5 | Add failing handler test for the destroy → DescribeKey flow: ScheduleKeyDeletion succeeds → DescribeKey returns 200 with KeyState=PendingDeletion (not 404). | P0 | T1, T2 |
| S106-T6 | Add orphan_check test: after ScheduleKeyDeletion, `/mock/state` for kms.keys doesn't list the PendingDeletion key. | P0 | T3 |
| S106-T7 | Update comment at `kms.go:144` — the old comment was wrong; new comment explains why we keep the key in PendingDeletion. | P0 | T1 |
| S106-T8 | Replay `aws-secrets-manager` scenario locally to verify destroy completes cleanly. | P0 | T1-T6 |
| S106-T9 | One PR against fakeaws. **Arc close-out folded in** (STATUS + NEXT_SESSION + ARCHIVE per Option C; STATUS + NEXT_SESSION + ARCHIVE go in infrafactory follow-up if needed, otherwise the fakeaws PR is the whole arc). | P0 | T1-T8 |

### Exit criteria

- fakeaws `kms.DescribeKey` returns 200 with `KeyState = "PendingDeletion"` after `ScheduleKeyDeletion` (test pins this).
- orphan_check state export filters PendingDeletion keys (test pins this).
- `aws-secrets-manager` scenario converges through destroy without the wait-loop error.
- ARCHIVE close-out entry lands in infrafactory.

## Autonomous-execution loop prompt

```
/loop until S106 in docs/plans/fakeaws-kms-soft-delete-plan.md is complete: exit criteria met, fakeaws PR merged with green CI, STATUS + NEXT_SESSION + ARCHIVE updated in infrafactory.

Read docs/NEXT_SESSION.md first for the prior arc's handoff, then docs/plans/fakeaws-kms-soft-delete-plan.md for the slice definition. All prior standing rules apply.

S106 folds the mandatory ARCHIVE + NEXT_SESSION close-out per the Option C arc shape — no separate close-out slice.

Authority: open + merge PRs with `gh pr merge <N> --squash --admin --delete-branch` once CI is green, in all four repos.

Reference fix pattern: S89 (Secrets Manager soft-delete in fakeaws@348322d). Same shape — don't hard-delete on schedule, set PendingDeletion state, filter PendingDeletion from orphan_check state export.

Stop only when: (a) S106 complete OR (b) you genuinely cannot proceed (document the blocker in NEXT_SESSION + stop).
```

## Fresh-context checklist

1. `AGENTS.md`
2. `docs/NEXT_SESSION.md`
3. This file
4. `docs/status/ARCHIVE.md` § "2026-06-05 sustain under renamed vocabulary" (S105's mock-gap discovery)
5. `../fakeaws/handlers/kms.go` (the file S106 modifies)
6. `../fakeaws/handlers/secretsmanager.go` (S89 reference fix for the same pattern)
