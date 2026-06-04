# Ticket: Rename the learning-system internals to be human-readable

Status: **CLOSED 2026-06-04** — shipped as S104 in `docs/plans/mock-gaps-and-rename-plan.md`. See `docs/status/ARCHIVE.md` § "2026-06-04 mock-gaps drain + learning-system rename".
Filed: 2026-06-04
Decisions locked: 2026-06-04
Priority: P2 — quality-of-life / OSS-readiness
Estimated effort: 2-3 hr (S104 within the arc)
Scheme: **Fix/Avoid**
Cutover: **Atomic single PR** (no grace period)
Binary rename: **Yes** — `cmd/n10extract` → `cmd/extract-pitfall`

> This doc holds the locked-in rename specifics. The slice plan + sub-tickets live in `docs/plans/mock-gaps-and-rename-plan.md` § "S104".

## Problem

The auto-learning loop's vocabulary leaks internal slice IDs into user-facing surfaces. Symbols like **N3**, **N10**, **N13**, **M97** appear in:

- Source code function names that work via Codex/PR review comments
- `pitfalls/*.yaml` `source:` enum values (`learned_from_diff`, `learned_from_diff_avoid`)
- `docs/auto-learning-loop.md` headings + READMEs across all four repos
- CI ratchet test names (`TestPitfallsNoHumanSeeding`, `TestPitfallsNoMockActionableSeeds`, etc.)
- AGENTS.md sweep-protocol bullet
- Auto-memory pointer files

A new reader can't parse "N13 silent this sweep" without first decoding which slice introduced N13. As infrafactory moves toward OSS visibility (all four repos Apache-2.0, pending public flip), this is a real onboarding tax.

## Final rename (locked 2026-06-04)

### Code identifiers

| Current | New |
|---|---|
| `IsMockActionable` | `IsMockServerBug` |
| `ExtractPrescriptiveFix` (N10) | `ExtractFixPitfall` |
| `ExtractPrescriptiveAvoid` (N13) | `ExtractAvoidPitfall` |
| `ExtractLearnedPitfall` | `ExtractDescriptivePitfall` |
| `cmd/n10extract` (binary) | `cmd/extract-pitfall` |
| `--mode fix \| avoid` (CLI flag) | unchanged — already matches scheme |

### YAML `source:` enum values (data migration)

| Current | New |
|---|---|
| `source: learned` | `source: descriptive` |
| `source: learned_from_diff` | `source: fix` |
| `source: learned_from_diff_avoid` | `source: avoid` |

### Test names

| Current | New |
|---|---|
| `TestPitfallsNoMockActionableSeeds` | `TestPitfallsNoMockServerBugSeeds` |
| `TestPitfallsSourceEnum` | unchanged (test scope unchanged — just asserts the new enum set) |
| `TestPitfallsNoHumanSeeding` | unchanged |
| `TestPitfallsNoOPADuplication` | unchanged |

### Concept renames in docs

| Current | New |
|---|---|
| "M97 templates" | "diff-pattern templates" |
| "N3 classifier" | "mock-server-bug classifier" |
| "N10 extractor" | "fix-pitfall extractor" |
| "N13 extractor" | "avoid-pitfall extractor" |

Slice-ID references in **docs/comments** (`N3 / N10 / N13 / M97`) get rewritten as full names. Slice IDs in **commit messages / ARCHIVE / memory** stay — they're historical.

## Blast radius

Code identifiers (touching ~10 Go files):
- `internal/generator/pitfalls_learn.go` (+ `_test.go`)
- `internal/generator/prescriptive_extractor.go` (+ `_test.go`)
- `internal/generator/pitfalls_source_ratchet_test.go`
- `internal/generator/pitfalls_source_enum_test.go`
- `internal/cli/run_command.go`
- `internal/cli/test_command.go`
- `internal/cli/run_command_oscillation_test.go`
- `internal/feedback/normalize.go`
- `cmd/n10extract/main.go` (consider renaming binary to `learnfix` or similar)
- `cmd/pitfall-merge/main.go`

