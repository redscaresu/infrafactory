# Arc: pitfall pruning automation

Status: **SHELVED 2026-06-05** — `pitfalls/*.yaml` isn't currently a problem (22 entries total across 3 clouds; not growing out of control). Reactivate when the file grows past ~50 entries OR when a sweep flake gets traced to a stale entry.
Owner: next-session claude (designed for autonomous execution)
Follows: `fakeaws-kms-soft-delete-plan.md` (closed 2026-06-05 with S106)
Shape: goal-named variable-length arc per AGENTS.md (1 slice, ~2-3 hr)

> **Shelf note**: design discussion on 2026-06-05 surfaced a key issue — single-replay is a weak signal for stochastic LLM failures. If/when this arc is reactivated, the design should use **N-trial replay (N=3 or 5) + label `REMOVE` outputs as `REMOVE_CANDIDATE`** for human review, OR add the schema needed for "load-bearing hits over time" tracking, OR build a quarantine-then-delete two-step flow. See the conversation that produced this doc for the full reasoning.

## Big picture — what this is and why

`pitfalls/<cloud>.yaml` is the LLM-side learning queue. Every entry is an auto-derived rule the system has used to prevent repeat failures (see `docs/auto-learning-loop.md` for the full architecture). The auto-learning loop only **adds**; nothing ever removes entries except sweep-end teardown of `fix`/`descriptive` (S94 — preserved are `avoid` entries per the renamed vocabulary). Over time, three things make a pitfall stale:

1. **Mock fidelity bugs get fixed**. The pitfall was originally needed because (say) fakeaws's KMS handler didn't model soft-delete — once S106 fixes that at source, the LLM-side pitfall that taught Claude to avoid the failure shape is no longer load-bearing.
2. **Other pitfalls subsume the lesson**. Two pitfalls about the same resource may accumulate; the more specific one carries the load and the older general one is dead weight in the prompt.
3. **The LLM context shifts under it**. Newer prompts or scenarios changed in ways that mean the original failure mode no longer reproduces even without the pitfall.

There's currently no protocol for finding stale entries. The `mock-gaps.md` drainage protocol (S103 + `docs/auto-learning-loop.md` § "mock-gaps drainage") is the closest existing pattern — replay each `discovered_from`, see if the failure reproduces. This arc builds the **automated version** of that protocol for the LLM-side learning queue.

## Why "automated" matters

Manual pruning means a human reads the YAML, picks an entry, removes it, runs a scenario, replays, decides. Slow + speculative. Automation lets the system itself answer "is this rule still load-bearing?" with a concrete test: remove the entry, replay `discovered_from`, observe whether the failure mode re-surfaces.

The output of this arc is one new tool (`cmd/pitfall-prune`) that:
- Walks every entry in `pitfalls/<cloud>.yaml`.
- Per entry: removes it, runs `infrafactory run scenarios/training/<discovered_from>.yaml --clean`, classifies the outcome, restores the entry.
- Writes a report flagging stale (now-dispensable) entries for human or autonomous-loop action.

It is **not** an automatic-deletion tool. It produces a report; a separate manual or arc step does the removal. Same shape as `mock-gaps.md` — the file is the queue, draining is a separate intentional action.

## Architecture context (a fresh agent will need this)

- **Pitfall storage**: `pitfalls/{aws,gcp,scaleway}.yaml`. Each file has shape:
  ```yaml
  provider: <cloud>
  pitfalls:
      - resource: <terraform_resource_type>
        rule: <prose or HCL snippet>
        source: descriptive | fix | avoid
        discovered_from: <scenario-name>
  ```
- **Source enum**:
  - `descriptive` — failure-message echo (least reliable signal).
  - `fix` — extracted from the ADDED side of an HCL diff between failing iter N and passing iter N+1 (`ExtractFixPitfall`).
  - `avoid` — extracted from the REMOVED side of the same diff (`ExtractAvoidPitfall`).
  - Anything else is forbidden by `TestPitfallsSourceEnum`.
