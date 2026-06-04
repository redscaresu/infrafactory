# The auto-learning loop

How `infrafactory` gets better at writing infrastructure-as-code without anyone editing prompt rules by hand.

This page is for someone reading the codebase cold who wants the architecture in a single sitting. It collapses ADR-0012, ADR-0015, ADR-0018, and the implementations under `internal/generator/` into one explainer.

If you only read one section: jump to "Worked example" near the end.

## The one-paragraph summary

When a scenario's LLM-generated HCL fails and a later iteration's HCL passes, the system inspects the *diff* between the two — what got added, what got removed — and writes a rule into `pitfalls/<cloud>.yaml` that future runs of any scenario in that cloud will see at the top of their prompt. The rule is grounded in a real run, not authored by hand. Three extractors (N3, N10, N13) plus a template pass (M97) decide which failures generate rules, what kind, and how to phrase them. Four CI ratchets guarantee the file never accumulates garbage. A sweep-time protocol decides which new rules survive past the sweep itself.

## The four extractors

Failures don't all deserve the same response. The pipeline routes each one based on its shape.

| Extractor | When it fires | What it produces | `source:` value |
|---|---|---|---|
| **N3** (`IsMockActionable`) | Failure detail contains a mock-bug substring (e.g. `plugin did not respond`, `access_token_type_unsupported`, `couldn't find resource`) | Routes the failure AWAY from pitfalls — it lands in `docs/mock-gaps.md` for the matching sibling repo (fakeaws/fakegcp/mockway). | n/a — not a pitfall |
| **N10** (`ExtractPrescriptiveFix`) | Run reaches `target_reached` AFTER ≥1 failing iteration AND the fix was an *addition* to the HCL | A pitfall body that shows the HCL snippet the LLM added to clear the failure. | `learned_from_diff` |
| **N13** (`ExtractPrescriptiveAvoid`) | Same trigger, but the fix was a *deletion* | A pitfall saying "do NOT use attribute X on resource Y — it caused failure Z that was resolved by removing it." | `learned_from_diff_avoid` |
| **M97 templates** | Failure detail matches a pattern from a small handcoded set (subnetwork attachment, 501 Not Implemented, AWS destroy blockers, etc.) | A prescriptive rule from the template's body. Fires when N10/N13 wouldn't (run terminated stuck, not converged). | `learned_from_diff` |
| **Descriptive fallback** (`ExtractLearnedPitfall`) | Run terminates `stuck` or `repair_budget_exhausted` AND N3 doesn't claim it AND no template matches | Captures the failure detail verbatim as a "this happened, don't trip on it" entry. Symptom-only — useful but less actionable than N10/N13. | `learned` |

Code: `internal/generator/pitfalls_learn.go` (N3, descriptive, IsMockActionable), `internal/generator/prescriptive_extractor.go` (N10, N13, M97 templates).

### Why three flavors, not one

`learned` is cheap but speculative — it just echoes the error. The LLM gets "scenario X failed with error Y" but no guidance on what to change.

`learned_from_diff` (N10) sees a *real successful fix*: what bytes were added between iter N and iter N+1 that cleared the error. The pitfall body literally contains those bytes. The LLM can copy them.

`learned_from_diff_avoid` (N13) sees a *real successful deletion*: what attribute or block disappeared. The pitfall body says "don't use this." Same evidentiary basis as N10 but the inverse shape.

N10 and N13 only fire on successful runs (the diff requires a passing iteration to compare against). N3 and the descriptive fallback fire on stuck/budget terminations. M97 templates fire on either.

## Storage

`pitfalls/<cloud>.yaml`. One file per cloud (`aws`, `gcp`, `scaleway`). Loaded at runtime by the scenario's `cloud` field. Rendered into the phase-2 prompt as a `<pitfalls>` block.

```yaml
provider: gcp
pitfalls:
    - resource: google_compute_instance
      rule: Always declare a `google_compute_network` AND at least one ...
      source: learned
      discovered_from: gcp-full-stack
    - resource: aws_subnet
      rule: 'Do NOT use attribute `map_public_ip_on_launch` on `aws_subnet` ...'
      source: learned_from_diff_avoid
      discovered_from: aws-eks
```

`source` is a closed enum: `learned`, `learned_from_diff`, `learned_from_diff_avoid`. `TestPitfallsSourceEnum` rejects anything else.

## The ratchets

Four CI tests guard the contract.

| Ratchet | Where | What it rejects |
|---|---|---|
| `TestPitfallsNoHumanSeeding` (M91) | `internal/generator/pitfalls_source_ratchet_test.go` | Entries with `source: seed` or `source: static`. The system seeds itself from real runs; hand-authored entries are forbidden. |
| `TestPitfallsNoMockActionableSeeds` (S55+) | `internal/generator/pitfalls_learn_test.go` | Entries whose rule body contains a mock-actionable substring (e.g. `plugin did not respond`). Those belong in `docs/mock-gaps.md`, not pitfalls. |
| `TestPitfallsNoOPADuplication` (S82) | `internal/generator/pitfalls_opa_dedup_test.go` | Entries whose rule body verbatim-copies an OPA policy's `msg := sprintf(...)` literal. OPA is the canonical carrier; the pitfall would waste prompt tokens. |
| `TestPitfallsSourceEnum` (S94) | `internal/generator/pitfalls_source_enum_test.go` | Entries with a `source` value outside the closed enum. Catches typos like `learned_from_diff_avold`. |

