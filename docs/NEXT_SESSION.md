# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## Read this first (handoff state as of 2026-08-31)

### START HERE — S166, the teardown guard

**Read `docs/plans/s166-teardown-guard-design.md` first — it is decided and ready
to build.** All four judgement calls were answered on 2026-08-31 and are recorded
there as decisions; you do not need to re-open them, but the reasoning is written
down so you can tell whether a new fact should.

S166 replaces `AssertProjectDeletable`'s state-derived cross-check — the guard
between an automated destroy and real infrastructure. Under ADR-0025 the project
is no longer a Terraform resource, so `terraform-live.tfstate` stops naming it
and the check loses its input. The proposal is two checks, both required: a
run-owned marker beside the state (identity) plus an API-side provenance check
against S165's `if-run-` stamp (class, not locally forgeable). Neither alone is
sufficient, and provenance alone would *regress* — it authorises deleting any
stamped project, so parallel runs could delete each other's.

**S166 and S167 are ONE change** (decided 2026-08-31). The guard's input changes
at exactly the moment the HCL changes, so they cannot be separated — and the
transition that would have separated them was dropped: there is no fleet to
migrate, and two code paths is where this arc's bugs came from.
`scaleway.create_run_project` is scaffolding and gets deleted by the cutover.

All four judgement calls in the design are **answered**; it is ready to build.

### Done: S165 (merged, canaried)

`scaleway.create_run_project` (default **false**) creates the run's project via
the Account API before the apply and passes it as `SCW_DEFAULT_PROJECT_ID`.
Canary on real Scaleway, 2026-08-31: `block-paris`, 14.2s, every stage pass,
account back to its 3 baseline projects with both ids returning 404.

Two things to know before using it:

- Enabling it **before S167** gives a run two projects — the pre-created one and
  the one its HCL still declares. Nothing leaks (the canary cleaned up both), but
  it is waste. The flag is mechanically complete, not coherent, until S167.
- A run whose destroy falls to `run`'s auto-destroy-on-failure path keeps its
  project. Reported as a skipped delete, not silent.

Nine review passes went into S165 and almost none found a wrong computation.
Every real finding was an **operation ordered so a failure left something
behind** — worth expecting rather than rediscovering.

### THE BLOCKER — read before planning anything Scaleway + compute

**Layer 1 requires a resource Layer 3 cannot create.**
`policies/scaleway/vpc_required.rego` denies any `scaleway_instance_server`
without a private NIC, and it *is* evaluated for Scaleway (`filterPolicyPathsByCloud`
drops only *other* clouds). But `scaleway_instance_private_nic` has **no
`project_id` attribute** (provider 2.81.0), so it lands in the provider's default
project — the shared containment project — while its server is in the run's own,
and the API refuses the mismatch.

**No Scaleway compute scenario satisfies both gates today.** Not
`web-live-paris`, not any other.

The fix is planned and proven: **ADR-0025** + `docs/plans/run-owned-project-plan.md`
(S165–S168) — create the run's project via the Account API *before* the apply,
pass it as `SCW_DEFAULT_PROJECT_ID`, and drop `scaleway_account_project` from the
HCL. A hand-run experiment applied a private NIC cleanly this way and destroyed
cleanly. **S166 is the slice to be careful with**: it replaces
`AssertProjectDeletable`'s state-derived cross-check, and must land before S167
removes that check's input.

Two diagnoses of this blocker were wrong before the right one, both from reading
configuration instead of running something: `IPAMFullAccess` (the API actually
wanted `write compute_private_networks` — `PrivateNetworksFullAccess`, granted
2026-08-30), and "`vpc_required` is AWS-only". Both retracted in ADR-0024.

### Also planned, not started

- `docs/plans/ui-deployment-arc-plan.md` (S159–S164) — drive a deployment from
  the UI. Mostly wiring over proven code. **S160 (deploy safety model) comes
  before any button**: the UI is unauthenticated on localhost.
- `docs/plans/live-learning-loop-plan.md` (S154–S156) — the arc the live work
  exists for. Until S156 lands, live deployments produce **logs, not learning**.
  Note `ExtractFixPitfall` needs a *diff*, which is why S155 (upgrade) is what
  makes S156 produce prescriptive rules rather than weak symptom text.

### Operational gotchas learned the hard way

- **Review with Codex, not `/code-review`.** `codex exec review --base main`,
  archived to `docs/review-passes/`. A Claude review of Claude-written code shares
  its blind spots: three rounds of fixes each reproduced the failure they
  targeted. Codex pass 10 returned **one** finding where the Claude pass returned
  fifteen — and it was the most serious of them.
- **Running infrafactory from inside a Claude Code session** used to hang
  `self_review` for the full 300s timeout. Cause: `claude_adapter.go` filtered
  only `CLAUDECODE=` while a parent session exports nine more `CLAUDE_CODE_*`
  variables. Fixed, with credentials and provider routing explicitly kept.
- **Layer 3 credentials** live in the `layer3` GitHub *environment*, not the repo.
  Locally they must be `SCW_ACCESS_KEY` / `SCW_SECRET_KEY` /
  `SCW_DEFAULT_ORGANIZATION_ID` in the environment. Never
  `SCW_DEFAULT_PROJECT_ID` — it is stripped and set from
  `scaleway.fallback_project_id`.
- **Cost**: the `lb-serving-paris` shape is **EUR 0.042/hour** (DEV1-S 0.00898,
  LB-S 0.023, two IPv4 at 0.005) — list prices read 2026-08-30. The binding
  constraint on TTL is exposure and forgetting, not money.

### Open, not blocked

- **#163** (TypeScript 5→7) is verified broken: `@sveltejs/kit@2.70.3` declares
  `peerOptional typescript@"^5.3.3 || ^6.0.0"`, so `npm install` fails with
  ERESOLVE. Close it, or downgrade to TypeScript 6, which satisfies the range.
- `docs/layer3-coverage.md` carries a governing retraction banner; its older IPAM
  references are superseded by it.


## Next arc candidates (no commitment)

1. **`lb-paris` as probe canary** — the designated Layer 3 follow-on, and the arc's own nominated opener. The real-probe path (`connectivity` / `http_probe` / `dns_resolution`) is still unexercised against real infrastructure; `block-paris` declares no probes. Costs more than the block canary — a load balancer and its IP — so scope it deliberately.
2. **AGENTS.md + README.md optimisation sweep across all 5 repos** (carried directive from 2026-06-05). Cross-repo docs cleanup — same 4-PR sweep pattern as S126/S127. Now unblocked.
3. **Pitfall-pruning automation** (shelved 2026-06-06; S107 slot). `docs/plans/pitfall-pruning-automation-plan.md`. Detects pitfalls that haven't fired in N sweeps and demotes them.
4. **5th cloud** (speculative — no concrete request). The day-one OSS checklist + contract-audit convention are now durable enough that adding a 5th cloud would be ~1 session of structural work.
5. **fakegenesys public visibility flip + branch protection** — operator click-ops, not engineering. Still pending.

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
