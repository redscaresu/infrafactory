# STATUS

Last updated: 2026-06-03

## Current phase

- 🎯 **Baseline: 39/39 deterministic, 0 panics** (achieved S90 in the S89–S93 arc). Full per-arc narratives in `docs/status/ARCHIVE.md`.
- **Active arc**: `docs/plans/sustain-and-n13-durability-plan.md` (first goal-named arc under Option C).
  - **S94 (this PR)**: N13 durability. `cmd/pitfall-merge/` selectively preserves `learned_from_diff_avoid` entries through sweep teardown; `learned` + `learned_from_diff` discarded as before. Schema-enum ratchet + sweep-side `N13_EMISSIONS=N` counter (soft warn on zero).
  - **S95 next**: three consecutive `make sweep-39` runs to validate the 39/39 baseline is stable + observe N13 emergence under the new protocol.

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

## Update policy

- Trim this file every arc close-out — historical detail belongs in `docs/status/ARCHIVE.md`.
- Goal: ≤ 30 lines. If it grows past 50, time to trim again.
- ADRs and `CONCEPT.md` carry durable architecture decisions; STATUS.md is just the current-shape pointer.