Plus a sibling test for prompts (S99): `TestPromptsNoOPAPolicyCitations` rejects prompt files that name an existing `.rego` policy by name — those rules belong as Category B retirements per ADR-0018.

## The sweep-time protocol

A 39-scenario `make sweep-39` produces a lot of auto-learned entries — most of them are noise from this specific run. The protocol decides which survive.

Default: discard everything. The next sweep will re-derive whatever's load-bearing.

Exception: `learned_from_diff_avoid` (N13) survives. Rationale — N13 only fires when iter N failed, iter N+1 passed, AND the diff was a deletion. The output is grounded in a successful run, not a speculative match. N10's `learned_from_diff` is more speculative (the addition might have been one of several changes; we attribute it to the fix); the descriptive `learned` is the most speculative (just an error echo).

Implementation: `bin/pitfall-merge` (`cmd/pitfall-merge/`). Reads pre + post pitfall YAMLs, writes a merged file: pre verbatim + post entries whose `source` is in `--keep` (default `learned_from_diff_avoid`), deduped by `(resource, rule)`. Invoked per-cloud at sweep end by `scripts/sweep_39.sh`.

Sweep also emits a `N13_EMISSIONS=N` line. Zero across multiple sweeps is a soft watchdog signal — either N13 is broken or the LLM stopped making deletion-recoverable mistakes.

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

The first organic N13 entry, captured 2026-06-04 during sustain re-validation sweep 1/3.

### What happened in the run

Scenario: `aws-eks`. The LLM-generated HCL for iteration N included an `aws_subnet` block with `map_public_ip_on_launch = true`. The mock-side state for that attribute didn't persist correctly across an Update wait-loop, and `tofu apply` failed with a timeout.

In iteration N+1, the LLM (seeing the failure detail in the prompt feedback) generated a new HCL — without the `map_public_ip_on_launch` attribute. Apply succeeded. The run terminated `target_reached`.

### What the extractor saw

`ExtractPrescriptiveAvoid` runs at end-of-run on successful termination. It walks the iter-pair `(iter N, iter N+1)` HCL files, finds the `aws_subnet` resource block in both, and diffs them. One line was deleted between N and N+1:

```hcl
- map_public_ip_on_launch = true
```

The extractor cross-checks: was the failure detail about `aws_subnet`? Yes. Was the deleted attribute the one named in the failure detail? Yes (case-insensitive match per S64). Both checks pass → emit the pitfall.

### What got written

```yaml
- resource: aws_subnet
  rule: 'exit status 1 | stderr: ╷ Do NOT use attribute `map_public_ip_on_launch` on `aws_subnet` — observed in scenario "aws-eks" to cause the failure above.'
  source: learned_from_diff_avoid
  discovered_from: aws-eks
```

### What happened at sweep-end

The sweep's `bin/pitfall-merge` step ran. Pre-sweep `pitfalls/aws.yaml` did not contain this entry; post-sweep did. The `source` was `learned_from_diff_avoid` — in the keep set. The entry survived. `N13_EMISSIONS=1` got logged.

### What happens on the next run

Any AWS scenario that runs after this point loads `pitfalls/aws.yaml` and renders the N13 entry into the phase-2 prompt. When the LLM is generating HCL for an `aws_subnet`, it sees the warning, and doesn't add `map_public_ip_on_launch`. The bug class is now closed — without anyone editing a prompt rule.

If a future fakeaws fix makes the attribute work correctly, the pitfall will *still* be in the file. That's pruning territory — `feedback_sweep_protocol.md` permits removing now-stale entries after the underlying mock-source bug is fixed.

## What the system can't (yet) do

- **Paraphrased pitfalls** — both N10 and N13 capture verbatim bytes. They can't summarize ("any subnet with public IP enabled fails on the wait-loop") or generalize across resources.
- **Cross-cloud transfer** — the GCP equivalent of the same bug class wouldn't be inferred from an AWS pitfall. Each cloud's pitfalls file is independent.
- **Retroactive pruning** — when a mock-source bug is fixed, the matching pitfall stays in the file. Pruning is manual (and explicitly permitted by the sweep protocol).
- **Long-tail one-shot mistakes** — N10/N13 require successful convergence. If the LLM never recovers, only the descriptive fallback or an M97 template will produce a pitfall.

## Key files at a glance

| File | Role |
|---|---|
| `internal/generator/pitfalls.go` | `PitfallEntry`/`PitfallsFile` types + load + render |
| `internal/generator/pitfalls_learn.go` | N3 classifier, descriptive fallback, append |
| `internal/generator/prescriptive_extractor.go` | N10, N13, M97 templates |
| `internal/generator/pitfalls_source_*_test.go` | The four ratchets |
| `cmd/pitfall-merge/` | Sweep-end selective restore |
| `scripts/sweep_39.sh` | The sweep harness that invokes everything |
| `pitfalls/{aws,gcp,scaleway}.yaml` | The runtime carrier |
| `docs/decisions/0012-dynamic-pitfalls.md` | The original architecture decision |
| `docs/decisions/0015-classifier-routing.md` | N3 classifier rationale |
| `docs/decisions/0018-n11-retirement-criteria.md` | Category A/B/C retirement framework |