- **Source-of-truth code**: `internal/generator/pitfalls.go` (types + load), `internal/generator/pitfalls_learn.go` (classifier + descriptive), `internal/generator/prescriptive_extractor.go` (fix/avoid).
- **Scenario file paths**: `scenarios/training/<discovered_from>.yaml`. Always present for a valid entry.
- **`infrafactory run` invocation**: `./bin/infrafactory run scenarios/training/<scenario>.yaml --clean --config infrafactory.yaml`. Exit 0 = `target_reached`. Non-zero = stuck / repair_budget_exhausted / transport_failed.
- **Existing tooling parallels**:
  - `cmd/pitfall-merge/` — sweep-end selective restore (keeps `avoid`, discards `fix`/`descriptive`).
  - `cmd/extract-pitfall/` — operator CLI to force-extract a fix/avoid candidate from a recorded run-pair (was `cmd/n10extract` pre-rename).
- **The classifier `IsMockServerBug`** (`internal/generator/pitfalls_learn.go:307`) — fires on substrings like `plugin did not respond`, `access_token_type_unsupported`, `couldn't find resource`, `501 not implemented`, `resourcenotfoundexception`, etc. If a scenario fails with one of those signals after the pitfall is removed, the failure has been re-classified as a mock-side bug; the pitfall should NOT have been there in the first place (`TestPitfallsNoMockServerBugSeeds` exists for exactly this), so flag for removal.

## Standing rules

Inherit from prior arcs:
- **Fix at source**: if pruning surfaces that a mock-side bug should have been fixed in fakeaws/fakegcp/mockway, file that as a follow-up arc rather than carrying a brittle pitfall.
- **Mandatory close-out per Option C**: folded into the single slice.
- **Per-slice naming**: this is **S107**.

## Slice

| Slice | Title | Effort |
|---|---|---|
| S107 | `cmd/pitfall-prune` tool + report format + close-out | ~2-3 hr |

## S107 — pitfall-prune tool

### Decisions to lock before coding

1. **Tool location and name**. `cmd/pitfall-prune/main.go`. Built into `bin/pitfall-prune` by `make build` (add to Makefile alongside `extract-pitfall` and `pitfall-merge`).

2. **What's "load-bearing" vs "stale"?**
   - **Load-bearing** (KEEP): scenario fails with a real LLM-side error after the entry is removed (same `discovered_from`, same resource type implicated).
   - **Stale** (REMOVE candidate): scenario succeeds after the entry is removed.
   - **Inconclusive** (RE-TEST): scenario fails with a transport-shape error (Claude rate limit, OpenTofu provider-registry blip — see S101 predicate in `scripts/sweep_39.sh::is_transport_failed_shape`). One retry; if still inconclusive, label as such in the report.
   - **Should-not-have-existed** (REMOVE + FILE MOCK BUG): scenario fails with a `IsMockServerBug`-classified shape. The entry slipped past `TestPitfallsNoMockServerBugSeeds` (or pre-dates the ratchet). Flag for removal AND file a `docs/mock-gaps.md`-style entry.

3. **Which entries to test**:
   - Default: all entries in `pitfalls/{aws,gcp,scaleway}.yaml`.
   - Flag `--cloud aws` (or comma-separated) to narrow.
   - Flag `--resource aws_subnet` to test only entries matching a resource type.
   - Flag `--source avoid` to test only one source flavor.

4. **State management during the run**:
   - Snapshot the original `pitfalls/<cloud>.yaml` once at start.
   - For each entry: write a temp YAML with that entry removed; point the run at it via `--pitfalls-dir` (already supported by `infrafactory run` through the config? **verify in code**). If not supported, the tool's only way to test is to mutate `pitfalls/<cloud>.yaml` in place + restore after each test. Plan for the in-place fallback.
   - At the end, restore the original.
   - Atomic restore even on Ctrl-C / SIGTERM — defer + signal handler.

