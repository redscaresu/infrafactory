# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## Read this first (handoff state as of 2026-06-10)

### Baseline

- **44/44 deterministic** (full scope, sweep 8, 2026-06-10).
- **fakegenesys v0.2.0 tagged + pushed** (commit `3e5dc55`). v0.1.0 GitHub release published (duplicate Draft cleaned up).
- **27 paired contracts CI-enforced across all 4 sibling fakes** via `handlers/contract_audit_test.go`. Convention is `CRITICAL[<id>]:` docstring ↔ `TestContract_<id>` test.
- **ADR-0021 cloud-prefix lockstep CI-enforced** via `internal/cli/cloud_prefix_lockstep_test.go`. The three sites (`resourceNameRe` / `addressRe` / `pitfallResourceMatchesCloud`) are parsed at test time; disagreement on the cloud-prefix set fails CI with a per-site diff. Same shape as the sibling-fake contract audit.
- Smoke check post-S135: genesys-full-stack target_reached in 329s / 2 iters (no LLM-layer regression).

### "Drift becomes failed `go test`" — the durable enforcement pattern

Three CI-enforced audits now cover wire-shape and pipeline conventions across the project:

| Audit | Where | What it catches |
|---|---|---|
| Sibling contract audit | `handlers/contract_audit_test.go` in mockway, fakegcp, fakeaws, fakegenesys | Missing `TestContract_<id>` for a `CRITICAL[<id>]:` docstring (or vice versa) |
| Cloud-prefix lockstep | `internal/cli/cloud_prefix_lockstep_test.go` | The three auto-learning regex sites disagree on the cloud-prefix set |
| OPA-dup ratchet | `internal/generator/pitfalls_opa_dedup_test.go` + `prompts_opa_dedup_test.go` | Pitfalls or prompts duplicate an OPA-policy citation verbatim |

Pattern: convention as code, not convention as doc. Each is empty-state-safe (zero coverage passes trivially) and self-tested where applicable. The convention is opt-in — adding a `CRITICAL[<id>]:` tag opts a handler in; handlers without one are invisible to the audit.

### Last arcs complete

**Testify-conversion cross-repo sweep (2026-06-11)** — every Go test in `handlers/` across the 4 sibling fakes now uses `github.com/stretchr/testify` where possible. fakegenesys#32 (7 files / ~200 blocks) and fakeaws#20 (15 files / 514 blocks) shipped via parallel agents in one turn each; fakegcp#21 cleaned one straggler; mockway phase was a no-op (its remaining stdlib lives in package-private helpers the sweep rule deliberately doesn't refactor). Rule lives in each repo's AGENTS.md (the 5-PR AGENTS sweep landed earlier today) + the user's global `~/.claude/CLAUDE.md` "Testing (Go)" section.

**Demo-Makefile sibling sweep (2026-06-11)** — 3 PRs mirroring fakegenesys#29's `make demo-*` lifecycle harness into the three sibling fakes. mockway#13, fakegcp#19, fakeaws#17 — eight `demo-*` targets per repo (help/up/down/env/shell/apply/destroy/clean) that drive a real provider through `init → apply → plan -detailed-exitcode → destroy` against the local mock with one command. All four sibling fakes now expose the same demo shape. Simpler than the fakegenesys variant because scaleway/google/aws providers all honour endpoint overrides natively. 8th iteration of the 4-PR cross-repo sweep pattern (see `reference_cross_repo_docs_sweep.md`).

**S136–S138 (2026-06-11)** — fakegenesys example drift fix arc. 3 PRs across fakegenesys. S136 brings the standalone smoke test's environment up to par with infrafactory's `cloudEnv` (HTTPS_PROXY, NO_PROXY, SSL_CERT_FILE, GENESYSCLOUD_OAUTHCLIENT_*, FAKEGENESYS_UPLOAD_HOST). S137 + S138 refresh 16 examples + 1 expected.txt against the current `mypurecloud/genesyscloud` provider schema. fakegenesys v0.2.1 tagged. Smoke runs 100% green in ~205s from a fresh clone. New `feedback_example_hcl_drift.md` memory captures the pattern: example HCL rot is a different layer from the contract audit.

