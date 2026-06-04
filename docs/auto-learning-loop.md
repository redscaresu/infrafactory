# The auto-learning loop

How `infrafactory` gets better at writing infrastructure-as-code without anyone editing prompt rules by hand.

This page is for someone reading the codebase cold who wants the architecture in a single sitting. It collapses ADR-0012, ADR-0015, ADR-0018, and the implementations under `internal/generator/` into one explainer.

If you only read one section: jump to "Worked example" near the end.

## The one-paragraph summary

When a scenario's LLM-generated HCL fails and a later iteration's HCL passes, the system inspects the *diff* between the two — what got added, what got removed — and writes a rule into `pitfalls/<cloud>.yaml` that future runs of any scenario in that cloud will see at the top of their prompt. The rule is grounded in a real run, not authored by hand. A mock-server-bug classifier filters out failures the LLM can't fix; three extractors plus a diff-pattern template pass decide which of the rest generate rules, what kind, and how to phrase them. Four CI ratchets guarantee the file never accumulates garbage. A sweep-time protocol decides which new rules survive past the sweep itself.

## The classifier + extractors

Failures don't all deserve the same response. The pipeline routes each one based on its shape.

| Stage | When it fires | What it produces | `source:` value |
|---|---|---|---|
| **Mock-server-bug classifier** (`IsMockServerBug`) | Failure detail contains a mock-bug substring (e.g. `plugin did not respond`, `access_token_type_unsupported`, `couldn't find resource`) | Routes the failure AWAY from pitfalls — it lands in `docs/mock-gaps.md` for the matching sibling repo (fakeaws/fakegcp/mockway). | n/a — not a pitfall |
| **Fix-pitfall extractor** (`ExtractFixPitfall`) | Run reaches `target_reached` AFTER ≥1 failing iteration AND the fix was an *addition* to the HCL | A pitfall body that shows the HCL snippet the LLM added to clear the failure. | `fix` |
| **Avoid-pitfall extractor** (`ExtractAvoidPitfall`) | Same trigger, but the fix was a *deletion* | A pitfall saying "do NOT use attribute X on resource Y — it caused failure Z that was resolved by removing it." | `avoid` |
| **Diff-pattern templates** | Failure detail matches a pattern from a small handcoded set (subnetwork attachment, 501 Not Implemented, AWS destroy blockers, etc.) | A prescriptive rule from the template's body. Fires when the fix/avoid extractors wouldn't (run terminated stuck, not converged). | `fix` |
| **Descriptive fallback** (`ExtractDescriptivePitfall`) | Run terminates `stuck` or `repair_budget_exhausted` AND the classifier doesn't claim it AND no template matches | Captures the failure detail verbatim as a "this happened, don't trip on it" entry. Symptom-only — useful but less actionable than the diff-derived forms. | `descriptive` |

Code: `internal/generator/pitfalls_learn.go` (classifier, descriptive, IsMockServerBug), `internal/generator/prescriptive_extractor.go` (fix, avoid, diff-pattern templates).

### Why three flavors, not one

`descriptive` is cheap but speculative — it just echoes the error. The LLM gets "scenario X failed with error Y" but no guidance on what to change.

`fix` sees a *real successful fix*: what bytes were added between iter N and iter N+1 that cleared the error. The pitfall body literally contains those bytes. The LLM can copy them.

`avoid` sees a *real successful deletion*: what attribute or block disappeared. The pitfall body says "don't use this." Same evidentiary basis as `fix` but the inverse shape.

The fix + avoid extractors only fire on successful runs (the diff requires a passing iteration to compare against). The classifier and descriptive fallback fire on stuck/budget terminations. Diff-pattern templates fire on either.

## Storage

`pitfalls/<cloud>.yaml`. One file per cloud (`aws`, `gcp`, `scaleway`). Loaded at runtime by the scenario's `cloud` field. Rendered into the phase-2 prompt as a `<pitfalls>` block.

```yaml
provider: gcp
pitfalls:
    - resource: google_compute_instance
      rule: Always declare a `google_compute_network` AND at least one ...
      source: descriptive
      discovered_from: gcp-full-stack
    - resource: aws_subnet
      rule: 'Do NOT use attribute `map_public_ip_on_launch` on `aws_subnet` ...'
      source: avoid
      discovered_from: aws-eks
```

`source` is a closed enum: `descriptive`, `fix`, `avoid`. `TestPitfallsSourceEnum` rejects anything else.

## The ratchets

Four CI tests guard the contract.