Data migration:
- `pitfalls/aws.yaml` (4 `source:` lines)
- `pitfalls/gcp.yaml` (8 `source:` lines)
- `pitfalls/scaleway.yaml` (10 `source:` lines)
- `docs/mock-gaps.md` (no source-enum field; just rename refs in surrounding text)

Docs:
- `docs/auto-learning-loop.md` (the explainer page; biggest doc change)
- `README.md` (cross-link blurb)
- `AGENTS.md` (sweep-protocol bullet)
- `../mockway/README.md`, `../fakegcp/README.md`, `../fakeaws/README.md` (cross-link blurbs)
- All `memory/*.md` files that reference N3/N10/N13/M97

CI ratchets:
- Loader in `pitfalls_loader.go` (or wherever source-enum is validated) needs to accept both old + new values during migration, OR ship the data migration in the same PR as the code change. Recommend: same PR — keeps the cutover atomic.

## Decisions (locked 2026-06-04)

1. **Names**: Fix/Avoid scheme (see table above).
2. **Binary**: `cmd/n10extract` → `cmd/extract-pitfall`. Makefile target + any scripts updated in same PR.
3. **Cutover**: Atomic single PR. Loader rejects old `learned*` values immediately; YAML migration ships in the same commit.
4. **Slice IDs in commit history / ARCHIVE / memory pointers**: leave alone. Historical changelog, not active vocabulary.

## Slice plan

Single slice (~2-3 hr), folds close-out:

| Sub-ticket | What |
|---|---|
| T1 | Rename code identifiers via `gopls rename` (preferred for correctness) or `grep -rl … \| xargs sed`. Run `go test ./...`. |
| T2 | Migrate `pitfalls/*.yaml` `source:` values: `learned`→`descriptive`, `learned_from_diff`→`fix`, `learned_from_diff_avoid`→`avoid`. Update loader enum constants. |
| T3 | Rename `cmd/n10extract` → `cmd/extract-pitfall`. Update Makefile target + `make build` output path + any references in scripts/sweep_39.sh. |
| T4 | Update `docs/auto-learning-loop.md` + AGENTS.md + 4 READMEs (infrafactory, mockway, fakegcp, fakeaws) + active memory pointers. Add a one-paragraph "renamed 2026-XX-XX from N3/N10/N13/M97" footnote in auto-learning-loop.md for searchability. |
| T5 | Update `bin/pitfall-merge` selective-discard logic: keeps `avoid` (was `learned_from_diff_avoid`), discards `fix` + `descriptive` (was `learned_from_diff` + `learned`). Update tests. |
| T6 | Run `make sweep-39` to confirm classifier still routes correctly + organic N13 emissions land with `source: avoid`. |
| T7 | One PR per repo touched. Fold close-out (STATUS + NEXT_SESSION + ARCHIVE) per Option C. |

## Why not now

The current `mock-gaps-verify-and-drain` arc is mid-execution. Sequence this **after** S102 + S103 land so:
- The rename doesn't compete with mock-gap fix-at-source work
- Whatever S103 confirms as real `IsMockActionable` gaps gets fixed *under the current names*, so the rename PR is a clean refactor without entangled semantic changes
- S102's classifier-widening (if any) ships under current names too

## Done when

- `git grep -E 'IsMockActionable|ExtractPrescriptive|ExtractLearnedPitfall|learned_from_diff'` returns zero hits in live code (only ARCHIVE / memory historical mentions allowed)
- `git grep -E '\bN3\b|\bN10\b|\bN13\b|\bM97\b'` returns zero hits in `docs/auto-learning-loop.md`, AGENTS.md, READMEs (only ARCHIVE / commits / memory historical mentions allowed)
- All four repos' READMEs use Fix/Avoid vocabulary
- `make sweep-39` passes; pitfalls reload from migrated YAML; CI ratchets green
- `bin/extract-pitfall` builds; `cmd/n10extract` directory is gone (renamed, not deleted-and-recreated, so git tracks the rename)
- `docs/auto-learning-loop.md` carries the footnote linking N3/N10/N13/M97 → new names so search still works
