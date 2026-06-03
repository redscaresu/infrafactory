# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## Read first

**🎯 Baseline: 39/39 deterministic, sustain-validated.** Three sustain sweeps on 2026-06-03 returned 39/39 + 39/39 + 32/39. The third sweep's 7 failures decompose into 6 pre-iter-1 LLM transport failures (Claude CLI rate-limit cluster — now correctly classified as `transport_failed` by S97) and one genuine convergence flake (`aws-route53` — fixed by S96).

After this arc's fixes (fakeaws Route 53 sort + tag handler), aws-route53 converges iter 1 cleanly. Next sustain sweep should hit 39/39 deterministic (or 38/39 + transport-classified noise).

**Scaffold shape (Option C)** — goal-named, variable-length arcs. Codified in `AGENTS.md` § "Planning a New Arc".

## Last arc complete

`docs/plans/post-sustain-tightening-plan.md` — second Option C arc. Five PRs. Full close-out: `docs/status/ARCHIVE.md` § "2026-06-03 post-sustain tightening".

- ✅ **S96** (fakeaws#7): fakeaws Route 53 list-sort + `ChangeTagsForResource` handler. aws-route53 fix.
- ✅ **S97** (#78): transport-failure classifier in `sweep_39.sh`.
- ✅ **S98** (#79): retire GCP phase3 self-review rule #13 (Category B — OPA `region_restriction` duplicate).
- ✅ **S99** (this PR): extend OPA-dup ratchet to `prompts/<cloud>/*.md`.

## Next arc planned

`docs/plans/sustain-revalidate-and-transport-retry-plan.md` — third Option C arc. Two slices, ~4–5 hr:

- **S100**: three consecutive `make sweep-39` runs. Validates the post-sustain-tightening behavioural changes (S96 route53 fix, S97 transport classifier, S98 rule #13 retirement, S99 ratchet) hold collectively. Generates live transport-failure data for S101.
- **S101**: LLM-transport retry in `sweep_39.sh`. When the existing S97 classifier detects a `transport_failed` shape mid-sweep, retry the scenario once before writing to `summary.tsv`. Emits `RETRY_TRANSPORT=N` + `RETRY_RECOVERED=M`. Arc close-out folded in per Option C.

Order matters: sustain first so we know whether the recent fixes hold AND so we have real transport-failure data for S101 to validate against.

Autonomous-execution loop prompt at the bottom of the plan file.

## Sweep entry point

`make sweep-39`. Output: `/tmp/sweep-39/summary.tsv` + `panics.log` + per-scenario logs + `N13_EMISSIONS=N` + `TRANSPORT_FAILED=N`. The `PASS=` line now reports deterministic and transport-failed counts separately.

## Recent arcs (full close-outs in `docs/status/ARCHIVE.md`)

- **post-sustain tightening** (2026-06-03): aws-route53 + transport classifier + OPA-dup follow-through. 4 PRs + 1 fakeaws.
- **sustain + N13 durability** (2026-06-03): first Option C arc. 2 PRs.
- **S89–S93** (2026-06-03): 🎯 39/39 first deterministic. 3 PRs.
- **S84–S88** (2026-06-03): gcp-full-stack convergence + panic gate. 3 PRs.
- **S79–S83** (2026-06-02): sibling-mock drainage + carve-out validation. 4 PRs.
- **S74–S78** (2026-06-02): AWS/Scaleway phase3 collapse + `make sweep-39` + N3 carve-out. 5 PRs.
- **S54–S73**: GCP phase2 collapse, sustain ratchets, N3/N10/N13 architecture build-out. ~22 PRs.