| Ratchet | Where | What it rejects |
|---|---|---|
| `TestPitfallsNoHumanSeeding` | `internal/generator/pitfalls_source_ratchet_test.go` | Entries with `source: seed` or `source: static`. The system seeds itself from real runs; hand-authored entries are forbidden. |
| `TestPitfallsNoMockServerBugSeeds` | `internal/generator/pitfalls_learn_test.go` | Entries whose rule body contains a mock-actionable substring (e.g. `plugin did not respond`). Those belong in `docs/mock-gaps.md`, not pitfalls. |
| `TestPitfallsNoOPADuplication` | `internal/generator/pitfalls_opa_dedup_test.go` | Entries whose rule body verbatim-copies an OPA policy's `msg := sprintf(...)` literal. OPA is the canonical carrier; the pitfall would waste prompt tokens. |
| `TestPitfallsSourceEnum` | `internal/generator/pitfalls_source_enum_test.go` | Entries with a `source` value outside the closed enum. Catches typos like `avold`. |

Plus a sibling test for prompts: `TestPromptsNoOPAPolicyCitations` rejects prompt files that name an existing `.rego` policy by name — those rules belong as Category B retirements per ADR-0018.

## The sweep-time protocol

A 39-scenario `make sweep-39` produces a lot of auto-learned entries — most of them are noise from this specific run. The protocol decides which survive.

Default: discard everything. The next sweep will re-derive whatever's load-bearing.

Exception: `avoid` entries survive. Rationale — the avoid extractor only fires when iter N failed, iter N+1 passed, AND the diff was a deletion. The output is grounded in a successful run, not a speculative match. `fix` is more speculative (the addition might have been one of several changes; we attribute it to the fix); `descriptive` is the most speculative (just an error echo).

Implementation: `bin/pitfall-merge` (`cmd/pitfall-merge/`). Reads pre + post pitfall YAMLs, writes a merged file: pre verbatim + post entries whose `source` is in `--keep` (default `avoid`), deduped by `(resource, rule)`. Invoked per-cloud at sweep end by `scripts/sweep_39.sh`.

Sweep also emits an `AVOID_EMISSIONS=N` line. Zero across multiple sweeps is a soft watchdog signal — either the extractor broke or the LLM stopped making deletion-recoverable mistakes.

## mock-gaps drainage

`docs/mock-gaps.md` is a git-ignored runtime artifact. The N3 classifier appends to it whenever a sweep surfaces a mock-actionable failure — never edited by hand. Each entry has a `discovered_from` scenario and a failure signal.

Entries accumulate across sweeps and don't self-prune. If a mock-source bug is fixed in `fakeaws`/`fakegcp`/`mockway`, the matching entry stays in the file until a human prunes it.

**Drainage protocol** (run after each major sustain arc, or whenever the file grows past ~10 entries):

1. For each entry, run the `discovered_from` scenario once: `./bin/infrafactory run scenarios/training/<scenario>.yaml --clean`.
2. If the scenario passes → entry is stale (the underlying mock bug got fixed without anyone updating this file). Delete the row.
3. If the scenario fails with the same signal → entry is real. Open a PR against the appropriate sibling mock with a failing handler test + fix.
4. After all entries are processed, blow the file away locally. The next sweep regenerates it from scratch if anything is still broken.

**Don't** treat the file as a stable queue you can pick from later. The entries are point-in-time snapshots; a mock fix elsewhere can render an entry stale without notice. Either drain promptly or replay-verify before acting on an entry.

The 2026-06-04 `mock-gaps-and-rename` arc drained 13 stale entries that had accumulated across the 2026-06-01 / 02 sweeps. All 13 `discovered_from` scenarios passed in the contemporaneous probe sweep — the entries were artifacts of mock fixes shipped in S77, S96, S100, etc. that nobody had pruned.

## Worked example: aws-subnet `map_public_ip_on_launch`

The first organic `avoid` entry, captured 2026-06-04 during sustain re-validation sweep 1/3.

### What happened in the run

Scenario: `aws-eks`. The LLM-generated HCL for iteration N included an `aws_subnet` block with `map_public_ip_on_launch = true`. The mock-side state for that attribute didn't persist correctly across an Update wait-loop, and `tofu apply` failed with a timeout.

In iteration N+1, the LLM (seeing the failure detail in the prompt feedback) generated a new HCL — without the `map_public_ip_on_launch` attribute. Apply succeeded. The run terminated `target_reached`.

### What the extractor saw

