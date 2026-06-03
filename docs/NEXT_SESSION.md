# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## Read first

**🎯 Baseline: 39/39 deterministic.** First fully-deterministic sweep landed S90. Full close-out: `docs/status/ARCHIVE.md` § "2026-06-03 S89–S93".

**Scaffold shape (Option C)** adopted via PR #71 — arcs are goal-named, variable-length (typically 2–4 slices), mandatory close-out. See `AGENTS.md` § "Planning a New Arc". The historical S93 analysis with three alternative shapes lives in `docs/status/ARCHIVE.md` § "2026-06-03 S89–S93" → "Scaffold question writeup".

## Next arc planned

`docs/plans/sustain-and-n13-durability-plan.md` — first goal-named arc under Option C. Two slices, ~3.5–5 hr:

- **S94 — N13 durability.** Modify `scripts/sweep_39.sh` so `learned_from_diff_avoid` entries survive sweep teardown (currently every pitfall addition gets discarded). N13 only fires on confirmed deletion-as-fix, so its output is grounded in a successful run — should be durable. Add a static schema ratchet + sweep-side watchdog that counts N13 emissions per sweep.
- **S95 — 3 consecutive sustain sweeps.** Validates the 39/39 baseline is stable (not a single-sweep fluke) AND observes N13 emergence under the new protocol. Arc close-out folded in per Option C.

Autonomous-execution loop prompt at the bottom of the plan file.

## Sweep entry point

`make sweep-39`. Output in `/tmp/sweep-39/summary.tsv` + `panics.log` + per-scenario logs. Current baseline: **39/39 target_reached, 0 panics**.

## Recent arcs (full close-outs in `docs/status/ARCHIVE.md`)

- **S89–S93** (2026-06-03): fakeaws Secrets Manager soft-delete + AWS phase2 audit + 🎯 39/39. 3 PRs.
- **S84–S88** (2026-06-03): gcp-full-stack convergence + panic gate. 3 PRs.
- **S79–S83** (2026-06-02): sibling-mock drainage + carve-out validation. 4 PRs.
- **S74–S78** (2026-06-02): AWS/Scaleway phase3 collapse + `make sweep-39` + N3 carve-out. 5 PRs.
- **S54–S73**: GCP phase2 collapse, sustain ratchets, N3/N10/N13 architecture build-out. ~22 PRs.
