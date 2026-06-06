# STATUS

Last updated: 2026-06-06

## Current phase

- 🎯 **Baseline: 39/39 deterministic, 0 panics** (pre-S114). Genesys-aware sustain (44/44 = 39 existing + 5 genesys) deferred to S115's actual sweep.
- **In progress arc**: `docs/plans/fakegenesys-arc-plan.md` — 4th cloud (Genesys Cloud CCaaS) build-out. fakegenesys S108-S113 merged + S114 merged + S115 cross-link/close-out merged. **S115 hardening (THIS PR + fakegenesys#10)** caught 3 dispatch bugs from the operator-driven smoke test: `detectAwsProviderWiring` over-matched on `aws_region`, `cloudEnv` missing `GENESYSCLOUD_*` env vars, scenario `acceptance_criteria` shape wrong. All fix-forward at source.
- **Last arc complete**: `docs/plans/fakeaws-kms-soft-delete-plan.md` (sixth Option C arc). Single slice (S106 / fakeaws#9). Closes the loop on the organic mock-gap S105 surfaced — fakeaws KMS now soft-deletes (state=PendingDeletion, DescribeKey returns 200) matching real AWS lifecycle. `aws-secrets-manager` converges target_reached in 1 iteration.
- **Prior arc**: `docs/plans/sustain-under-renamed-vocab-plan.md` (S105). 117/117 deterministic across 3 sweeps; rename durable under live conditions.
- **Last arc complete**: `docs/plans/post-sustain-tightening-plan.md` (second Option C arc). Five PRs landed.
  - **S96** (fakeaws#7): fakeaws Route 53 — sort records lexicographically before `maxitems=1` filter; add `ChangeTagsForResource` POST handler. aws-route53 converges iter 1 end-to-end.
  - **S97** (#78): transport-failure classifier in `sweep_39.sh`. Reclassifies pre-iter-1 Claude CLI failures as `transport_failed` distinct from `repair_budget_exhausted`. Dry-run on sweep-s95-3 data: 5/7 correctly reclassified.
  - **S98** (#79): retired GCP phase3 self-review rule #13 (Category B — verbatim OPA `region_restriction` citation). AWS/Scaleway phase3 audit: zero same-shape candidates. Validated end-to-end via gcp-cloud-run iter 2.
  - **S99 (this PR)**: extended OPA-dup ratchet to `prompts/<cloud>/*.md` via new `TestPromptsNoOPAPolicyCitations`. Verified retroactively against rule #13. Arc close-out folded in.
- **Last arc complete**: `docs/plans/sustain-and-n13-durability-plan.md` — Option C's first goal-named arc. S94 landed `cmd/pitfall-merge/` (selectively preserves N13 entries through sweep teardown). S95 ran 3 sustain sweeps + folded close-out. N13 zero emissions across all 3 sweeps (no organic deletion-as-fix this cycle).

## Recent arcs

| Arc | Outcome | PRs |
|---|---|---|
| S89–S93 (2026-06-03) | 🎯 39/39 first deterministic; fakeaws Secrets Manager soft-delete fix; AWS phase2 audit (10/10 Cat C). Option C scaffold shape adopted. | fakeaws#6, #69, #70 |
| S84–S88 (2026-06-03) | gcp-full-stack convergence (servicenetworking escape pitfall); `scripts/sweep_39.sh` panic gate. | #64, #65, #66 |
| S79–S83 (2026-06-02) | Sibling-mock drainage + N3 carve-out validation; `cmd/s3router/` shim added. | fakeaws#5, #58–#61 |
| S74–S78 (2026-06-02) | AWS + Scaleway phase3 collapse; `make sweep-39`; N3 GCP-escape carve-out. | 5 PRs |
| S54–S73 | GCP phase2 collapse, sustain ratchets, N3/N10/N13 architecture build-out. | ~22 PRs across 4 sub-arcs |

Full per-arc close-outs in `docs/status/ARCHIVE.md`.

## OSS-readiness

All four repos (`infrafactory`, `mockway`, `fakegcp`, `fakeaws`) ship Apache-2.0 + SECURITY.md + CODE_OF_CONDUCT.md + CONTRIBUTING.md + CHANGELOG.md + release workflow. Pre-commit hook (`gitleaks` + `go test`) installable via `make install-hooks`. Full-history `gitleaks detect` zero leaks.

**User-only click-ops pending** (private → public visibility flip + branch protection on each repo).

## Known blockers

None.

## Open tickets

None — `docs/tickets/rename-learning-system.md` closed by S104.

## Update policy

- Trim this file every arc close-out — historical detail belongs in `docs/status/ARCHIVE.md`.
- Goal: ≤ 30 lines. If it grows past 50, time to trim again.
- ADRs and `CONCEPT.md` carry durable architecture decisions; STATUS.md is just the current-shape pointer.
