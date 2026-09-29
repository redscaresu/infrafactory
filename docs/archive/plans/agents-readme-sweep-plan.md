# Arc: AGENTS.md + README cross-repo cleanup

Status: planned (2026-06-10)
Owner: next-session claude (designed for autonomous execution)
Follows: `sibling-critical-sweep-plan.md` (S128–S130 finish the v0.2 contract-audit durability story). Lands AFTER S128–S130 so any cross-references this sweep adds reflect the post-S130 state.
Shape: 5-PR cross-repo sweep (7th instance of `reference_cross_repo_docs_sweep.md`).

## Big picture

User asked 2026-06-05: "update, optimise agents.md, readmes.md across all the repos." Carried directive — postponed through the v0.2 hardening arc, now unblocked.

This is the **fresh-eye consistency pass**. Not a rewrite. Each file gets a read-and-tighten. Specific failure modes to look for:

1. **Stale claims**: test counts, line counts, "current state" sections written months ago.
2. **Drifted duplication**: when the same fact lives in multiple files (e.g. sibling-list, port assignments, OSS-checklist items), check the copies say the same thing.
3. **Broken cross-references**: file paths that moved, slice IDs that retired, deleted features still cited.
4. **Inconsistent terminology**: after v0.2 we added "Contract-coverage convention" — every cross-reference should use that exact wording. After v0.1 + S114 we have "Genesys Cloud CCaaS as 4th cloud peer of scaleway/gcp/aws" — same exact phrasing.
5. **Stale OSS checklist mention**: post-v0.2, the OSS-mature-day-one checklist is 14 items (was 13). Any file that quotes "13" is stale.
6. **Outdated sibling list**: some files may still say "3 siblings" or list only 3 fakes pre-fakegenesys.

## Slices

| Slice | Title | Effort |
|---|---|---|
| S131 | infrafactory: AGENTS.md + README.md | ~30-45 min |
| S132 | mockway: AGENTS.md + README.md | ~30-45 min |
| S133 | fakegcp: AGENTS.md + README.md | ~30-45 min |
| S134 | fakeaws: AGENTS.md + README.md | ~30-45 min |
| S135 | fakegenesys: AGENTS.md + README.md + arc close-out (Option C) | ~30-45 min + close-out |

**Total**: ~2.5–4 hours.

## Per-slice methodology

For each repo:

1. Read AGENTS.md cover-to-cover, then README.md.
2. Make a list of specific issues (cited by line number) in a scratch comment for the PR.
3. Edit ONLY the issues. Don't reflow paragraphs that are fine.
4. Open one PR per repo with a "Changes" section in the description listing what was tightened + a "Left alone" section listing anything considered-and-rejected.

## Standing rules

- **Don't rewrite for the sake of rewriting.** If a section reads well, leave it. The default action on any line is "leave it." Justify edits by citing the specific failure mode (1-6 above).
- **No content additions** beyond fixing stale/wrong content. New explanatory sections belong in a separate arc.
- **Cross-reference consistency check**: after editing repo N, run a `grep -r "<terminology>"` across all 5 repos to make sure your edit didn't introduce a new inconsistency.
- Codex anti-nitpick: if pass 1 finds nothing substantive, single-pass is fine. The doc cleanup itself doesn't warrant deep adversarial review.
- Inherit `feedback_test_coverage_metrics.md`'s "contract coverage, not line count" rule when reviewing test-coverage claims in READMEs.

## Specific things to check across all 5 repos

| Pattern | Why it's stale | Replacement |
|---|---|---|
| "3 sibling repos" / "three siblings" | fakegenesys exists since S114 | "4 sibling repos" |
| "OSS-mature 13-item checklist" | Item 14 (contract_audit_test.go) added in S127 | "14-item checklist" |
| Old fakegenesys references like "planned at `../fakegenesys/`" | fakegenesys is shipped, on v0.2.0 | "shipped, current tag v0.2.0" |
| Slice IDs referencing removed scope (N3/N10/N13 → renamed in S104) | Already known stale | Verify the current names are used |
| Test-coverage claims with hard line counts | per `feedback_test_coverage_metrics.md` line counts are vanity | Replace with contract-coverage framing OR remove the count |
| Outdated workflow counts ("ci + release") | fakegenesys gained docker.yml in S125 → 3 workflows now | "ci + release + docker" |

## S131 — infrafactory

### Files

- `AGENTS.md` (~150 lines per earlier read; new § "Contract-coverage convention" added in S127)
- `README.md`

### Specific checks

- "ADR-0022" (harness pre-places `flow.yaml`) — verify this is still the latest ADR; if S130 introduces a new ADR, mention it.
- The cross-cutting fidelity-strategy comparison table — make sure it lists 4 fakes, not 3.
- The smoke-harness § and the new contract-audit § should be ADJACENT (sibling-wide conventions live together).

## S132 — mockway

### Files

- `AGENTS.md`
- `README.md` (has explicit per-section TOC starting at line 1; lots of detail)

### Specific checks

- Cross-repo `[mockway]` self-references should ONLY appear in cross-sibling context, not in mockway's own README's first-person sections.
- "Provider Compatibility Matrix" section likely has stale entries from before recent fixes.
- "What mockway catches" — check for stale-claim test counts.

## S133 — fakegcp

### Files

- `AGENTS.md`
- `README.md`

### Specific checks

- M-ticket references (M44/M45/M47/M49) — verify all are still load-bearing.
- The "Cross-repo e2e from infrafactory" section — make sure it references the current `cmd/s3router/` setup correctly.

## S134 — fakeaws

### Files

- `AGENTS.md`
- `README.md` (the most complex of the 4 siblings; references the 17-pass codex review)

### Specific checks

- "17-pass codex review" claim — still factually correct, but verify it doesn't imply ongoing reviews; the loop closed at S48.
- "Provider version pin" section — verify the pinned version is current.
- The "9 services" claim from S43–S48 — verify still accurate.

## S135 — fakegenesys

### Files

- `AGENTS.md`
- `README.md`

### Specific checks

- "Testing examples" stanza was just added in S126/S127 → make sure wording matches the canonical version landed in the other 3 siblings.
- "Quickstart" port `:8083` references.
- The "API compatibility" section table should match the 4-sibling pattern, not 3.

### Close-out (S135-T-final, Option C)

After the 5 PRs merge:

1. Update `infrafactory/STATUS.md` baseline pointer.
2. Append entry to `infrafactory/docs/status/ARCHIVE.md` § "2026-MM-DD AGENTS + README cleanup sweep".
3. Update `infrafactory/docs/NEXT_SESSION.md`.
4. Update `MEMORY.md` "Latest" entry.
5. Record cross-repo sweep as 7th instance of `reference_cross_repo_docs_sweep.md`.

## Out of scope

- New architectural docs.
- Reorganizing AGENTS.md's TOC.
- Adding new ADRs.
- Migration docs from old slice IDs to new ones (already handled in S104).
- Adding a docs/style-guide.md.
