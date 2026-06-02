# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## S74–S78 arc complete (2026-06-02)

All five slices landed:
- ✅ **S74**: AWS phase3 rule 3 sub-bullets on DB subnet group ordering + SG cycle avoidance retired (Category A).
- ✅ **S75**: Scaleway phase3 rule 6.b (private NIC requirement) retired (Category B).
- ✅ **S76**: 39-scenario sweep — 37/39 (historical baseline). N3 routed both failures cleanly to mock-gaps.md.
- ✅ **S77**: First sibling-mock fix — fakeaws PR #4 adds KMS rotation persistence. aws-full-stack converges in 3 iters.
- ✅ **S78**: `make sweep-39` Makefile target + N3 GCP-escape carve-out (`access_token_type_unsupported` × five GCP resource types → routes to N13 instead of mock-gaps).

## READ FIRST (next session)

**Both major prompt collapses are done.** GCP phase2 (S54–S73) + AWS/Scaleway phase3 (S74–S75). All four arcs (S54–S62, S63–S67, S68–S72, S74–S78) converged the auto-derivation loop to a steady 37/39 baseline with conservative classifier routing.

**Next arc planned**: `docs/plans/slices-79-83-plan.md` — sibling-mock drainage + carve-out validation + N2 pruning. Five slices, ~11-17 focused hours:
- **S79**: fakeaws KMS tag persistence (same shape as S77 rotation fix; ~half-day).
- **S80**: fakeaws `aws_s3_bucket_public_access_block` (bigger feature; ~1 day).
- **S81**: post-fix 39-scenario sweep + N3 GCP-escape carve-out validation (does N13 actually learn?).
- **S82**: N2 pruning audit of `pitfalls/*.yaml` (drift sweep + ratchet test).
- **S83**: next sibling-mock fix (data-driven from S81).

Autonomous-execution loop prompt at the bottom of the plan file.

**Sweep entry point**: `make sweep-39` (or `bash scripts/sweep_39.sh` directly). Output lands in `/tmp/sweep-39/`.

## Recent arcs (full close-outs in `docs/status/ARCHIVE.md`)

- **S73** (2026-06-02): retired GCP phase2 rules 9 + 12. GCP phase2 collapsed 11 → 9 rules.
- **S68–S72 arc** (2026-06-02): N3 coverage + M96/M98 close-outs + `cmd/n10extract` CLI + 2 retirements + regression ratchets. 5 PRs (#43–#47).
- **S63–S67 arc** (2026-06-02): 39/39 deterministic sweep, N13 case-insensitive attribution, two flakes verified non-reproducible, `infrafactory mock reset` CLI. 5 PRs (#37–#41).
- **S54–S62 arc** (2026-06-02): 9 GCP retirements + ADR-0018 retirement framework. 9 PRs (#26–#34).
