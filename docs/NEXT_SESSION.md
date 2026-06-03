# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## Read first

**🎯 Baseline: 39/39 deterministic, sustain-validated.** Three consecutive sweeps on 2026-06-03 returned 39/39 + 39/39 + 32/39. The third sweep's 7 failures decompose into (a) 6 pre-iter-1 LLM transport failures (Claude CLI rate-limit, 5-9s durations) and (b) 1 genuine LLM convergence flake (`aws-route53`, `repair_budget_exhausted` after 5 iters). Modulo transport, the baseline is robust.

**Scaffold shape (Option C)** — goal-named, variable-length arcs. Codified in `AGENTS.md` § "Planning a New Arc".

## Last arc complete

`docs/plans/sustain-and-n13-durability-plan.md` — first goal-named arc under Option C. Full close-out: `docs/status/ARCHIVE.md` § "2026-06-03 sustain + N13 durability".

- ✅ **S94** (PR #75): N13 durability. `cmd/pitfall-merge/` selectively preserves `learned_from_diff_avoid` entries through sweep teardown; other sources still discarded. Schema-enum ratchet + sweep-side `N13_EMISSIONS=N` watchdog.
- ✅ **S95**: 3 sustain sweeps (39/39, 39/39, 32/39 with transport tail) + arc close-out folded in.

## Suggested next arc

Two natural follow-ups (you pick):

- **aws-route53 flake investigation** — single scenario, single PR. The flake reproduces (sweep 3 hit it after sweeps 1 + 2 didn't). Read `.infrafactory/runs/aws-route53/<sweep-3-failed-run>/iterations/{1..5}/generated/*.tf` and identify what oscillates. Likely outcome: a pitfall (Category C — load-bearing rule) or a fakeaws Route 53 handler fix. ~1-2 hr.

- **LLM-transport robustness** — the sweep-3 tail showed 6 scenarios failing at `iteration_1_generate` with 5-9s durations. That's the Claude CLI hitting some limit and failing fast. Make the sweep harness more resilient: detect the transport-failure shape vs convergence-failure shape, retry the transport class once, distinguish in `summary.tsv`. ~2-3 hr.

The aws-route53 flake is the smaller-scope option with a clearer payoff (39/39 sustained). LLM-transport robustness is broader infra-quality work.

## Sweep entry point

`make sweep-39`. Output: `/tmp/sweep-39/summary.tsv` + `panics.log` + per-scenario logs + `N13_EMISSIONS=N` line.

## Recent arcs (full close-outs in `docs/status/ARCHIVE.md`)

- **sustain + N13 durability** (2026-06-03): first Option C arc. 2 PRs (#75 S94 durability + this S95 close-out).
- **S89–S93** (2026-06-03): fakeaws Secrets Manager soft-delete + AWS phase2 audit + 🎯 39/39 first deterministic. 3 PRs.
- **S84–S88** (2026-06-03): gcp-full-stack convergence + panic gate. 3 PRs.
- **S79–S83** (2026-06-02): sibling-mock drainage + carve-out validation. 4 PRs.
- **S74–S78** (2026-06-02): AWS/Scaleway phase3 collapse + `make sweep-39` + N3 carve-out. 5 PRs.
- **S54–S73**: GCP phase2 collapse, sustain ratchets, N3/N10/N13 architecture build-out. ~22 PRs.
