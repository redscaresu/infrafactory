# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## Read first

**🎯 Baseline: 39/39 deterministic, sustain-validated across two arcs.** S95 (3 sweeps): 39/39 + 39/39 + 32/39 (transport tail). S100 (3 sweeps): 39/39 + 38/39 + 39/39 (the 38/39 was a tofu init 502 — provider-registry transport, not a regression). S101 in-loop retry now recovers both Claude CLI rate-limits AND OpenTofu provider-registry blips.

**First organic N13 emission preserved**: `pitfalls/aws.yaml` now carries `aws_subnet` "do NOT use `map_public_ip_on_launch` — observed in aws-eks". S94's selective discard worked as designed.

**Scaffold shape (Option C)** — goal-named, variable-length arcs.

## Read this first (handoff state as of 2026-06-06 end-of-session)

### What just landed — fakegenesys arc S108-S115

The 4th-cloud arc (`docs/plans/fakegenesys-arc-plan.md`) is **structurally complete** — fakegenesys repo built + integrated into infrafactory + 4 sibling crosslinks shipped. **One outstanding manual step**: the actual sustain sweep validation (S115-T1 through T3) needs to be run by an operator with LLM credentials. Detailed below.

- **fakegenesys repo**: 8 PRs merged (S108 scaffold + S109 identity + S110 routing + S111 architect/IDP + S112+S113 codex review × 2 = 14 substantive bugs fixed). 15 resources, OSS-mature day-one, port `:8083`. github.com/redscaresu/fakegenesys.
- **infrafactory#92 (S114)**: Genesys wired as the 4th cloud across every dispatch point + scenario.schema.json + prompts/genesys/* + policies/genesys/* + pitfalls/genesys.yaml (empty for cold-start auto-learning test) + 5 training scenarios + Makefile + README + AGENTS + ADR-0020 + STATUS update.
- **4-PR cross-link sweep**: mockway#9 + fakegcp#15 + fakeaws#13 add fakegenesys to their sibling lists. fakegenesys README already cross-links the others (landed in S108).
- **ARCHIVE**: per-arc close-out entry under "2026-06-06 fakegenesys arc S108-S115 close-out" (this commit).

### What needs operator action FIRST in next session

1. **Sustain sweep (S115-T1/T2/T3)**: run `make sweep-N` 3× with LLM credentials. The expected shape:
   - Win condition: 44/44 deterministic × 3 (5 genesys + 39 existing).
   - **Cold-start auto-learning observation**: sweep 1 may legitimately fail on some genesys scenarios (pitfalls/genesys.yaml ships empty). Sweep 2 / sweep 3 should show learned-pitfall-driven improvement if the loop closes from cold start.
   - Track per-sweep: `pitfalls/genesys.yaml` line count delta + AVOID_EMISSIONS per-cloud + any `IsMockServerBug`-classified failures → `docs/mock-gaps.md` (fix at source in fakegenesys, mirror S106 KMS soft-delete pattern).
2. **Tag fakegenesys v0.1.0** once sustain is clean: `cd ../fakegenesys && git tag v0.1.0 && git push origin v0.1.0`. The release workflow auto-builds linux/darwin × amd64/arm64.
3. **User click-ops**:
   - Flip `github.com/redscaresu/fakegenesys` to public (currently private).
   - Enable branch protection on `main` matching the three other siblings' rules.

### What's in flight (poll + merge when CI green)

- **infrafactory#92** (S114 dispatch integration + S115 close-out folded in)
- **mockway#9** / **fakegcp#15** / **fakeaws#13** (sibling crosslinks, pure-docs)

All four should be green ~1-2 min after push. Merge with `gh pr merge <N> --repo redscaresu/<repo> --squash --admin --delete-branch`.

### Outstanding directive (carried from 2026-06-05)
User asked: **"update, optimise agents.md, readmes.md across all the repos"** once the fidelity sweep lands. The fakegenesys arc carried sibling-list updates; broader optimization pass deferred. Could be a single-slice docs arc post-sustain.

## Next planned arc (queued, not started)

**`docs/plans/fakegenesys-arc-plan.md`** — fakegenesys (Genesys Cloud CCaaS mock) + infrafactory integration. **8 slices (S108-S115)**, ~5-7 days focused effort.

- **Repo already exists** at `../fakegenesys/` (`https://github.com/redscaresu/fakegenesys`, private). User created with initial commit + LICENSE on 2026-06-05. S108-T1 ("Create `../fakegenesys/` repo") is therefore effectively done — first commits should land directly there.
- Port `:8083`, Apache-2.0, mirror fakeaws OSS-mature layout
- Mature scope: 15 balanced resources (5 identity + 5 routing + 5 architect, including `flow` with multipart upload + lock/publish state machine)
- **Spec-driven fidelity** (mirrors mockway): Genesys publishes OpenAPI; downloaded into `specs/genesys-openapi.json`
- 5 training scenarios; sustain win condition is 44/44 (39 existing + 5 new) deterministic × 3 sweeps
- Bundled infrafactory integration (S114): prompts/policies/scenarios/dispatch/topology
- Also the **strongest end-to-end test of the auto-learning loop** the project has run — `pitfalls/genesys.yaml` starts empty; loop bootstraps a new cloud from cold start
- Anti-nitpick rule documented for the codex review slices (S112/S113)
- Full autonomous-execution loop prompt at the bottom of the plan doc

