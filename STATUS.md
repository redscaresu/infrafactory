# STATUS

Last updated: 2026-06-02

## Current phase

- **S69 — M96 closed as superseded.** Audit found `ExtractLearnedPitfall` is not superseded by N10/N13 — they're layered, not competing. N10/N13 fire only on `target_reached` and produce prescriptive rules from real iter-pair diffs; M97 templates inside `ExtractLearnedPitfall` cover the still-stuck-runs niche where the auto-correction loop hasn't converged yet; the descriptive fallback is the last-resort base case when neither matches. M96's original question (path 1 vs path 2) was answered architecturally by the N10→N11→N13 sequence over Slices 54-67, not by changing `ExtractLearnedPitfall`. No code change; BACKLOG row marked done with the audit rationale, ADR-0012 amended with the four-layer extractor model.
- **S68 — N3 classifier coverage gap closed.** Added two patterns to `IsMockActionable` from the S63 sweep audit: (a) `waiting for state to become '...'` (provider polling on a mock-side field that doesn't persist after Update — AWS KMS rotation, EC2 Subnet MapPublicIpOnLaunch, similar); (b) `empty result` (mock acks Create but Read returns 0 rows — aws_route53_record). The pre-existing `aws_subnet` `MapPublicIpOnLaunch` stale entry that the ratchet caught is removed from `pitfalls/aws.yaml`. Three new positive-case tests in `TestIsMockActionable_FivePositiveSignalClasses`. M91 ratchet was already source-aware and now enforces the new signals at CI time.
- **S67 — `infrafactory mock reset` CLI command landed. S63–S67 arc CLOSED.** New subcommand resets every configured mock backend (mockway + fakegcp + fakeaws + s3 cascade via `cloudMockStateRouter.ResetAll`) in one call. Closes the S54 SeaweedFS state-leak gap: sweep harnesses no longer need a bare-curl carve-out to drop SeaweedFS buckets. Two unit tests pin the fan-out (with + without s3 configured). Smoke-tested against the live stack. With this, the entire S63–S67 arc is closed (5 PRs: #37 S63, #38 S64, #39 S65, #40 S66, this S67 PR).
- **S66 — gcp-full-stack `google_apikeys_key` flake no longer reproducible.** Ran the scenario 5× consecutively (S66-T4): 4× target_reached iter 1 (164-266s), 1× target_reached iter 2 (414s). Zero `google_apikeys_key` mentions across any of the 5 logs — the LLM never reached for the unsupported resource. The S57 mock-gap was non-deterministic LLM behavior. Closing without a code change. Safety net: if the LLM reaches for `google_apikeys_key` in a future run, the apply will fail clearly (the resource isn't implemented by fakegcp); N13 should catch the removal organically once the LLM self-corrects.
- **S65 — gcp-cloud-run `deletion_policy` flake no longer reproducible.** Ran the scenario 5× consecutively (S65-T1): 4× target_reached iter 1 (24-32s), 1× target_reached iter 2 (53s). No `deletion_policy` hallucination in any run. The S59 stuck-pattern was non-deterministic LLM behavior, not a systemic gap. Closing without a code change. Safety net: S64's case-insensitive N13 attribution + the existing `google_cloud_run_v2_service` `deletion_protection` learned pitfall mean any future recurrence would: (a) feed back through the dynamic correction loop, (b) self-learn into pitfalls via N13's `learned_from_diff_avoid` shape if the LLM removes the offending attr to clear it.
- **S64 — N13 case-insensitive attribution CLOSED.** Closes finding (1) from S63's audit. `ExtractPrescriptiveAvoid` now matches removed attribute names against the failure detail in three forms — literal, case-insensitive snake_case, and camelCase (`map_public_ip_on_launch` → `MapPublicIpOnLaunch`). The AWS provider echoes JSON-side field names verbatim in many timeout errors, so the original strict-substring check missed legitimate deletion-as-fix patterns. New regression test pins the aws_subnet `MapPublicIpOnLaunch` shape; existing four N13 tests remain green. ADR-0012 amended. S63 audit findings (2) and (3) carried into S65/S66 (the two flake-triage slices) since both flakes also didn't recur in S63 — those slices become "verify reproducibility + close if stable" rather than fix-with-code.
- **S63 — 39/39 deterministic sweep CLOSED.** Post-collapse re-validation across all 39 training scenarios: every scenario passed (`target_reached`) under the prompt-collapsed state from S54–S62. No regression from the six N11 retirements. Three audit findings carried into S64: (a) `aws_subnet` learned_from_diff false positive — N10 captured added attrs while the actual fix was a REMOVAL of `map_public_ip_on_launch` (N13 case but the failure detail used the camelCase `MapPublicIpOnLaunch` while the HCL attribute is `map_public_ip_on_launch`, so attribution missed); (b) two mock-actionable failures (`aws_kms_key` rotation timeout, `aws_route53_record` empty-result) bypassed the N3 classifier and landed in pitfalls as `learned`; (c) N13 didn't fire organically — the gcp-cloud-run `deletion_policy` flake from S59 didn't recur this sweep. Pitfall pollution discarded per protocol; the legitimate entries will re-emerge.
- **S54–S62 sustain + prompt-collapse arc CLOSED.** Nine PRs merged (#26–#34). GCP phase2 prompt collapsed from 17 → 11 prescriptive rules. ADR-0018 codifies the three-category N11 retirement framework. The N10 → N11 → N13 sequence (addition + removal auto-derivation) is end-to-end across GCP + AWS + Scaleway.
- Older milestones (S1–S53) are in `docs/status/ARCHIVE.md`.

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
