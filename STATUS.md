# STATUS

Last updated: 2026-06-02

## Current phase

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