## Other open / shelved items

- **Layer 3 real-cloud validation** — open since S93. Big arc (cloud credentials, money, cleanup discipline). High value, high coordination cost.
- **fakeaws `/mock/reset` purges KMS keys** — known pre-existing limitation noted in S106 close-out. ~20-30min single-slice if it causes a sweep flake.
- **`docs/plans/pitfall-pruning-automation-plan.md`** (S107, **SHELVED**) — automation for self-pruning stale `pitfalls/*.yaml` entries. Shelved 2026-06-05 because the file isn't currently a problem (22 entries total). Shelf note includes N-trial-replay design recommendation for if reactivated. S107 slice ID stays reserved (don't reuse).

## Standing preferences (this user)

Captured as memory entries — also worth knowing inline:

- **Don't let codex nitpick.** Triage every codex finding into substantive vs style; act on substantive only. Document declined nitpicks with rationale in `docs/review-passes/passN.md`. Stop iterating after 2 consecutive no-substantive-findings passes.
- **Sustain sweeps cover ALL scenarios.** Never run a reduced sweep "just for the new stuff" — the dispatch wiring changes that go with new clouds/features touch code paths existing scenarios traverse; a reduced sweep would miss regressions.
- **Mature OSS scope from day one** for new sibling fakes. Apache-2.0 + SECURITY + CONTRIBUTING + CODE_OF_CONDUCT + CHANGELOG + release workflow + gitleaks pre-commit + branch protection. Mirror fakeaws layout exactly.
- **Cost-sensitive on CI.** Declined nightly sweep proposal because of LLM API cost. Don't pitch it again unless something material changes.
- **`/loop` autonomous execution is the default for big arcs.** Plan docs end with the verbatim loop prompt; user kicks it off and walks away.

## Reference patterns (used this session, worth knowing)

- **4-PR cross-repo sweep**: for cross-cutting docs changes (smoke harness, fidelity strategy), open one PR per repo (3 siblings + infra) with consistent commit messages cross-referencing each other. Merge in any order once all green.
- **Single-slice arc with close-out folded** is the right shape when the work is genuinely one cohesive unit (e.g. S106 KMS soft-delete; S105 sustain). 2-4 slices when the work splits naturally (e.g. S102/103/104 mock-gaps-and-rename).
- **Slice IDs are sequential and shelved IDs stay reserved.** S107 = reserved-shelved (pitfall pruning). Next active = S108.

## Sweep entry point

`make sweep-N` (was `make sweep-39`; renames at S114-T8 to discover all scenarios under `scenarios/training/`). Output: `/tmp/sweep-*/summary.tsv` + `panics.log` + per-scenario logs. Summary lines:
- `PASS=X / TOTAL=Y (deterministic: X/Z; transport_failed: W)`
- `PANIC_LINES=N`
- `AVOID_EMISSIONS=N` (per-cloud breakdown in pitfall-merge output)
- `RETRY_TRANSPORT=N` / `RETRY_RECOVERED=M`
- `TRANSPORT_FAILED=N`

## Sweep entry point

`make sweep-39`. Output: `/tmp/sweep-39/summary.tsv` + `panics.log` + per-scenario logs. New summary lines from this arc:
- `PASS=X / TOTAL=Y (deterministic: X/Z; transport_failed: W)` (S97)
- `PANIC_LINES=N` (S87)
- `N13_EMISSIONS=N` (S94)
- `RETRY_TRANSPORT=N` (S101 attempted retries)
- `RETRY_RECOVERED=M` (S101 succeeded on retry)
- `TRANSPORT_FAILED=N` (S97 end-of-sweep classification, post-retry)

## Recent arcs (full close-outs in `docs/status/ARCHIVE.md`)

- **sustain re-validation + transport retry** (2026-06-04): 2 PRs. First organic N13 entry; transport-retry shipped.
- **post-sustain tightening** (2026-06-03): 4 PRs + 1 fakeaws. aws-route53 + classifier + rule #13 + prompts ratchet.
- **sustain + N13 durability** (2026-06-03): 2 PRs. First Option C arc.
- **S89–S93** (2026-06-03): 🎯 39/39 first deterministic. 3 PRs.
- **S84–S88** (2026-06-03): gcp-full-stack convergence + panic gate. 3 PRs.
- **S79–S83** (2026-06-02): sibling-mock drainage + carve-out validation. 4 PRs.
- **S74–S78** (2026-06-02): AWS/Scaleway phase3 collapse + `make sweep-39` + N3 carve-out. 5 PRs.
- **S54–S73**: GCP phase2 collapse, sustain ratchets, N3/N10/N13 architecture build-out. ~22 PRs.
