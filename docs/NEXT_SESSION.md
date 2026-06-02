# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## READ FIRST — 2026-06-02

The S54–S62 sustain + prompt-collapse arc closed. The S63–S67 arc is planned but not yet started.

**Start here:** `docs/plans/slices-63-67-plan.md` — five slices, ~6-10 focused hours:
- **S63**: Post-collapse deterministic 39-scenario sweep (re-validate after 6 prompt retirements).
- **S64**: N13 first-production audit (`learned_from_diff_avoid` entries from S63).
- **S65**: gcp-cloud-run `deletion_policy` hallucination triage.
- **S66**: gcp-full-stack `google_apikeys_key` mock gap.
- **S67**: Sweep harness sustain ratchet (`infrafactory mock reset` CLI) + optional permanent `cmd/n10extract`.

The autonomous-execution loop prompt to start the arc is at the bottom of that plan file.

## What's already done (S54–S62 close-out)

| Slice | PR | Outcome |
|---|---|---|
| S54 | #26 | 39/39 deterministic sweep baseline + N10 dedupe fix |
| S55 | #27 | N10 wiring + trim + ratchet fixes |
| S56 | #28 | Retire GCP rule 11 (firewall) — Category A |
| S57 | #29 | Retire GCP rule 13 (GKE) — Category A |
| S58 | #30 | Retire GCP rules 14 + 15 (Cloud SQL + GCS) — Category A |
| S59 | #31 | Retire GCP rule 10 (VPC) — Category B |
| S60 | #32 | Retire AWS RDS + Scaleway RDB/LB — Category A |
| S61 | #33 | N13 deletion-as-fix extractor |
| S62 | #34 | ADR-0018 retirement-criteria framework |

**Architectural milestones**:
- GCP phase2 prompt collapsed from 17 prescriptive rules to 11 (9 if hand-applied retirements count as Category C).
- N10 → N11 → N13 sequence end-to-end: addition-as-fix via `learned_from_diff`, removal-as-fix via `learned_from_diff_avoid`, both fed by real run diffs.
- 7-step retirement protocol generalized across GCP + AWS + Scaleway.

## Open follow-ups carried into the S63–S67 arc

- **gcp-cloud-run `deletion_policy` hallucination** (from S59 step 5) — S65 handles.
- **gcp-full-stack `google_apikeys_key` mock gap** (from S57) — S66 handles.
- **SeaweedFS state-leak in bare-curl sweep harness** (from S54) — S67-T1 handles via `infrafactory mock reset` CLI command.
- **Permanent `cmd/n10extract`** — S67-T3 (optional).

## Important context still relevant

- **ADR-0012** (with two amendments) captures N10 + N13 design.
- **ADR-0018** codifies N11 retirement criteria — read before any further retirement.
- **`feedback_sweep_protocol.md`**: never hand-edit `pitfalls/*.yaml`. Discard sweep pollution with `git checkout pitfalls/`. The N10/N13 extractors are the only legitimate authors.
- **`feedback_mock_design.md`**: mocks optimize for fast feedback, not realism. Mock gaps get fixed at source.
- **Standing rules for autonomous execution** are in `docs/plans/slices-54-62-plan.md` § "Standing rules". S63–S67 inherits them all.

## Older context

- **Per-slice close-out notes for S54–S61** + **prior-session narratives back to 2026-05-30** live in `docs/status/ARCHIVE.md` (S54-S62 section).
- **`STATUS.md`** has the rolling "current phase" log; older entries move to `docs/status/ARCHIVE.md` per the existing update policy.
