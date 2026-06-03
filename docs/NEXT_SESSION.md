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

## Suggested next arc

The natural next move is a **sustain re-validation sweep** — three more `make sweep-39` runs to confirm the S96 + S97 + S98 fixes hold across multiple invocations. The classifier should split flakes cleanly into deterministic / transport. Likely a 1-2 slice arc:

- **Slice 1**: Three consecutive sweeps under the new protocol (S94 N13 durability + S97 transport classifier + S96 route53 fix). Document pass-count stability + transport-flake rate.
- **Slice 2** (optional): arc close-out + STATUS/NEXT_SESSION/ARCHIVE update.

Alternative bigger-scope arcs (if you want to push into new territory):

- **LLM-transport retry** — now that S97 classifies transport failures, the next step is to RETRY them once before recording as failed. ~2-3 hr. Closes the transport noise.
- **Layer 3 real-cloud validation** — still on the open-followups list from S93. Big arc; needs real cloud credentials and cleanup discipline.

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
