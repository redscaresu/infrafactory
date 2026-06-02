# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## S74–S78 progress

- ✅ **S74**: AWS phase3 rule 3 sub-bullets on DB subnet group ordering + SG cycle avoidance retired (Category A).
- ✅ **S75**: Scaleway phase3 rule 6.b (private NIC requirement) retired (Category B — covered by existing `scaleway_instance_server` pitfall).
- ✅ **S76**: 39-scenario sweep — 37/39 (historical baseline). Two AWS failures, both correctly classified into `docs/mock-gaps.md`: `aws_kms_key` rotation timeout (mock-side fakeaws state-divergence), `aws-vpc-network` empty `main.tf` (LLM transport flake).
- ⬜ S77, S78: remaining.

## READ FIRST

**GCP phase2 prompt-collapse complete.** The 9-retirement target arc described in `docs/plans/slices-54-62-plan.md` § "Big picture" is done — GCP phase2 is now system-contract + scenario-intent only (rules 1–8 + 16 + 17). All 9 originally-prescriptive rules retired between S56 (firewall) and S73 (project_service + project_iam_member). 39/39 deterministic sweep confirmed at S63 and S72 baselines.

**Start here:** `docs/plans/slices-74-78-plan.md` — five slices, ~8–12 focused hours:
- **S74**: AWS phase2/3 audit + Category-A retirements (mirror of the GCP collapse).
- **S75**: Scaleway phase2/3 audit + Category-A retirements.
- **S76**: Post-collapse 39-scenario deterministic sweep.
- **S77**: `docs/mock-gaps.md` triage — 2-3 sibling-mock PRs to drain the queue.
- **S78**: `make sweep-39` Makefile target + N3 classifier escape carve-out.

Autonomous-execution loop prompt at the bottom of the plan file.

## Recent arcs (full close-outs in `docs/status/ARCHIVE.md`)

- **S73** (2026-06-02): retired GCP phase2 rules 9 + 12. GCP phase2 collapsed 11 → 9 rules.
- **S68–S72 arc** (2026-06-02): N3 coverage + M96/M98 close-outs + `cmd/n10extract` CLI + 2 retirements + regression ratchets. 5 PRs (#43–#47).
- **S63–S67 arc** (2026-06-02): 39/39 deterministic sweep, N13 case-insensitive attribution, two flakes verified non-reproducible, `infrafactory mock reset` CLI. 5 PRs (#37–#41).
- **S54–S62 arc** (2026-06-02): 9 GCP retirements + ADR-0018 retirement framework. 9 PRs (#26–#34).