**Post-S135 follow-up (2026-06-10)** — two paper-cut fixes: lockstep audit (`internal/cli/cloud_prefix_lockstep_test.go`) brings ADR-0021 from code-review-enforced to CI-enforced; duplicate v0.1.0 GitHub release Draft cleaned up. One PR (infrafactory#105). ADR-0021 amended with the enforcement note. AGENTS.md § 3 cross-references the test.

**S128–S135 (2026-06-10)** — sibling CRITICAL sweep + AGENTS+README cleanup + smoke. 8 PRs across all 5 repos. Bridged 10 new paired contracts in mockway (2) + fakegcp (4) + fakeaws (4) — combined with fakegenesys's 17 = 27 family-wide. Cross-repo AGENTS/README normalization with new Contract-coverage convention sections in each sibling's docs. Full close-out in `docs/status/ARCHIVE.md` § "2026-06-10 sibling CRITICAL sweep + AGENTS/README cleanup".

**S123–S127 (earlier 2026-06-10)** — fakegenesys v0.2 hardening (full arc detail below).

**`docs/plans/fakegenesys-v0.2-hardening-plan.md`** — fakegenesys v0.2 hardening + cross-repo contract-coverage convention rollout. 5 slices shipped:

- **S123**: 17 `TestContract_*` regression tests for the post-S116/S122 surface. Three coverage bars (regression-per-mock-gap, docstring-derived, nil-deref defenses). Per-row matrix in `fakegenesys/docs/contract-matrix-s123.md`. NO line-count target per `feedback_test_coverage_metrics.md`.
- **S124**: codex review-pass loop closed at pass 4 (`NOTHING_TO_IMPROVE × 2`). `fakegenesys/docs/review-passes/pass3.md` documents both.
- **S125**: `.github/workflows/docker.yml` — sibling parity (3 workflows now: ci, release, docker).
- **S126**: all 4 sibling READMEs share the same "Testing examples" stanza pointing at `go test ./examples/...` (or `./e2e/...` for mockway). Shell scripts stay as documented supplementary aids.
- **S127**: `handlers/contract_audit_test.go` rolled out across all 4 siblings (5th instance of the 4-PR cross-repo doc/code sweep pattern). Empty-state passes; future contracts inherit enforcement.

Full close-out: `docs/status/ARCHIVE.md` § "2026-06-10 fakegenesys v0.2 hardening".

## ACTIVE ARC — read first

### Ready to run autonomously. Scope: S143 run 2, then arc close-out, then STOP.

`docs/plans/layer3-real-scaleway-plan.md`. Everything except run 2 and the close-out has shipped — 13 PRs (infrafactory #136–#146, mockway #21).

**Layer 3 works.** On 2026-08-22 it applied to and destroyed real Scaleway infrastructure with the sweep confirming clean:

```
sandbox_deploy/preflight:    pass (endpoint asserted as https://api.scaleway.com)
sandbox_deploy/init/plan/apply/destroy: pass
sandbox_deploy/orphan_sweep: pass (project destroyed; no resources left outside it)
```

That was run 1, with **fixed HCL**, deliberately — isolating the Layer 3 harness from LLM generation on the first-ever real apply. It found two bugs immediately (both fixed in #144): the sweep read state after destroy emptied it, and mockway's seeded default project counted as a Layer 2 orphan, which would have failed `no_orphans` for every scenario in the suite.

#### What to do

1. **S143 run 2** — `infrafactory run scenarios/training/block-paris.yaml` with Layer 3 enabled, letting the **LLM generate the HCL**. Run 1 proved the harness; this proves the pipeline. Expect the generator to need the `scaleway_account_project` (ADR-0010) — `validateLayer3ProjectResource` enforces it and `Layer3Guidance` prompts for it, but that path has never been exercised against a real apply.
2. **Arc close-out** (mandatory per AGENTS.md): `docs/status/ARCHIVE.md` section, `docs/NEXT_SESSION.md` repoint, `STATUS.md` trim.
3. **Stop.** Do not widen to more scenarios without asking.

#### Setup that already works

- **Credentials**: `~/.config/scw/config.yaml` has a working key. Export `SCW_ACCESS_KEY` / `SCW_SECRET_KEY` / `SCW_DEFAULT_ORGANIZATION_ID` from it — the org id is required (each run creates its own project).
- **Config**: `/tmp/l3run/infrafactory.yaml` (ephemeral; recreate if gone). It needs `constraint_policies` mapped to **absolute** policy paths or `mock_deploy/state_policy` fails.
- **mockway must be running** on :8080, built from current main (needs the S140 `account/v3` handlers): `go -C ../mockway build -o /tmp/mockway ./cmd/mockway && /tmp/mockway --port 8080 &`
- **M99 is fixed** (#146), so `make test` now passes with mockway up. No more stop-the-mock-before-committing dance.

#### Account facts that constrain everything

The org holds **`openclaw`** (`0b6a8a6a-…`) with live infrastructure — a 20GB volume at minimum. Never let a destroy path near it; `harness.AssertProjectDeletable` enforces this, and the dedicated `infrafactory` project (`2397e80e-…`) catches resources that omit `project_id`.

#### Repo mechanics that will bite

- `main` on both repos has a **ruleset** (`gh api repos/<o>/<r>/rulesets`, not `/branches/main/protection`). infrafactory requires 1 approval → self-merge needs `--admin`. Both now require status checks.
- A **PreToolUse hook** blocks `gh pr merge` unless every check on the head SHA is green. It has no bypass; fix the checks.
- **Doc Hygiene is CI-only.** Any `internal/cli/` or `cmd/infrafactory/` change needs an ADR touch; any `cmd/|internal/|prompts/|policies/|scenarios/` change needs `STATUS.md`. The pre-commit hook does not check this, so it fails after push.
- `make test` rewrites `ui/package-lock.json` (strips `libc` fields). **Restore it before `git add`, not after** — #144 committed the churn and needed #145 to undo it.
- `gofmt -w <dir>` reformats unrelated files; ~19 are already unformatted on main. Format only what you changed.



**`docs/plans/layer3-real-scaleway-plan.md`** (S139–S143, planned 2026-08-22). Goal: get one scenario (`block-paris`) through apply → verify → destroy → provably-zero-orphans against **real Scaleway**.

Layer 3 has been code-complete since S30 (Slices 26–30: plan/apply/destroy on `terraform-live.tfstate`, real probes, credential preflight, auto-destroy, UI toggle) and has **never made a single call to `api.scaleway.com`**. It was validated entirely against mockway standing in for Scaleway.

Two blockers found during planning, both of which mean Layer 3 is currently unreachable end-to-end:

- **B1 false-green env leak** — `sandboxCommandEnv()` (`internal/cli/test_command.go:559`) returns only the two credential keys, and `execCommandRunner` merges `os.Environ()` on top. An override map can set but never unset, so an inherited `SCW_API_URL` retargets the "real" apply at mockway and it reports pass. No test asserts otherwise. Fixed in S139.
- **B2 self-managed-project deadlock** — ADR-0010 requires `scaleway_account_project` in the HCL and `validateLayer3ProjectResource` enforces it, but mockway registers only `/account/v2alpha1/ssh-keys` and 501s the project call. Layer 2 gates Layer 3, so mock apply fails and Layer 3 never runs. Fixed in S140 (mockway PR).

Verified NOT gaps (don't re-investigate): `tofu plan/apply -state=` works on OpenTofu 1.11.5; same-HCL dual-apply is sound for Scaleway because the provider block is bare and the endpoint comes only from `SCW_API_URL`; auto-destroy-on-failure and `plan-live.txt` capture are both implemented.

**S139–S142 are autonomous. S143 is operator-gated** — it needs `SCW_ACCESS_KEY` / `SCW_SECRET_KEY` / `SCW_DEFAULT_ORGANIZATION_ID` with project-manager rights, and it spends real money.

## Next arc candidates (no commitment)

1. **AGENTS.md + README.md optimisation sweep across all 5 repos** (carried directive from 2026-06-05). Cross-repo docs cleanup — same 4-PR sweep pattern as S126/S127. Now unblocked.
2. **Pitfall-pruning automation** (shelved 2026-06-06; S107 slot). `docs/plans/pitfall-pruning-automation-plan.md`. Detects pitfalls that haven't fired in N sweeps and demotes them.
3. **5th cloud** (speculative — no concrete request). The day-one OSS checklist + contract-audit convention are now durable enough that adding a 5th cloud would be ~1 session of structural work.
4. **fakegenesys public visibility flip + branch protection** — operator click-ops, not engineering. Still pending.

## Standing preferences (this user)

- **Don't let codex nitpick.** Act on substantive only. Stop after 2 no-substantive passes.
- **Sustain sweeps cover ALL scenarios** by default. Reduced-scope is a per-loop override only on explicit user directive.
- **Mature OSS scope from day one** for new sibling fakes — 14 items now (the original 13 + `handlers/contract_audit_test.go`). See `feedback_oss_mature_day_one.md`.
- **Contract coverage, not line count.** When proposing a test-backfill arc, frame coverage along the three bars (regression-per-mock-gap, docstring-derived, nil-deref defenses) — not "≥N test lines." See `feedback_test_coverage_metrics.md`.
- **Cost-sensitive on CI.** Don't pitch nightly sweeps unprompted.
- **`/loop` autonomous execution is the default for big arcs.**
- **NEVER hand-edit `pitfalls/*.yaml`.** Auto-learning writes them; prompts + code are the legitimate intervention points.
- **`repair_budget_exhausted` is never "expected cold-start"** unless the auto-learning pipeline has emitted at least one pitfall.
- **Adding a new cloud means updating three regex/switch sites in lockstep**: `internal/generator/pitfalls_learn.go::resourceNameRe`, `internal/generator/prescriptive_extractor.go::addressRe`, `internal/cli/run_command.go::pitfallResourceMatchesCloud`. ADR-0021.

## Sweep entry point

`make sweep-N`. Output: `/tmp/sweep-*/summary.tsv` + `panics.log` + per-scenario logs.

Reduced-scope override: `SCENARIOS_FILE=/path/to/list.txt make sweep-N`.

## Recent arcs (full close-outs in `docs/status/ARCHIVE.md`)

- **S123–S127 (2026-06-10)**: fakegenesys v0.2 hardening + cross-repo contract audit rollout. 7 PRs across 4 repos. v0.2.0 tagged. ADR-free arc (no schema/architecture changes).
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
