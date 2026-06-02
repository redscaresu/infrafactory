# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## READ FIRST — 2026-06-02

S63 (post-collapse 39-scenario sweep) closed: **39/39 deterministic**, no regression from the S56–S60 prompt retirements. Four S63–S67 slices remain:
- **S64**: N13 first-production audit. **N13 didn't fire organically this sweep**, so the PR is small — capture that outcome + audit the three S63 audit findings below.
- **S65**: gcp-cloud-run `deletion_policy` hallucination triage. **The flake didn't recur in S63** — re-run gcp-cloud-run 5× to confirm consistency. If stable, the slice may close as "no longer reproducible."
- **S66**: gcp-full-stack `google_apikeys_key` mock gap. Also didn't recur in S63 (3 iters → target_reached). Same "verify reproducibility" pattern.
- **S67**: Sweep harness sustain ratchet (`infrafactory mock reset` CLI command).

**S63 audit findings carried into S64**:
1. `aws_subnet` `learned_from_diff` is a false positive — the LLM's iter-pair diff captured ADDED attrs (`cidr_block`, `availability_zone`) but the actual fix was REMOVAL of `map_public_ip_on_launch`. N13 should have caught it but the failure detail uses camelCase (`MapPublicIpOnLaunch`) while the HCL attribute is snake_case (`map_public_ip_on_launch`) — N13's `strings.Contains(failureDetail, attr)` attribution failed. **Fix in S64**: case-insensitive (or snake↔camel) attribute matching.
2. Two mock-actionable failures (`aws_kms_key` rotation timeout, `aws_route53_record` empty-result) bypassed the N3 classifier and landed as `learned` pitfalls. The N3 `IsMockActionable` predicate needs to recognize "rotation update timeout" + "empty result" as signals. **Investigate in S64-T2 territory; may spawn a small N3-classifier patch slice.**
3. No `learned_from_diff_avoid` entries — N13 in production confirms the heuristic doesn't false-positive (good) but also confirms the gcp-cloud-run flake from S59 wasn't deterministic.

Plan file with full slice definitions: `docs/plans/slices-63-67-plan.md`.

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
