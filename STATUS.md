# STATUS

Last updated: 2026-06-02

## Current phase

- **S79 + S80 in flight (2026-06-02).** S79: fakeaws KMS tag persistence — fakeaws PR #5 merged, end-to-end validation running. S80: `cmd/s3router/` shim landed locally — splits S3 traffic between SeaweedFS (data plane) and fakeaws (`?publicAccessBlock` subresource). Twelve regression tests cover the routing + fan-out logic. `make mocks-up` now starts the shim on :9091; `infrafactory.yaml` `s3.url` points there. ADR-0015 amended with the architectural rationale.
- **S78 — `make sweep-39` Makefile target + N3 classifier GCP-escape carve-out landed.** Two-part close of the S74–S78 arc: (1) `scripts/sweep_39.sh` + `make sweep-39` replaces every prior arc's `/tmp/sweep-*.sh` reinvention — uses `./bin/infrafactory mock reset` between scenarios (the S67 CLI that cascades to SeaweedFS; bare `curl` resets don't), discards pitfall pollution per protocol, emits `summary.tsv` + per-scenario logs. (2) `IsMockActionable` carve-out: when the matched signal is `access_token_type_unsupported` AND the failure detail references `google_project_service` / `google_service_networking_connection` / `google_project_iam_{member,binding,policy}`, return false so N13 can learn the avoid pattern via `ExtractLearnedPitfall` (mock-side fix is structurally impossible for these resources; LLM-side drop IS attainable). Without the carve-out, S73's retirement of GCP phase2 rules 9 + 12 leaves no auto-learning path for the rule. 7 regression tests pin both shapes. ADR-0015 amended with carve-out rationale + Makefile target. **S74–S78 arc complete.**
- **S77 — first sibling-mock fix landed: fakeaws KMS rotation persistence.** Sibling-mock PR ([fakeaws#4](https://github.com/redscaresu/fakeaws/pull/4)) added \`RotationEnabled\` to \`kmsKey\` and routed Enable/Disable/GetKeyRotationStatus through new state-aware handlers. Closes the S76 \`aws_kms_key rotation update: timeout while waiting for state to become 'TRUE'\` mock-gap entry. Validated end-to-end: aws-full-stack post-fix → target_reached iter 3 (1260s), **zero rotation_timeout mentions** in the log (previously this was the gate that exhausted the budget). Slower convergence than ideal because two NEW mock gaps surfaced behind the rotation gap (KMS `tag update` persistence — same shape, ~10 min fix; \`aws_s3_bucket_public_access_block\` 501 — bigger fakeaws feature). Both captured for next tranche.
- **S76 — post-AWS+Scaleway-collapse 39-scenario sweep: 37/39.** Same baseline as S63 / S72. Two failures both correctly classified to `docs/mock-gaps.md` (no pitfall pollution — the N3 classifier from S68 routed cleanly): `aws-full-stack` on `aws_kms_key` rotation timeout (fakeaws mock-side state-divergence), `aws-vpc-network` on empty `main.tf` from the LLM (transport flake, pre-existing). Both are S77 candidates. The 4 new `learned_from_diff` entries that emerged this sweep (2 GCP + 2 Scaleway) were discarded per protocol — they'll re-emerge on future sweeps when the same iter-pair patterns recur.
- **S75 — Scaleway phase3 rule 6.b retired (Category B).** Audit: Scaleway phase2 (10 rules) all Category C; phase3 rule 6 sub-bullets 6.b (private NIC for servers) + 6.c (no public DB) are Category B with existing pitfalls + OPA policies as replacement. Retired 6.b. Validation across 3 scaleway-instance-server scenarios: `compute-lb-multi-paris` → target_reached iter 3 (9 NICs); `private-lb-db-paris` → target_reached iter 2 (4 NICs); `web-app-paris` → repair_budget_exhausted iter 5 on a pre-existing `scaleway_lb_backend.server_ips` empty-list issue UNRELATED to the retirement (2 NICs were correctly generated). The auto-learned `scaleway_instance_server` VPC pitfall carries rule 6.b. 15th N11 retirement.
- **S74 — AWS phase3 retirements landed.** Two Category-A retirements: rule 3 sub-bullets on **DB subnet group ordering** (Category A, `tofu plan` error if reference missing) and **SG cycle avoidance** (Category A, `tofu plan` cycle error from inline ingress/egress blocks self-referencing). Re-validated against aws-rds (target_reached iter 1, 3 `aws_db_subnet_group` resources correctly produced) + aws-eks (target_reached iter 1, 0 inline SG blocks) + aws-full-stack (target_reached iter 4, the slow convergence was on an unrelated `aws_kms_alias` naming-policy quirk that the LLM self-resolved). AWS phase3 rule 3 down to 2 remaining sub-bullets (VPC ordering — Category B, IAM profile chain — Category C). Per ADR-0018.
- **GCP phase2 prompt-collapse complete.** Nine retirements landed between S56 (firewall) and S73 (project_service + project_iam_member). Phase2 prompt now contains only rules 1–8 (system contract) + 16 (region) + 17 (naming) — the destination described in `slices-54-62-plan.md` § "Big picture". The N10→N11→N13 auto-derivation pipeline is end-to-end and battle-tested across two sustain-ratchet sweeps (S63, ~implicit via S72/S73 runs).
- **Next arc planned**: `docs/plans/slices-79-83-plan.md` — sibling-mock drainage (fakeaws KMS tags + `aws_s3_bucket_public_access_block`) + post-fix 39-scenario sweep + N3 carve-out validation + N2 pruning audit. ~11–17 hr.
- Older arc close-outs (S54–S73) and milestones (S1–S53) live in `docs/status/ARCHIVE.md`.

## In progress

- No active implementation tickets. S63–S67 arc planned but not started — see `docs/plans/slices-63-67-plan.md`.

## OSS-readiness (2026-05-23)

All four repos (`infrafactory`, `mockway`, `fakegcp`, `fakeaws`) ship: Apache-2.0 LICENSE, SECURITY.md, CODE_OF_CONDUCT.md, CONTRIBUTING.md, CHANGELOG.md, .editorconfig, .github/ISSUE_TEMPLATE/ + pull_request_template.md, release workflow. Pre-commit hook (`gitleaks` + `go test`) installable via `make install-hooks` on every repo. Full-history `gitleaks detect` returned zero leaks.

**Click-ops still pending (user-only):**
- `gh repo edit --visibility public` on each repo when ready.
- Branch protection rules on `main` (free-tier blocks on private; works post-flip).

## Known blockers

- None.

## Next actions

1. Run the S63–S67 arc per `docs/plans/slices-63-67-plan.md`.
2. Keep `go test -tags noui ./...` + `bash scripts/check_all.sh` green.

## Update policy

- Update at end of each meaningful coding session.
- Keep entries concise.
- Move old detail to `docs/status/ARCHIVE.md`.
- Put durable architecture decisions in ADRs and `CONCEPT.md`.
- Keep startup/read-order in `SESSION_START.md` to avoid duplication.

## Recent updates

See `docs/status/ARCHIVE.md` § "2026-06-02 S54–S62 close-out" for the per-slice narrative. Older recent-updates content (S33–S53 close-outs, the auto-learning loop design notes, the 2026-05-30 → 2026-05-31 sweep narratives) is in the same file.