5. **Output**: a Markdown report file (`/tmp/pitfall-prune-report.md` or `--out`-overridable) with one table per cloud:

   ```markdown
   ## aws (4 entries tested)

   | Resource | Source | discovered_from | Verdict | Detail |
   |---|---|---|---|---|
   | aws_subnet | avoid | aws-eks | KEEP | scenario failed with same resource shape after removal |
   | aws_db_instance | fix | aws-rds | REMOVE | scenario passed without the entry |
   | aws_kms_key | descriptive | aws-secrets-manager | REMOVE_MOCK | scenario failed with mock-actionable signal `resourcenotfoundexception` — file fakeaws bug |
   | aws_iam_role | descriptive | aws-iam | INCONCLUSIVE | transport failure on both attempts |
   ```

6. **Verdict legend**: `KEEP` / `REMOVE` / `REMOVE_MOCK` / `INCONCLUSIVE`.

7. **What we do NOT do in this slice**:
   - **No automatic deletion**. The tool produces a report; humans (or a follow-on arc) act on it.
   - **No integration into `sweep_39.sh`**. Pruning is a periodic-maintenance task, not a sweep step. Document the cadence in `docs/auto-learning-loop.md`.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S107-T1 | Verify whether `infrafactory run` supports a `--pitfalls-dir` override or equivalent injection point. Grep `internal/cli/run_command.go` + `internal/config/`. If yes, use it. If no, plan for in-place mutate-and-restore. Document the chosen approach in the tool's package comment. | P0 | — |
| S107-T2 | `cmd/pitfall-prune/main.go`: skeleton with flags (`--cloud`, `--resource`, `--source`, `--out`, `--config`). Parse pitfalls YAML, enumerate entries. | P0 | T1 |
| S107-T3 | Per-entry test loop: remove entry, invoke `infrafactory run scenarios/training/<discovered_from>.yaml --clean`, capture exit + log, restore entry, classify outcome. | P0 | T2 |
| S107-T4 | Outcome classifier: parse the scenario's `app.log` for `terminal_reason` (`target_reached` / `stuck` / `repair_budget_exhausted`); for non-success, grep the failure detail for `IsMockServerBug` substrings to distinguish `REMOVE_MOCK` from `KEEP`; for transport-failure shape (per S101 predicate), classify as `INCONCLUSIVE` and retry once. | P0 | T3 |
| S107-T5 | Atomic state restore: defer + signal handler so a Ctrl-C in the middle of a 22-entry run doesn't leave `pitfalls/*.yaml` mutated. Snapshot the original file once at start; write it back from the snapshot on cleanup (not from the in-memory state — failure-mode safety). | P0 | T2 |
| S107-T6 | Markdown report writer per the format above. Default output `/tmp/pitfall-prune-report.md`; `--out` flag for custom. | P0 | T4 |
| S107-T7 | Unit tests for the tool's pure logic: outcome classification, YAML round-trip with entry removal, report format. Use the same `testutil.NewTestServer` pattern as fakeaws if helpful, but most of the tool's logic is pure I/O over local files. | P0 | T3, T4 |
| S107-T8 | Wire into Makefile: `go build -o bin/pitfall-prune ./cmd/pitfall-prune` alongside `extract-pitfall` and `pitfall-merge`. | P0 | T2 |
| S107-T9 | Document the tool + cadence in `docs/auto-learning-loop.md` (new § "pitfall pruning", paralleling the existing § "mock-gaps drainage"). Add a sub-bullet to AGENTS.md's sweep-protocol bullet noting the new periodic-maintenance task. | P0 | T6 |
| S107-T10 | Smoke-validate end-to-end: run `bin/pitfall-prune --cloud aws` against the current `pitfalls/aws.yaml` (4 entries). Verify report is sensible: at least one entry classified clearly, no atomic-restore failures, original YAML untouched after run. | P0 | T1-T9 |
| S107-T11 | One PR. **Arc close-out folded in** (STATUS + NEXT_SESSION + ARCHIVE per Option C). | P0 | T1-T10 |

### Exit criteria

