# Next Session

Self-contained brief for a fresh Claude / engineer starting in this repo.

## Read this first (handoff state as of 2026-06-06 end-of-session)

### What landed this session (S116–S121)

- **fakegenesys#11 (S116)** + **fakegenesys#12 (S116b)**: TLS MITM CONNECT-proxy on `:8443` with persisted CA at `~/.fakegenesys/`. The genesyscloud provider hits `login.<region>.pure.cloud` unchanged; the proxy MITM-terminates TLS with leaf certs signed by a boot-time self-signed CA. CA exposed at `<fakegenesys>/mock/ca-cert`.
- **infrafactory#94 (S117)**: `cloudEnv` fetches the CA at runtime and sets `HTTPS_PROXY` + `SSL_CERT_FILE` + `NO_PROXY` for cloud:genesys scenarios.
- **fakegenesys#13 + #14 + #15 (S116c, S119)**: 10+ read-after-create mock-gap fixes — post-auth SDK probes (`/organizations/me`, `/authorization/products`, `/authorization/divisions{/,/home}`, `/tokens/me`), OAuth Basic Auth, user `division` default, voicemail userpolicy, user routing utilization, user routing skills/languages, `/users/search` with the correct `{results:[...]}` shape, routing queue `memberCount` derivation, routing queue create returns 200 (not 201), wrapupcodes subresource. The S119 fix added `oAuthClient.organization.id = "purecloud-builtin"` to `/tokens/me` — the OAuth client create path dereferenced this unconditionally and segfaulted the plugin.
- **infrafactory#96 (S118)**: cloud-prefix set in the auto-learning pipeline. `resourceNameRe`, `addressRe`, `pitfallResourceMatchesCloud` were hardcoded to `(scaleway|google|aws)_`. After S114 added genesys as a peer cloud, **zero pitfalls had ever been auto-learned from any genesys run** because `ExtractResourceFromDetail` returned `""` for `genesyscloud_*` resources. ADR-0021 codifies the three-site lockstep rule + AGENTS.md § "The auto-learning pipeline is load-bearing — never excuse its silence" + regression test.
- **infrafactory#97 (S120)**: prompt-level guidance for the `genesyscloud_flow` + `local_file` pattern. The provider rejects `file_content_hash` as unconfigurable; the LLM was oscillating on it across all five sweep iterations.

### Sustain validation outcome (S119/S120/S121)

| Sweep | target_reached | failures | Notes |
|---|---|---|---|
| 1 (cold start, post-S118 regex fix) | 40/44 | 3 genesys + 1 web-app-paris transport-flake | First sweep where auto-learning emits genesys pitfalls (4 events, 2 net new entries) |
| 2 | 38/44 | 3 genesys + 3 transport flakes | Session-limit reset between sweeps fixed the transport flakes |
| 3 (post-S119 + S120) | 41/44 | 3 genesys persistent | aws/gcp/scaleway: 39/39 clean. genesys: 2/5 |

**The 44/44 × 3 sweep win condition is NOT met.** Stopping under the "genuinely cannot proceed without harness work" branch.

### What's blocking (must be addressed before next sustain sweep)

Three genesys scenarios fail consistently. Each has a known root cause and a concrete fix:

1. **`genesys-architect-flow` + `genesys-full-stack`** (repair_budget_exhausted, both scenarios) — `genesyscloud_flow` reads `filepath` at PLAN time via CustomizeDiff. No tofu pattern (`local_file` + `depends_on`, `null_resource`, etc.) can satisfy this because the file must exist on disk BEFORE tofu plan runs. S120's prompt guidance steered the LLM to use the right pattern, but the underlying constraint can't be solved in HCL.

   **Fix path**: harness-level pre-placement. In `internal/harness/` or `internal/cli/`, when running a `cloud: genesys` scenario, write a minimal `flow.yaml` (and any other declared assets) into the workdir before invoking tofu init. Either: (a) the scenario YAML grows an `assets:` section listing files + contents, or (b) the harness drops a fixed `flow.yaml` stub when it detects `genesyscloud_flow` references. Option (a) is the cleaner design — generalises to other file-on-disk dependencies.

