# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## Read this first (handoff state as of 2026-06-10)

### Baseline

- **44/44 deterministic** (full scope, sweep 8, 2026-06-10).
- **8/8 deterministic** (reduced scope, sweep 11, 2026-06-10) — confirms close-out under the reduced-scope sweep per user 2026-06-10 directive (5 genesys + 3 full-stacks: aws-full-stack, gcp-full-stack, full-stack-paris).
- **fakegenesys v0.1.0 tagged + pushed** (commit `ba2de5a`).

### What landed this session (S116–S122)

- **fakegenesys#11 + #12 (S116/S116b)**: TLS MITM CONNECT-proxy on `:8443` with persisted CA at `~/.fakegenesys/`. CA exposed at `<fakegenesys>/mock/ca-cert`.
- **infrafactory#94 (S117)**: `cloudEnv` plumbs `HTTPS_PROXY` + `SSL_CERT_FILE` + `NO_PROXY` for cloud:genesys.
- **fakegenesys#13 + #14 + #15 (S116c, S119)**: 10+ post-auth read-after-create stubs; `/tokens/me` includes `OAuthClient.organization.id = "purecloud-builtin"` (PascalCase key — SDK has custom UnmarshalJSON).
- **infrafactory#96 (S118)** + **ADR-0021**: cloud-prefix set in 3 auto-learning regex sites; AGENTS.md § "auto-learning is load-bearing"; regression test. Closed S114 oversight where zero genesys pitfalls had ever been auto-learned.
- **infrafactory#97 (S120)**: prompt-level rule for `genesyscloud_flow` + `local_file` pattern.
- **fakegenesys#16–#21 (S122a–g)** + **infrafactory#98–#100 (S122 main, prompts)** + **ADR-0022**: S122 sub-arc unwound 7 successive mock-gap layers as the genesyscloud provider's call chain surfaced new 501s — group subresources, flow upload-job protocol, OAuthClient PascalCase + responsemanagement libraries, terraform-user role chain, authorization subjects/grants, user password POST + bulkadd/bulkremove. ADR-0022 covers harness-level `flow.yaml` pre-placement (provider reads `filepath` at PLAN via CustomizeDiff).

Sweep table: see `docs/status/ARCHIVE.md` § "2026-06-10 fakegenesys S116–S122 sustain validation + v0.1.0".

### What's stable

- All four clouds (Scaleway, GCP, AWS, Genesys) pass full-stack scenarios deterministically.
- Auto-learning pipeline is genesys-aware. `pitfalls/genesys.yaml` grew organically across the sweep arc.
- fakegenesys + TLS MITM end-to-end is solid. `tofu apply` of the 5-resource basic-queue completes <1 min.

## Next arc — queued (planned 2026-06-10)

**`docs/plans/fakegenesys-v0.2-hardening-plan.md`** — fakegenesys v0.2 hardening + cross-repo example-test convergence. 4 slices (S123–S126), ~7–10 hr, ends at `fakegenesys v0.2.0`.

Closes four standalone-quality gaps fakegenesys had vs siblings at v0.1.0:
1. **S123**: table-driven test backfill for S116/S122 surface (target ≥6000 test lines; floor = fakegcp).
2. **S124**: structured codex review-pass loop (close on 2× NOTHING_TO_IMPROVE), mirroring fakeaws's S48 hardening loop. Continues fakegenesys's review-pass numbering from S112/S113.
3. **S125**: add `.github/workflows/docker.yml` (multi-arch, tag-push trigger only — no nightly).
4. **S126**: 4-PR cross-repo convergence on the **fakeaws in-test example pattern**. Extend each sibling's `examples/provider_smoke_test.go` from smoke-pass to per-example `tofu apply` matrix; delete `mockway/scripts/test-examples.sh` + `test-misconfigured.sh` + `test-updates.sh`; delete `fakegcp/scripts/e2e.sh`; normalise the "Testing examples" README section across all 4 siblings; record convergence in infra `AGENTS.md`. Then tag `v0.2.0` + arc close-out.

Same 4-PR shape as the smoke-harness and fidelity-strategy doc sweeps — third instance of `reference_cross_repo_docs_sweep.md`.

### Pre-flight for the next session

