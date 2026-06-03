# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## S84–S88 arc complete (2026-06-03)

All five slices landed. 38/39 deterministic sweep maintained; persistent failure shape shifted from LLM-side (S81: gcp-full-stack SNC escape) to mock-side (S88: aws-full-stack orphan_check on Secrets Manager soft-delete). Full close-out: `docs/status/ARCHIVE.md` § "2026-06-03 S84–S88".

- ✅ **S84** — timeboxed investigation; root cause identified (`docs/investigations/gcp-full-stack-2026-06-03.md`).
- ✅ **S85** — `learned` pitfall against `google_service_networking_connection`. End-to-end validated.
- ✅ **S86** — fakegcp panic triage; all 5 historical mock-gap entries non-reproducible. Findings in `docs/investigations/fakegcp-panics-2026-06-03.md`.
- ✅ **S87** — `scripts/sweep_39.sh` panic-detection gate.
- ✅ **S88** — post-arc 39-scenario sweep (38/39, zero panics).

## READ FIRST (next session)

**Persistent aws-full-stack failure** — different shape from S81's. Now `repair_budget_exhausted` via `stuck` on `orphan_check`: `aws_secretsmanager_secret` is not fully destroyed by the mock when the LLM removes it; the orphan_check sees the entry in `/mock/state` after destroy. Classifier already labels it `LLMSoftDelete` subshape.

This is the same pattern as the historical `aws_iam_policy auto-seeded ARN` and `aws_subnet MapPublicIpOnLaunch` orphans (both now resolved). Fix shape: fakeaws Secrets Manager handler needs to honor the soft-delete window (real AWS sets the secret to PendingDeletion with a window, then garbage-collects). For test purposes, immediate hard-delete on `DeleteSecret` is fine — same approach S77 took for KMS.

**Suggested S89-T1**: investigate fakeaws Secrets Manager `DeleteSecret` handler; either drop the secret immediately on delete (mirrors S77 KMS rotation pattern) or expose a `force_delete_without_recovery` short-circuit. Then re-run aws-full-stack.

## Suggested next arc

**Planned**: `docs/plans/slices-89-93-plan.md` — five slices, ~5–7 focused hours (smallest arc since S54):

- **S89**: fakeaws Secrets Manager `DeleteSecret` immediate hard-delete (sibling-mock PR; same shape as S77 KMS rotation fix). Unblocks aws-full-stack.
- **S90**: post-S89 39-scenario sweep + 39/39 confirmation.
- **S91**: AWS phase2 audit per ADR-0018 (classify every rule Cat A/B/C). 10 rules; ~1 hr.
- **S92**: Retire AWS phase2 Category-A candidates (0-2 expected; "no candidates" is a valid outcome).
- **S93**: Post-retirement sweep + arc close-out + **scaffold-question writeup** (agent writes the analysis + 2-3 alternative shapes + recommendation; **user picks the shape**, agent does NOT commit to one).

Autonomous-execution loop prompt at the bottom of the plan file.

**Sweep entry point**: `make sweep-39`. Output lands in `/tmp/sweep-39/summary.tsv` + `panics.log` + per-scenario logs.

## Recent arcs (full close-outs in `docs/status/ARCHIVE.md`)

- **S84–S88** (2026-06-03): gcp-full-stack convergence + panic gate. 3 PRs.
- **S79–S83** (2026-06-02): sibling-mock drainage + carve-out validation. 4 PRs.
- **S74–S78** (2026-06-02): AWS/Scaleway phase3 collapse + `make sweep-39` + N3 carve-out. 5 PRs.
- **S68–S72** (2026-06-02): N3 coverage + M96/M98 close-outs + `cmd/n10extract` CLI. 5 PRs.
- **S63–S67** (2026-06-02): 39/39 deterministic sweep, `infrafactory mock reset` CLI. 5 PRs.
- **S54–S62** (2026-06-02): 9 GCP retirements + ADR-0018 retirement framework. 9 PRs.