`ExtractAvoidPitfall` runs at end-of-run on successful termination. It walks the iter-pair `(iter N, iter N+1)` HCL files, finds the `aws_subnet` resource block in both, and diffs them. One line was deleted between N and N+1:

```hcl
- map_public_ip_on_launch = true
```

The extractor cross-checks: was the failure detail about `aws_subnet`? Yes. Was the deleted attribute the one named in the failure detail? Yes (case-insensitive). Both checks pass → emit the pitfall.

### What got written

```yaml
- resource: aws_subnet
  rule: 'exit status 1 | stderr: ╷ Do NOT use attribute `map_public_ip_on_launch` on `aws_subnet` — observed in scenario "aws-eks" to cause the failure above.'
  source: avoid
  discovered_from: aws-eks
```

### What happened at sweep-end

The sweep's `bin/pitfall-merge` step ran. Pre-sweep `pitfalls/aws.yaml` did not contain this entry; post-sweep did. The `source` was `avoid` — in the keep set. The entry survived. `AVOID_EMISSIONS=1` got logged.

### What happens on the next run

Any AWS scenario that runs after this point loads `pitfalls/aws.yaml` and renders the avoid entry into the phase-2 prompt. When the LLM is generating HCL for an `aws_subnet`, it sees the warning, and doesn't add `map_public_ip_on_launch`. The bug class is now closed — without anyone editing a prompt rule.

If a future fakeaws fix makes the attribute work correctly, the pitfall will *still* be in the file. That's pruning territory — `feedback_sweep_protocol.md` permits removing now-stale entries after the underlying mock-source bug is fixed.

## What the system can't (yet) do

- **Paraphrased pitfalls** — both the fix and avoid extractors capture verbatim bytes. They can't summarize ("any subnet with public IP enabled fails on the wait-loop") or generalize across resources.
- **Cross-cloud transfer** — the GCP equivalent of the same bug class wouldn't be inferred from an AWS pitfall. Each cloud's pitfalls file is independent.
- **Retroactive pruning** — when a mock-source bug is fixed, the matching pitfall stays in the file. Pruning is manual (and explicitly permitted by the sweep protocol).
- **Long-tail one-shot mistakes** — fix/avoid require successful convergence. If the LLM never recovers, only the descriptive fallback or a diff-pattern template will produce a pitfall.

## Key files at a glance

| File | Role |
|---|---|
| `internal/generator/pitfalls.go` | `PitfallEntry`/`PitfallsFile` types + load + render |
| `internal/generator/pitfalls_learn.go` | mock-server-bug classifier, descriptive fallback, append |
| `internal/generator/prescriptive_extractor.go` | fix + avoid extractors, diff-pattern templates |
| `internal/generator/pitfalls_source_*_test.go` | The four ratchets |
| `cmd/extract-pitfall/` | Operator CLI for force-extracting a fix or avoid pitfall from a recorded run pair |
| `cmd/pitfall-merge/` | Sweep-end selective restore |
| `scripts/sweep_39.sh` | The sweep harness that invokes everything |
| `pitfalls/{aws,gcp,scaleway}.yaml` | The runtime carrier |
| `docs/decisions/0012-dynamic-pitfalls.md` | The original architecture decision |
| `docs/decisions/0015-classifier-routing.md` | mock-server-bug classifier rationale |
| `docs/decisions/0018-n11-retirement-criteria.md` | Category A/B/C retirement framework |

## Historical note: the slice-ID names

Earlier code, ADRs, plan docs, and commit messages refer to the components by their introducing-slice IDs:

| Slice ID | Concept (this doc) | Code identifier |
|---|---|---|
| **N3** | mock-server-bug classifier | `IsMockServerBug` (was `IsMockActionable`) |
| **N10** | fix-pitfall extractor | `ExtractFixPitfall` (was `ExtractPrescriptiveFix`) |
| **N13** | avoid-pitfall extractor | `ExtractAvoidPitfall` (was `ExtractPrescriptiveAvoid`) |
| **M97** | diff-pattern templates | (no exported identifier — internal template set) |

The 2026-06-04 `mock-gaps-and-rename` arc (S104) renamed everything to the concept-based vocabulary used throughout this doc. The slice IDs remain in ARCHIVE entries, commit history, and ADRs as historical attribution — they're not part of the active vocabulary. The `source:` enum values likewise migrated: `learned` → `descriptive`, `learned_from_diff` → `fix`, `learned_from_diff_avoid` → `avoid`.
