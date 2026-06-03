# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## Read first

**🎯 Baseline: 39/39 deterministic, sustain-validated.** Three consecutive sweeps on 2026-06-03 returned 39/39 + 39/39 + 32/39. The third sweep's 7 failures decompose into (a) 6 pre-iter-1 LLM transport failures (Claude CLI rate-limit, 5-9s durations) and (b) 1 genuine LLM convergence flake (`aws-route53`, `repair_budget_exhausted` after 5 iters). Modulo transport, the baseline is robust.

**Scaffold shape (Option C)** — goal-named, variable-length arcs. Codified in `AGENTS.md` § "Planning a New Arc".

## Last arc complete

`docs/plans/sustain-and-n13-durability-plan.md` — first goal-named arc under Option C. Full close-out: `docs/status/ARCHIVE.md` § "2026-06-03 sustain + N13 durability".

- ✅ **S94** (PR #75): N13 durability. `cmd/pitfall-merge/` selectively preserves `learned_from_diff_avoid` entries through sweep teardown; other sources still discarded. Schema-enum ratchet + sweep-side `N13_EMISSIONS=N` watchdog.
- ✅ **S95**: 3 sustain sweeps (39/39, 39/39, 32/39 with transport tail) + arc close-out folded in.

## Next arc planned

`docs/plans/post-sustain-tightening-plan.md` — second goal-named arc under Option C. Four slices, ~3.5–5.5 hr:

- **S96**: aws-route53 flake fix. Investigation-first — either pitfall or fakeaws handler depending on what the iter HCL reveals.
- **S97**: Transport-failure classification in `sweep_39.sh`. Detect "iter_1_generate fail in <30s" and report as `transport_failed` distinct from `repair_budget_exhausted`. Doesn't retry yet — just classifies so flake-budget characterization gets sharper.
- **S98**: Retire GCP phase3 self-review rule #13 (Category B per ADR-0018, identified mid-arc-89-93 but never executed). Audit AWS + Scaleway phase3 for the same shape.
- **S99**: Extend `TestPitfallsNoOPADuplication` to scan `prompts/*.md` — closes the gap that let rule #13 slip past S82. Arc close-out folded in.

Autonomous-execution loop prompt at the bottom of the plan file.

## Sweep entry point

`make sweep-39`. Output: `/tmp/sweep-39/summary.tsv` + `panics.log` + per-scenario logs + `N13_EMISSIONS=N` line.

## Recent arcs (full close-outs in `docs/status/ARCHIVE.md`)

- **sustain + N13 durability** (2026-06-03): first Option C arc. 2 PRs (#75 S94 durability + this S95 close-out).
- **S89–S93** (2026-06-03): fakeaws Secrets Manager soft-delete + AWS phase2 audit + 🎯 39/39 first deterministic. 3 PRs.
- **S84–S88** (2026-06-03): gcp-full-stack convergence + panic gate. 3 PRs.
- **S79–S83** (2026-06-02): sibling-mock drainage + carve-out validation. 4 PRs.
- **S74–S78** (2026-06-02): AWS/Scaleway phase3 collapse + `make sweep-39` + N3 carve-out. 5 PRs.
- **S54–S73**: GCP phase2 collapse, sustain ratchets, N3/N10/N13 architecture build-out. ~22 PRs.