- `bin/pitfall-prune` builds; standalone CLI works against the 22 entries currently in `pitfalls/{aws,gcp,scaleway}.yaml`.
- Markdown report emitted at `/tmp/pitfall-prune-report.md` (default).
- Original YAML files unchanged after the tool runs (atomic restore verified).
- `docs/auto-learning-loop.md` documents the tool and the pruning cadence.
- `AGENTS.md` sweep-protocol bullet mentions pruning as periodic-maintenance.
- ARCHIVE close-out for the arc lands.

## Cost estimate

22 entries × ~30-60s per scenario replay × possible 1-retry on transport ≈ **15-30 minutes of LLM calls per full prune run**. The tool is invoked manually (not in CI), so cost is bounded by operator cadence. Cheap enough to run after each major sustain arc; expensive enough that we don't want it in `make sweep-39`.

## Why this shape

Single slice because the tool is self-contained, the I/O is local, and the test pattern (replay-and-classify) is already established by the mock-gaps drainage protocol — this is the same idea applied to the LLM-side queue. Splitting into "build tool" + "use tool" slices would be artificial: the first run IS the validation that the tool works.

If the tool surfaces something unexpected during the smoke validation (T10) — e.g. half the entries classify INCONCLUSIVE because of transport flakes — that becomes a follow-up arc, not an in-slice fix. The exit criterion is "the tool runs and emits a sensible report," not "the report shows zero stale entries."

## Autonomous-execution loop prompt

```
/loop until S107 in docs/plans/pitfall-pruning-automation-plan.md is complete: exit criteria met, PR merged with green CI, STATUS + NEXT_SESSION updated.

Read docs/NEXT_SESSION.md first for the prior arc's handoff, then docs/plans/pitfall-pruning-automation-plan.md for the slice definition. All prior standing rules apply (slices-54-62 through fakeaws-kms-soft-delete).

S107 folds the mandatory ARCHIVE + NEXT_SESSION close-out per the Option C arc shape — no separate close-out slice.

Authority: open + merge PRs with `gh pr merge <N> --squash --admin --delete-branch` once CI is green.

Implementation reference: `cmd/pitfall-merge/` and `cmd/extract-pitfall/` are the closest existing tools. Same I/O style, same flag patterns.

The tool DOES NOT auto-delete entries. It produces a Markdown report flagging stale entries for human or follow-on-arc action — same shape as mock-gaps.md.

Stop only when: (a) S107 complete OR (b) you genuinely cannot proceed (document the blocker in NEXT_SESSION + stop).
```

## Fresh-context checklist

The autonomous prompt assumes the agent reads, in order:

1. `AGENTS.md` (Option C goal-named arcs; sweep-protocol bullet)
2. `docs/NEXT_SESSION.md`
3. This file (`docs/plans/pitfall-pruning-automation-plan.md`)
4. `STATUS.md`
5. `docs/auto-learning-loop.md` — full architecture; the new "pitfall pruning" section will live here
6. `pitfalls/{aws,gcp,scaleway}.yaml` — the 22 entries to test against
7. `cmd/pitfall-merge/main.go` — closest existing tool for I/O patterns
8. `cmd/extract-pitfall/main.go` — second-closest, also useful for run-dir conventions
9. `internal/generator/pitfalls.go` — `PitfallEntry` + `PitfallsFile` types
10. `internal/generator/pitfalls_learn.go::IsMockServerBug` — classifier signals to match in T4
11. `scripts/sweep_39.sh::is_transport_failed_shape` — S101 transport predicate (for INCONCLUSIVE classification)
12. `docs/decisions/0012-dynamic-pitfalls.md`, `docs/decisions/0019-learning-system-vocabulary.md` — relevant ADRs

## Open questions to resolve in T1

- Does `infrafactory run` support a pitfalls-dir override flag or env var? (Affects whether T3 uses isolated YAML files or mutate-and-restore.)
- Does `--clean` already invalidate cached state across all four mocks, or do we need an explicit `infrafactory mock reset` between entries? (Likely needed; the tool should call it.)
