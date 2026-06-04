# Arc: mock-gaps drain + learning-system rename

Status: planned (2026-06-04)
Owner: next-session claude (designed for autonomous execution)
Follows: `sustain-revalidate-and-transport-retry-plan.md` (closed 2026-06-03 with S100 + S101)
Shape: goal-named variable-length arc per AGENTS.md "Planning a New Arc" (3 slices, ~5–7 hr)

## Big picture

`docs/mock-gaps.md` has accumulated 13 historical entries (9 GCP, 2 Scaleway, 6 AWS) from sweeps on 2026-06-01 / 02. The 2026-06-04 probe sweep produced PASS=38/39 with zero new mock-gaps emissions — confirming the file is now a **hangover queue**, not a live signal. Several entries are already known fixed (aws_subnet `MapPublicIpOnLaunch` via S100's N13 pitfall, aws_route53_record `empty result` via S96, aws_kms_key rotation via S77). Several GCP entries (`access_token_type_unsupported`, `plugin did not respond`) were already verified non-reproducible during S86 but never pruned.

The 2026-06-04 sweep also surfaced one fresh failure (`web-app-paris` → `scaleway_domain_record: resource with ID is not found`) that N3 classified as LLM-actionable (`source: learned`). The error shape (empty ID) is suspicious — could be a mockway DNS handler bug that *should* have routed to mock-gaps.

Three complementary slices:

1. **S102 — diagnose web-app-paris.** Reproduce, trace the empty-ID failure to source (mockway DNS handler OR LLM-side missing zone), fix in correct repo. If failure shape *should* have been mock-actionable, widen `IsMockActionable` so future sweeps catch it.

2. **S103 — replay + prune the 13 historical entries.** For each, re-run `discovered_from`, drop stale entries, ship sibling-mock fixes for confirmed-real ones, commit the pruned file.

3. **S104 — rename learning system (N3/N10/N13/M97 → Fix/Avoid) + fold arc close-out.** Per `docs/tickets/rename-learning-system.md` (decisions locked 2026-06-04). Atomic single-PR migration: code identifiers + binary (`cmd/n10extract` → `cmd/extract-pitfall`) + YAML `source:` enum (`learned*` → `descriptive`/`fix`/`avoid`) + docs + 4 READMEs + memory pointers. Folds ARCHIVE + NEXT_SESSION close-out per Option C.

Order: S102 first because it's cheap (~30min) and the classifier-widening insight (if any) reshapes how we treat S103's "verified non-reproducible" decisions. S103 second so the prune lands under existing names (smaller diff). S104 last so the rename is one clean refactor over a settled codebase — no entanglement with semantic changes from S102's potential classifier work or S103's YAML deletions.

## Slices

| Slice | Title | Effort |
|---|---|---|
| S102 | Diagnose web-app-paris empty-ID + (maybe) widen `IsMockActionable` | ~1-1.5 hr |
| S103 | Replay + prune 13 historical mock-gaps entries | ~2-2.5 hr |
| S104 | Rename N3/N10/N13/M97 → Fix/Avoid; fold arc close-out | ~2-3 hr |

**Total**: ~5–7 hr.

## Standing rules

Inherit all rules from prior arcs (slices-54-62 through `sustain-revalidate-and-transport-retry-plan.md`). Specifically:

- **Selective pitfall discard (S94)**: `learned_from_diff_avoid` survives sweep teardown via `bin/pitfall-merge`; `learned` + `learned_from_diff` discarded as before.
- **Transport retry (S101)**: `sweep_39.sh` retries `transport_failed` shape once before recording.
- **Fix at source (`feedback_sweep_protocol.md`)**: mock bugs go to fakeaws/fakegcp/mockway, never hand-edit `pitfalls/*.yaml`.
- **Mandatory close-out per Option C**: folded into S103.

## S102 — diagnose web-app-paris empty-ID failure

### Motivation

The 2026-06-04 probe sweep produced this failure:

```
scaleway-sdk-go: resource  with ID  is not found
  with scaleway_domain_record.app, on dns.tf line 1
```

Notice the **double space** — both the resource type word and the ID word are empty. That's not a normal "resource not found" — it's a malformed error message. Two hypotheses:

| Hypothesis | Owner | Test |
|---|---|---|
| Mockway DNS handler returns 404 with empty body when the record's parent zone doesn't exist, and scaleway-sdk-go formats the empty fields into the message | mockway | curl the DNS list endpoint with bogus zone; inspect response shape |
| LLM-generated HCL has `scaleway_domain_record` without a `dns_zone` (or with an empty interpolation), so the SDK call goes out with empty resource type + ID | LLM-side / scenario | inspect the generated `.tf` artifacts from the failed run |

N3 classified this as LLM-actionable (got `source: learned` in scaleway.pitfalls.diff), so right now `IsMockActionable` doesn't catch this signal.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S102-T1 | Read `/tmp/sweep-mockgaps-probe/web-app-paris.log` + the generated `.tf` from that run. Establish: did the LLM write a malformed `scaleway_domain_record`, or did the SDK get a malformed mockway response? | P0 | — |
| S102-T2 | If LLM-side: confirm `source: learned` pitfall covers the failure mode; verify next sweep would self-correct via descriptive pitfall. Document outcome; no code change needed. | P0 | T1 |
| S102-T3 | If mockway-side: write a failing handler test in `../mockway/` against the malformed-response path; fix the handler; ship as PR to mockway. Bump infrafactory's mockway dependency in `go.mod` if needed. | P0 | T1 |
| S102-T4 | If both (LLM created a degenerate config that should have been a 400 from mockway, not a 404 with empty body): fix mockway to return 400; consider whether the descriptive pitfall is sufficient for the LLM-side. | P0 | T1 |
| S102-T5 | If the failure shape *should* have routed to mock-gaps (mockway-side bug masquerading as `resource not found`): widen `IsMockActionable` signal list. Add test case in `internal/generator/pitfalls_learn_test.go`. | P1 | T3 or T4 |
| S102-T6 | One PR per repo touched (infrafactory + maybe mockway). | P0 | T1–T5 |

### Exit criteria

- web-app-paris failure root-cause identified (LLM vs mockway).
- Fix shipped in correct repo (or documented as "no code change, descriptive pitfall covers it").
- If classifier was wrong, `IsMockActionable` widened + test.
- Sweep replay of web-app-paris passes (or, if blocked by Claude rate limit, doesn't reproduce the same shape).

## S103 — replay + prune 13 historical mock-gaps entries; fold close-out

### Motivation

`docs/mock-gaps.md` has these entries to verify:

**GCP (9 entries, expected mostly stale per S86):**
- google_project_service: access_token_type_unsupported (gcp-cloud-run)
- google_service_networking_connection: access_token_type_unsupported (gcp-cloud-sql)
- google_kms_crypto_key_iam_member: plugin did not respond (gcp-gke-cluster)
- google_container_node_pool: plugin did not respond (gcp-gke-cluster)
- _(none)_: 501 not implemented (gcp-storage)
- google_compute_instance: plugin did not respond (gcp-full-stack)
- google_sql_database: 501 not implemented (gcp-cloud-sql)
- google_service_account: access_token_type_unsupported (gcp-full-stack)
- google_sql_database_instance: plugin did not respond (gcp-full-stack)
- google_project_iam_member: access_token_type_unsupported (gcp-full-stack)

**Scaleway (2 entries):**
- _(none)_: 501 not implemented (compute-lb-multi-paris)
- _(none)_: plugin did not respond (compute-lb-multi-paris)

**AWS (6 entries, expected mostly already-fixed):**
- aws_iam_policy: managed-policy ARN leak (aws-full-stack)
- aws_subnet: MapPublicIpOnLaunch persistence (aws-full-stack) — **already addressed by S100 N13 pitfall**
- aws_kms_key: rotation waiting (aws-full-stack) — **already addressed by S77**
- aws_route53_record: empty result (aws-route53) — **already addressed by S96**
- aws_subnet: MapPublicIpOnLaunch (aws-vpc-network) — same as above
- _(none)_: resourcenotfoundexception KMS delete (aws-full-stack)

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S103-T1 | For each AWS entry: run the `discovered_from` scenario once. If it passes, mark entry as stale. If it fails with same signal, the existing pitfall didn't cover it — investigate. | P0 | — |
| S103-T2 | For each Scaleway entry: same protocol. The two compute-lb-multi-paris entries are unverified — could be live gaps. | P0 | — |
| S103-T3 | For each GCP entry: same protocol. Most expected stale per S86, but verify rather than assume. | P0 | — |
| S103-T4 | Ship sibling-mock fixes for any confirmed-real entries. PRs against fakeaws/fakegcp/mockway as appropriate. | P0 | T1, T2, T3 |
| S103-T5 | Rewrite `docs/mock-gaps.md` to reflect post-prune state. Either (a) leave it as runtime artifact and just delete the stale rows so the next sweep starts from a clean slate, or (b) commit it as a tracked file with a "verified non-reproducible 2026-06-04" annotation block. Pick (a) if there are 0 confirmed-real entries left; pick (b) if 1+ entries survive pruning. | P0 | T4 |
| S103-T6 | Update AGENTS.md sweep-protocol bullet: add "mock-gaps drainage" as a periodic-maintenance task. Cadence: after each major sweep (every ~4 arcs). | P1 | T5 |
| S103-T7 | One PR. (Arc close-out **moved to S104** since S104 is now the closing slice.) | P0 | T1–T6 |

### Exit criteria

- Every entry in `docs/mock-gaps.md` either verified stale (deleted) or fixed at source (sibling-mock PR landed).
- Pruned file reflects reality.
- AGENTS.md mentions periodic drainage cadence.

## S104 — rename learning system + fold arc close-out

### Motivation

Decisions locked 2026-06-04 in `docs/tickets/rename-learning-system.md`. The internal slice IDs (N3, N10, N13, M97) leak into user-facing surfaces — function names, YAML `source:` enum values, all four repos' READMEs, AGENTS.md. As infrafactory moves toward OSS visibility (Apache-2.0 + pending public flip), this is an onboarding tax. S104 fixes it in one atomic refactor.

Sequencing rationale: S102's potential `IsMockActionable` widening and S103's `pitfalls/*.yaml` deletions both land under existing names. S104 is a pure refactor — no semantic changes entangled with the rename, so the diff is auditable line-by-line.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S104-T1 | Rename code identifiers via `gopls rename` (preferred for correctness) or `grep -rl … \| xargs sed`. `IsMockActionable` → `IsMockServerBug`; `ExtractPrescriptiveFix` → `ExtractFixPitfall`; `ExtractPrescriptiveAvoid` → `ExtractAvoidPitfall`; `ExtractLearnedPitfall` → `ExtractDescriptivePitfall`. Run `go test ./...`. | P0 | S103 complete |
| S104-T2 | Migrate `pitfalls/*.yaml` `source:` values: `learned`→`descriptive`, `learned_from_diff`→`fix`, `learned_from_diff_avoid`→`avoid`. Update loader enum constants + `TestPitfallsSourceEnum`. | P0 | T1 |
| S104-T3 | Rename `cmd/n10extract` → `cmd/extract-pitfall` (use `git mv` so history tracks the rename). Update Makefile + `make build` output + scripts/sweep_39.sh references. | P0 | T1 |
| S104-T4 | Update `bin/pitfall-merge` selective-discard: keep `avoid`, discard `fix` + `descriptive`. Update `cmd/pitfall-merge/main_test.go`. | P0 | T2 |
| S104-T5 | Update `docs/auto-learning-loop.md` + AGENTS.md + 4 READMEs (infrafactory, mockway, fakegcp, fakeaws) + active memory pointers. Add a one-paragraph "renamed 2026-XX-XX from N3/N10/N13/M97" footnote in auto-learning-loop.md for searchability. | P0 | T1-T4 |
| S104-T6 | Update `TestPitfallsNoMockActionableSeeds` → `TestPitfallsNoMockServerBugSeeds`. Leave other CI ratchet names unchanged. | P0 | T1 |
| S104-T7 | `make sweep-39` to confirm: classifier still routes, organic emissions use new `source:` values, `bin/extract-pitfall` works under `--mode fix/avoid`. | P0 | T1-T6 |
| S104-T8 | One PR per repo touched (infrafactory + mockway + fakegcp + fakeaws). **Arc close-out folded in** (STATUS + NEXT_SESSION + ARCHIVE per Option C). | P0 | T1-T7 |

### Exit criteria

- `git grep -E 'IsMockActionable|ExtractPrescriptive|ExtractLearnedPitfall|learned_from_diff'` returns zero hits in live code (ARCHIVE / memory historical mentions allowed).
- `git grep -E '\bN3\b|\bN10\b|\bN13\b|\bM97\b'` returns zero hits in `docs/auto-learning-loop.md`, AGENTS.md, READMEs.
- All four repos' READMEs use Fix/Avoid vocabulary.
- `make sweep-39` passes; pitfalls reload from migrated YAML; CI ratchets green.
- `bin/extract-pitfall` builds; `cmd/n10extract` directory is gone.
- ARCHIVE close-out for the arc lands.

## Why this order, in one paragraph

S102 first because (a) it's cheap — 30min diagnosis — and concrete (one fresh failure with a specific error shape to chase); (b) if the diagnosis surfaces a classifier-widening insight, that changes how S103 treats the "verified non-reproducible" decisions on stale entries (e.g., if `IsMockActionable` was missing a signal, some "non-reproducible" GCP entries might actually be reproducible-but-misrouted). S103 second because pruning 13 entries goes faster once you know the classifier you're trusting, and the deletions land on existing source-enum values (smaller diff than reshaping AND renaming at once). S104 last because the rename is a pure-refactor — when it runs over a settled codebase (S102 + S103 already merged), the diff is auditable line-by-line with no entangled semantic changes.

## Autonomous-execution loop prompt

```
/loop until all three slices (S102, S103, S104) in docs/plans/mock-gaps-and-rename-plan.md are complete: every slice's exit criteria met, every PR merged with green CI, STATUS + NEXT_SESSION updated.

Read docs/NEXT_SESSION.md first for the prior arc's handoff, then docs/plans/mock-gaps-and-rename-plan.md for the slice definitions, then docs/tickets/rename-learning-system.md for the S104 rename specifics (decisions are locked — DO NOT re-decide). All prior standing rules apply (slices-54-62 through sustain-revalidate-and-transport-retry).

Standing rule that changes mid-arc: S94 selective-discard renames as part of S104. Before S104 lands, learned_from_diff_avoid survives; learned + learned_from_diff discarded. After S104 lands, `avoid` survives; `fix` + `descriptive` discarded.

Work slices in order S102 → S103 → S104. S104 folds the mandatory ARCHIVE + NEXT_SESSION close-out per the Option C arc shape — no separate close-out slice.

Authority: open + merge PRs with `gh pr merge <N> --squash --admin --delete-branch` once CI is green, in all four repos.

S102 exit decision matters: if web-app-paris is a mockway-side bug that should have been mock-actionable, widening IsMockActionable is mandatory before S103 (so the replay sweeps in S103 use the new classifier). If it's a clean LLM-side fix, proceed to S103 directly.

S104 is a pure refactor — DO NOT introduce semantic changes alongside the rename. If a semantic issue surfaces during the rename, file a follow-up ticket and address it after S104 lands.

Stop only when: (a) all three slices complete OR (b) you genuinely cannot proceed (document the blocker in NEXT_SESSION + stop).

Before stopping for any reason, update docs/NEXT_SESSION.md with: which slices closed, which are blocked, what next session should do FIRST.
```

## Fresh-context checklist

The autonomous prompt assumes the agent reads, in order:
1. `AGENTS.md` (Option C goal-named arcs; sweep-protocol bullet)
2. `docs/NEXT_SESSION.md`
3. This file (`docs/plans/mock-gaps-and-rename-plan.md`)
4. `docs/tickets/rename-learning-system.md` (locked-in S104 specifics)
5. `STATUS.md`
6. `docs/status/ARCHIVE.md` § "2026-06-03 sustain re-validation + transport retry" (the prior arc context)
7. `docs/mock-gaps.md` (the 13 entries to verify)
8. `internal/generator/pitfalls_learn.go::IsMockActionable` (the classifier S102 may widen)
9. `/tmp/sweep-mockgaps-probe/web-app-paris.log` + generated `.tf` (the live S102 diagnosis input)
10. `docs/auto-learning-loop.md` (the doc S104 rewrites with new vocabulary)