- Read the plan doc.
- Read `../fakeaws/examples/provider_smoke_test.go` BEFORE starting S126 — it's the canonical pattern all four siblings will converge on.
- fakegenesys's example tests need `HTTPS_PROXY` + `SSL_CERT_FILE` plumbed when invoking `tofu` (CA is at `<fakegenesys>/mock/ca-cert` at runtime, persisted on disk at `~/.fakegenesys/`). The existing smoke test handles this — extend, don't rewrite.
- mockway's `examples/updates/` is two-phase (apply v1 → apply v2 idempotency). S126-T5 must preserve that semantics.
- Sweep-noise warning: `git status` currently shows `pitfalls/{aws,gcp,scaleway,genesys}.yaml` modified. That's auto-learning sweep state, not real changes. `git stash push pitfalls/` before any close-out commit; don't ever hand-edit (per `feedback_sweep_protocol.md`).

## Other queued work (not blocking this arc)

- **AGENTS.md + README.md optimisation sweep** (carried from 2026-06-05). Could fold into S126 README sweep, but the scope (5 repos × 2 docs) is larger than the S126 README touch-up. Defer to its own arc.
- **Pitfall-pruning automation** (shelved 2026-06-06; S107 slot). `docs/plans/pitfall-pruning-automation-plan.md`.
- **fakegenesys public visibility flip + branch protection** — operator click-ops, not engineering.

## Standing preferences (this user)

- **Don't let codex nitpick.** Act on substantive only. Stop after 2 no-substantive passes.
- **Sustain sweeps cover ALL scenarios** by default. Reduced-scope is a per-loop override only on explicit user directive.
- **Mature OSS scope from day one** for new sibling fakes.
- **Cost-sensitive on CI.** Don't pitch nightly sweeps unprompted.
- **`/loop` autonomous execution is the default for big arcs.**
- **NEVER hand-edit `pitfalls/*.yaml`.** Auto-learning writes them; prompts + code are the legitimate intervention points.
- **`repair_budget_exhausted` is never "expected cold-start"** unless the auto-learning pipeline has emitted at least one pitfall. AGENTS.md § "The auto-learning pipeline is load-bearing" + `feedback_learning_failure_is_a_bug.md` + ADR-0021.
- **Adding a new cloud means updating three regex/switch sites in lockstep**: `internal/generator/pitfalls_learn.go::resourceNameRe`, `internal/generator/prescriptive_extractor.go::addressRe`, `internal/cli/run_command.go::pitfallResourceMatchesCloud`. ADR-0021.

## Sweep entry point

`make sweep-N`. Output: `/tmp/sweep-*/summary.tsv` + `panics.log` + per-scenario logs. Summary lines:
- `PASS=X / TOTAL=Y (deterministic: X/Z; transport_failed: W)`
- `PANIC_LINES=N`
- `AVOID_EMISSIONS=N` (per-cloud breakdown in pitfall-merge output)
- `RETRY_TRANSPORT=N` / `RETRY_RECOVERED=M`
- `TRANSPORT_FAILED=N`

Reduced-scope override: `SCENARIOS_FILE=/path/to/list.txt make sweep-N`.

## Recent arcs (full close-outs in `docs/status/ARCHIVE.md`)

- **S116–S122 (2026-06-10)**: fakegenesys sustain validation + v0.1.0. 11 sweeps; sweep 8 = 44/44 full scope; sweep 11 = 8/8 reduced scope. 7 fakegenesys mock-gap PRs + 5 infrafactory PRs. ADR-0021 + ADR-0022.
- **S108–S115 (2026-06-06)**: fakegenesys arc shipped — 4th cloud structurally integrated. 9 PRs + 3 cross-link PRs.
- **sustain re-validation + transport retry** (2026-06-04): 2 PRs.
- **post-sustain tightening** (2026-06-03): 4 PRs + 1 fakeaws.
- **sustain + N13 durability** (2026-06-03): 2 PRs.
- **S89–S93** (2026-06-03): 🎯 39/39 first deterministic. 3 PRs.
- **S84–S88** (2026-06-03): gcp-full-stack convergence + panic gate. 3 PRs.
- **S79–S83** (2026-06-02): sibling-mock drainage + carve-out. 4 PRs.
- **S74–S78** (2026-06-02): phase3 collapse + `make sweep-39` + N3 carve-out. 5 PRs.
- **S54–S73**: GCP phase2 collapse, sustain ratchets, N3/N10/N13 architecture. ~22 PRs.