2. **`genesys-rbac-and-oauth`** (repair_budget_exhausted) — three sub-issues:
   - LLM keeps writing `authorized_grant_type = "CLIENT_CREDENTIALS"` (underscore) — the provider wants `"CLIENT-CREDENTIALS"` (hyphen). Pitfall is learned but the descriptive form isn't enough; the LLM trusts its training data. **Fix**: promote to a prompt-level instruction in `prompts/genesys/phase2_generate_hcl.md` § "Instructions" — the same way S120 handled the flow file pattern.
   - `genesyscloud_group` plugin crash (iter 2, sweep 3). Likely missing fields in the group create or read response — similar pattern to S116c's user `division` fix. Reproduce with a single-resource HCL, find the panic line, add the missing field.
   - `/api/v2/groups/{groupId}/voicemail` returns 501 (iter 4, 5, sweep 3). Add the handler — mirror voicemail userpolicy at `handlers/voicemail.go`.

### What's stable

- **39/39 non-genesys scenarios pass in all three sweeps.** No regressions from any of the S116–S120 changes.
- **Auto-learning pipeline is now genesys-aware** — pitfalls/genesys.yaml grew from 2 entries (S118 commit) to 9 net entries across the three sweeps. The pipeline IS working.
- **fakegenesys + TLS MITM end-to-end** is solid. `tofu apply` of a five-resource basic-queue scenario completes in <1 min.

### Standing rules that came up this session

- **`repair_budget_exhausted` is never "expected cold-start"** unless you've verified the auto-learning pipeline emitted at least one pitfall. AGENTS.md § "The auto-learning pipeline is load-bearing" + `feedback_learning_failure_is_a_bug.md` + ADR-0021 all reinforce this. The S118 fix surfaced because the user pushed back on my "expected" framing.
- **Adding a new cloud means updating three regex/switch sites in lockstep**: `internal/generator/pitfalls_learn.go::resourceNameRe`, `internal/generator/prescriptive_extractor.go::addressRe`, `internal/cli/run_command.go::pitfallResourceMatchesCloud`. Missing one breaks learning silently for that cloud. ADR-0021.

### In flight (no action required unless CI flakes)

None. All session PRs are merged.

## Standing preferences (this user)

- **Don't let codex nitpick.** Act on substantive only. Stop after 2 no-substantive passes.
- **Sustain sweeps cover ALL scenarios.** Never reduce.
- **Mature OSS scope from day one** for new sibling fakes.
- **Cost-sensitive on CI.** Don't pitch nightly sweeps unprompted.
- **`/loop` autonomous execution is the default for big arcs.**
- **NEVER hand-edit `pitfalls/*.yaml`.** Auto-learning writes them; prompts + code are the legitimate intervention points.

## Outstanding directive (carried)

User asked 2026-06-05: **"update, optimise agents.md, readmes.md across all the repos"** once the fidelity sweep lands. Still queued; could be a single-slice docs arc after the genesys harness/pitfall work above lands.

## Sweep entry point

`make sweep-N`. Output: `/tmp/sweep-*/summary.tsv` + `panics.log` + per-scenario logs. Summary lines:
- `PASS=X / TOTAL=Y (deterministic: X/Z; transport_failed: W)`
- `PANIC_LINES=N`
- `AVOID_EMISSIONS=N` (per-cloud breakdown in pitfall-merge output)
- `RETRY_TRANSPORT=N` / `RETRY_RECOVERED=M`
- `TRANSPORT_FAILED=N`

## Recent arcs (full close-outs in `docs/status/ARCHIVE.md`)

- **S116–S121 (this session, 2026-06-06)**: TLS MITM + read-after-create stubs + auto-learning genesys-awareness + sustain sweeps 1/2/3. 41/44 ceiling; 3 genesys scenarios blocked by described root causes. 8 PRs merged across infrafactory + fakegenesys.
- **S108–S115 (2026-06-06)**: fakegenesys arc shipped — 4th cloud structurally integrated. 9 PRs + 3 cross-link PRs.
- **sustain re-validation + transport retry** (2026-06-04): 2 PRs.
- **post-sustain tightening** (2026-06-03): 4 PRs + 1 fakeaws.
- **sustain + N13 durability** (2026-06-03): 2 PRs.
- **S89–S93** (2026-06-03): 🎯 39/39 first deterministic. 3 PRs.
- **S84–S88** (2026-06-03): gcp-full-stack convergence + panic gate. 3 PRs.
- **S79–S83** (2026-06-02): sibling-mock drainage + carve-out. 4 PRs.
- **S74–S78** (2026-06-02): phase3 collapse + `make sweep-39` + N3 carve-out. 5 PRs.
- **S54–S73**: GCP phase2 collapse, sustain ratchets, N3/N10/N13 architecture. ~22 PRs.
