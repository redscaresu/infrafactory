# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## S89–S93 arc complete (2026-06-03) — 🎯 39/39 deterministic

First fully-deterministic 39-scenario sweep of the project. Three PRs landed (fakeaws#6 S89, infrafactory #69 S91+S92, infrafactory S93 close-out). Full close-out: `docs/status/ARCHIVE.md` § "2026-06-03 S89–S93".

- ✅ **S89** (fakeaws#6) — filter PendingDeletion + Destroyed from fakeaws Secrets Manager `/mock/state`. Unblocks `aws-full-stack` orphan_check.
- ✅ **S90** — post-S89 sweep: **39/39 target_reached, 0 panics.** aws-full-stack converged iter 1 / 273s.
- ✅ **S91+S92** (#69) — AWS phase2 audit per ADR-0018: 10/10 Category C, no retirements. The prompt was already lean.
- ✅ **S93** (this PR) — arc close-out + scaffold-question writeup (below).

## Next arc planned

`docs/plans/sustain-and-n13-durability-plan.md` — first goal-named arc under Option C. Two slices, ~3.5–5 hr:

- **S94**: N13 durability — change the sweep protocol so `learned_from_diff_avoid` entries survive (`learned` + `learned_from_diff` still discarded as today). Add a static schema ratchet + sweep-side watchdog that counts N13 emissions per sweep.
- **S95**: Three consecutive `make sweep-39` invocations to validate the 39/39 baseline is stable (not a single-sweep fluke) AND observe whether N13 emits + sticks. Arc close-out folded in per Option C.

Autonomous-execution loop prompt at the bottom of the plan file.

## Scaffold question — RESOLVED (Option C adopted)

The S93 scaffold-question writeup proposed three shapes; the user picked Option C (goal-named, variable-length arcs, mandatory close-out). Codified in `AGENTS.md` § "Planning a New Arc" via PR #71 (2026-06-03). Original analysis preserved below for reference.

## Scaffold question (user decision)

The S88 close-out flagged that the 5-slice scaffold is feeling heavy for arcs where most substantive work is 1-2 fixes + 2-3 documentation slices. S93 promised analysis + alternatives + recommendation; **the decision is yours**.

### Evidence from S84–S88 + S89–S93

| Arc | Substantive slices | Documentation slices | Mock-side / LLM-side |
|---|---|---|---|
| S84–S88 | 2 (S85 pitfall, S87 panic gate) | 3 (S84 investigation, S86 investigation, S88 close-out) | 1 LLM-side + 1 infrastructure |
| S89–S93 | 1 (S89 fakeaws fix) | 4 (S90 sweep, S91 audit, S92 trivial closure, S93 close-out) | 1 mock-side |

Pattern: each arc shipped 1-2 PRs of substantive change + 2-3 PRs of documentation. The five-slice scaffold buys structure but the documentation slices (especially S88 / S93 close-outs) repeat the same "STATUS + NEXT_SESSION + ARCHIVE" work each time. Standalone sweep slices (S90) are an artifact of the template, not a planning constraint.

The scaffold *did* help in S54–S73 when arcs landed 5-9 substantive PRs. As deterministic baseline holds and individual fixes shrink to one PR, the scaffold's overhead-to-content ratio inverts.

### Alternatives (you pick)

#### Option A — Keep the 5-slice scaffold (default, no change)

**Pros**: predictable rhythm. Forces an explicit close-out which catches loose ends. Documentation slices double as memory between sessions (the per-arc ARCHIVE entries are the project's institutional memory now).
**Cons**: friction when an arc is naturally 1-2 PRs. Repeated boilerplate. "Empty" closure slices feel like make-work.
**When it shines**: foundational work (auto-learning loop design, new mock surface) where 5+ meaningful PRs land.

#### Option B — Single-PR arcs

Drop the scaffold entirely. Each fix is a PR. Documentation lives in the PR body + ARCHIVE entry written at merge time. No close-out slice.

**Pros**: zero overhead. Optimal when the deterministic baseline is steady and work is incremental.
**Cons**: loses the cross-PR narrative ("this arc was about X"). Harder to onboard a fresh session because there's no single doc that says "this is what we just did and why" — it's spread across N PR descriptions.
**When it shines**: maintenance phase. Project at steady state, one fix at a time.

#### Option C — Goal-named arcs of variable length (2-4 slices)

Drop the fixed-slice-count, keep the arc narrative. Each arc is named by goal ("39/39 sustain", "fakegcp panic audit") and has whatever slices the goal requires — 2 or 7. Mandatory artifacts: one close-out (STATUS + NEXT_SESSION + ARCHIVE). Investigation + sweep slices are optional, not template-driven.

**Pros**: matches the actual shape of work. Preserves narrative. Removes "padding to 5" pressure.
**Cons**: harder to size-estimate ("how many hours?"). Requires more judgment up front — Option A is more mechanical to plan.
**When it shines**: now. Project is mature enough to vary cadence.

### Agent recommendation

**Option C, with one constraint**: every arc still requires a written close-out under `docs/status/ARCHIVE.md` + a `docs/NEXT_SESSION.md` update at arc end. The close-out is what makes a fresh session bootable — that's load-bearing and shouldn't be optional.

**Tradeoff**: Option C trades the mechanical predictability of Option A for shape-fit. If you'd rather not exercise judgment on "how many slices is the right number" every arc-start, stay with Option A. If the past two arcs felt padded, Option C is the lighter shape.

I haven't written a S94+ plan. The user picks; the agent then drafts a plan that matches the chosen shape.

## Recent arcs (full close-outs in `docs/status/ARCHIVE.md`)

- **S89–S93** (2026-06-03): fakeaws Secrets Manager soft-delete + AWS phase2 audit + 39/39 first deterministic sweep. 3 PRs.
- **S84–S88** (2026-06-03): gcp-full-stack convergence + panic gate. 3 PRs.
- **S79–S83** (2026-06-02): sibling-mock drainage + carve-out validation. 4 PRs.
- **S74–S78** (2026-06-02): AWS/Scaleway phase3 collapse + `make sweep-39` + N3 carve-out. 5 PRs.
- **S54–S73**: GCP phase2 collapse, sustain ratchets, N3/N10/N13 architecture build-out. ~22 PRs across 4 arcs.

## Sweep entry point

`make sweep-39`. Output lands in `/tmp/sweep-39/summary.tsv` + `panics.log` + per-scenario logs. Current baseline: **39/39 target_reached, 0 panics**.
