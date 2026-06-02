# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## READ FIRST — 2026-06-02

**S63–S67 arc CLOSED.** Five PRs merged (#37–#41). The post-collapse arc validated 39/39 deterministic + tightened N13 attribution + verified two flakes are non-reproducible + added `infrafactory mock reset` CLI for sweep harness ergonomics.

**Closed this session**:
- S63: 39/39 deterministic post-collapse sweep — no regression from the six N11 retirements.
- S64: N13 case-insensitive attribution. `attributeAppearsInDetail` matches removed attribute names in literal + case-insensitive snake_case + camelCase forms. Closes the aws_subnet `MapPublicIpOnLaunch` false-positive finding from S63. ADR-0012 amended.
- S65: gcp-cloud-run `deletion_policy` flake no longer reproducible (5/5 clean runs).
- S66: gcp-full-stack `google_apikeys_key` flake no longer reproducible (5/5 clean runs, zero apikeys mentions).
- S67: `infrafactory mock reset` CLI command added. `cloudMockStateRouter.ResetAll` fans out across mockway + fakegcp + fakeaws + s3 (SeaweedFS) cascade. Closes the S54 SeaweedFS state-leak gap — sweep harnesses no longer need a bare-curl carve-out.

## S68–S72 progress

- ✅ **S68**: N3 classifier coverage gap — `IsMockActionable` now recognizes `waiting for state to become` + `empty result` shapes.
- ✅ **S69**: M96 closed as superseded. ADR-0012 amended with the four-layer extractor model.
- ✅ **S70**: Permanent `cmd/n10extract` CLI — drives N10/N13 against a recorded run dir, emits a candidate pitfall YAML. `--run-dir` shorthand auto-discovers the iter pair. Three unit tests + smoke-tested against gcp-cloud-sql. `make build` includes it.
- ⬜ S71, S72: remaining.

## Next arc: S68–S72

Plan file: `docs/plans/slices-68-72-plan.md`. Five slices, ~6–10 focused hours, designed for one autonomous loop session.

- **S68**: N3 classifier coverage gap — add the two patterns from S63's audit (`aws_kms_key` rotation timeout, `aws_route53_record` empty-result). ~30 min.
- **S69**: Close M96 (descriptive vs prescriptive) as superseded by N10/N13 — audit the legacy extractor's call sites. ~1 hr.
- **S70**: Promote the throwaway `cmd/n10extract` helper to a permanent CLI command. ~1-2 hr.
- **S71**: M98 — fix OPA known-after-apply false-fire on `vpc_required.rego` + `encryption.rego` (false-flagging correct HCL because `planned_values` shows `null` for known-after-apply references). ~half-day.
- **S72**: ADR-0018 sustaining audit — AWS + Scaleway phase3 retirements. Expect 1–2 more Category-A per cloud. ~2-3 hr.

Autonomous-execution loop prompt at the bottom of the plan file.


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
