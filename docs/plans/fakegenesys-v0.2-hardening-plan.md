# Arc: fakegenesys v0.2 hardening (sibling-parity close)

Status: planned (2026-06-10)
Owner: next-session claude (designed for autonomous execution)
Follows: `fakegenesys-arc-plan.md` (S108–S115 structural integration) + 2026-06-10 sustain validation (S116–S122; sweep 8 = 44/44, sweep 11 = 8/8, v0.1.0 tagged).
Shape: goal-named variable-length arc per AGENTS.md "Planning a New Arc" (4 slices, ~6–9 hr).

## Big picture

fakegenesys shipped v0.1.0 functionally-mature and OSS-mature day-one: it passes the harness's full and reduced sustain sweeps, has the full OSS checklist (LICENSE/SECURITY/CONTRIBUTING/COC/CHANGELOG/.gitleaks/pre-commit), is spec-driven, has a TLS MITM proxy, and drained 7 mock-gap layers (S122a–g) under real provider-call drainage.

Where v0.1.0 still **lags** its three sibling fakes (mockway, fakegcp, fakeaws):

| Gap | mockway | fakegcp | fakeaws | fakegenesys (v0.1.0) |
|---|---|---|---|---|
| Go test lines | 9259 | 6416 | 8194 | **1918** (4–5× smaller) |
| `.github/workflows/docker.yml` | ✓ | ✓ | ✓ | **✗** |
| Example shell harness | 3 scripts + audit | `e2e.sh` | (in-test) | **(none)** |
| Codex review-pass depth | (mature) | (mature) | 17 passes | **2 passes (pre-S116)** |

The fidelity is real (S122 drainage = sweep-driven hardening), but the **standalone surface hasn't been hardened to sibling-parity yet**. This arc closes those gaps in the same shape fakeaws's S48 hardening loop did, ending in `v0.2.0`.

- **S123** backfills table-driven unit tests for the S122a–g endpoints and the broader post-S116 surface (TLS MITM, organization/tokens probes, user subresources, subjects/grants, password POST + bulkadd). Target: ~6k test lines (3× v0.1.0, parity-floor with fakegcp).
- **S124** runs a structured codex review pass against the post-S122 surface — same loop fakeaws used (substantive-only, anti-nitpick, stop on 2 consecutive NOTHING_TO_IMPROVE). Documents passes in `docs/review-passes/passN.md`.
- **S125** adds `.github/workflows/docker.yml` mirroring the sibling pattern (multi-arch buildx, push to ghcr on tag).
- **S126** converges all four sibling fakes onto the **fakeaws in-test pattern** for example-driven coverage: extend each repo's `examples/provider_smoke_test.go` from a provider-init smoke pass into a per-example `tofu apply` matrix (with `t.Parallel()` + `t.TempDir()`), then retire the shell scripts (`mockway/scripts/test-examples.sh` + `test-misconfigured.sh` + `test-updates.sh`; `fakegcp/scripts/e2e.sh`). 4-PR cross-repo sweep per `reference_cross_repo_docs_sweep.md` — one PR per sibling plus the infrafactory close-out. Folds the v0.2.0 fakegenesys tag + arc close-out.

S126 owns the close-out per Option C (no separate slice).

### Why in-test, not shell

fakeaws is the most recent sibling and chose in-test deliberately. The mockway/fakegcp shell scripts predate that pattern and aren't a considered "better than in-test" choice — they're earlier convergence. The in-test pattern wins on every dimension that matters to a contributor:

| Dimension | in-test (fakeaws) | shell (mockway/fakegcp) |
|---|---|---|
| Selective re-run | `go test -run TestExamples/working/foo` | none — script runs everything |
| Parallel execution | `t.Parallel()` free | sequential |
| CI coverage | runs in `make test` automatically | needs separate CI step |
| Setup/teardown | `t.TempDir()` + `t.Cleanup()` | bash trap acrobatics |
| Cross-platform | yes | bash/grep/sed portability bugs |
| Maintenance surface | one Go file | one Go file + N bash scripts |

Operator UX is the only argument for shell, and anyone running these fakes has Go (they built the binary) or pulls the container.

## Slices

| Slice | Title | Effort |
|---|---|---|
| S123 | Table-driven test backfill for S122a–g + post-S116 surface | ~3–4 hr |
| S124 | Codex review pass loop (post-S116 surface) — close on 2× NOTHING_TO_IMPROVE | ~1–2 hr |
| S125 | `.github/workflows/docker.yml` (multi-arch + ghcr) | ~30 min |
| S126 | Cross-repo example-test convergence on the fakeaws in-test pattern + README updates + v0.2.0 tag + arc close-out | ~2.5–3.5 hr + close-out |

**Total**: ~7–10 focused hours.

## Standing rules

Inherit all rules from `fakegenesys-arc-plan.md`, `slices-43-48-plan.md` (fakeaws review-loop precedent), and S116–S122 (see `docs/status/ARCHIVE.md` § "2026-06-10 fakegenesys S116–S122 sustain validation"). Specifically:

- **Codex anti-nitpick** (S124): act on substantive only; stop after 2 consecutive NOTHING_TO_IMPROVE passes; record declined items in `docs/review-passes/passN.md`. See `feedback_codex_anti_nitpick.md`.
- **OSS-mature day-one** (S125/S126): docker.yml + scripts mirror sibling layout exactly. No bespoke patterns. See `feedback_oss_mature_day_one.md`.
- **No nightly CI cost** (S125): the workflow runs on tag push only, NOT on a nightly schedule. See `feedback_cost_sensitive_ci.md`.
- **Merge authority**: `gh pr merge <N> --squash --admin --delete-branch` once CI green, in `../fakegenesys`.
- **Test scope** (S123): exercise corner cases of S122a–g endpoints (PascalCase JSON key sensitivity, subject grants empty/bulkadd/bulkremove flow, password POST 204, user/groups subresources). Do NOT pad coverage by exercising trivial happy paths already covered by integration; favor the contracts the genesyscloud provider depends on (recorded in handler docstrings).
- **All work in `../fakegenesys`** — no infrafactory changes expected unless S123 surfaces a real fidelity bug, in which case follow the standard cross-repo pattern.

## S123 — Table-driven test backfill for S122a–g + post-S116 surface

### Motivation

`find ../fakegenesys -name '*_test.go' | xargs wc -l` returns 1918 across 8 files. Sibling floor is ~6.4k (fakegcp). Most S116c/S119/S122a–g endpoints landed under sweep pressure without unit-level corner-case tests — the handler docstrings document the wire-shape requirements, but there's no automated test enforcing them. Future refactors (e.g. if someone touches `organization.go::handleTokensMe`) could silently break the `OAuthClient` PascalCase key invariant and the regression wouldn't surface until the next sustain sweep.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S123-T1 | Inventory: list every handler added in S116c, S119, S122a, S122b, S122c, S122d, S122f, S122g. Map each to its existing test (if any) in `handlers/*_test.go`. Output a gap matrix in the PR description. | P0 | — |
| S123-T2 | Add `handlers/tokens_me_test.go` — exercise `/api/v2/tokens/me`. Assert response JSON contains key literally named `"OAuthClient"` (PascalCase, not camelCase — the SDK's custom UnmarshalJSON requires it). Assert `.OAuthClient.organization.id == "purecloud-builtin"`. This is THE critical invariant — without it, every oauth_client create segfaults. | P0 | S123-T1 |
| S123-T3 | Add `handlers/authorization_subject_test.go` — exercise `GET /api/v2/authorization/subjects/{id}` and `POST /api/v2/authorization/subjects/{id}/bulkadd` / `/bulkremove`. Assert 200 with `grants:[]` on GET; 204 on POST bulkadd/bulkremove. | P0 | S123-T1 |
| S123-T4 | Add `handlers/user_password_test.go` — exercise `POST /api/v2/users/{id}/password`. Assert 204 regardless of body. | P1 | S123-T1 |
| S123-T5 | Add `handlers/users_me_test.go` — exercise `GET /api/v2/users/me`. Assert 200 with non-nil `division.id` (terraform user must have a division — the provider dereferences it). | P0 | S123-T1 |
| S123-T6 | Add `handlers/user_roles_test.go` — exercise GET/PUT `/api/v2/users/{id}/roles`. Assert GET returns `version` + `roles[]`; PUT echoes the role IDs with `selfUri` populated. | P1 | S123-T1 |
| S123-T7 | Add `handlers/group_subresources_test.go` — exercise `/api/v2/groups/{id}/individuals`, `/members`, `/voicemail`. Assert 200 + correct paged shape (`entities` / `total`). | P1 | S123-T1 |
| S123-T8 | Add `handlers/flow_jobs_test.go` — exercise the flow upload-job protocol: POST `/api/v2/flows/jobs` → 200 with presigned URL fields → PUT (or accept) the artifact → GET `/api/v2/flows/jobs/{id}` returns terminal status. | P1 | S123-T1 |
| S123-T9 | Add `handlers/responsemanagement_libraries_test.go` — exercise list/create endpoints (added in S122c alongside the OAuthClient fix). | P2 | S123-T1 |
| S123-T10 | Extend `architect_test.go` (or add `architect_datatable_test.go`) — assert datatable response includes `division` field (S122b dropped this in alongside the flow upload-job protocol). | P2 | S123-T1 |
| S123-T11 | Run `go test ./...` from `../fakegenesys`. Run `make test` from infrafactory. Confirm both pass. Confirm `wc -l handlers/*_test.go` shows ≥ 6000 lines (parity floor with fakegcp). | P0 | S123-T2 ... S123-T10 |
| S123-T12 | Single PR with all new tests. PR description includes the gap matrix from T1 + line-count delta. Title: `S123: table-driven test backfill for S116/S122 surface`. | P0 | S123-T11 |

### Exit criteria

- All 9 endpoint families from S116c/S119/S122a–g have at least one test file with at least one assert on the contract the genesyscloud provider relies on (documented in the existing handler docstrings).
- `wc -l handlers/*_test.go` ≥ 6000.
- `go test ./...` green; CI green; `make test` green; PR squash-merged.

## S124 — Codex review pass loop on post-S116 surface

### Motivation

The fakegenesys codex review loop closed at pass 3 in S113, **before** any of the S116–S122 surface existed. The TLS MITM proxy, all 7 S122 mock-gap layers, the auto-learning regex fix, and the S125 docker workflow are all unreviewed by an independent agent. fakeaws ran 17 review passes before tagging; mockway/fakegcp matured organically over 9 / multiple releases. fakegenesys can't replicate that organic timeline without a structured review loop.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S124-T1 | Spawn codex review on `../fakegenesys` HEAD. Constrain scope to `handlers/` + `tls_mitm.go` + `.github/workflows/` + `cmd/`. Anti-nitpick prompt per `feedback_codex_anti_nitpick.md`. | P0 | S123 merged |
| S124-T2 | Triage findings: substantive vs nitpick. Document declined nitpicks in `docs/review-passes/pass3.md` (continues fakegenesys's numbering from S112/S113 pass 1+2). | P0 | S124-T1 |
| S124-T3 | Fix substantive findings in a single PR. Re-run review (pass 4). Repeat until 2 consecutive NOTHING_TO_IMPROVE passes. | P0 | S124-T2 |
| S124-T4 | Final pass document: `docs/review-passes/passN.md` (N = final pass number) with NOTHING_TO_IMPROVE × 2 entry. Loop-close marker. | P0 | S124-T3 |

### Exit criteria

- ≥ 2 consecutive review passes with NOTHING_TO_IMPROVE.
- Each pass documented in `docs/review-passes/passN.md`.
- All substantive findings landed via squash-merged PRs.
- Nitpicks documented as declined with one-line rationale.

## S125 — Docker workflow parity

### Motivation

`ls ../mockway/.github/workflows/` returns `ci.yml docker.yml release.yml`. fakegenesys is missing `docker.yml`. Siblings ship a container build on tag push (multi-arch buildx; pushes to `ghcr.io/redscaresu/<repo>:<tag>` and `:latest`). Operators consuming fakegenesys via container are blocked.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S125-T1 | Copy `../mockway/.github/workflows/docker.yml` to `../fakegenesys/.github/workflows/docker.yml`. Adjust the image name from `mockway` → `fakegenesys`. Verify Dockerfile in `../fakegenesys` builds locally first (`docker build -t fakegenesys-test .`). | P0 | — |
| S125-T2 | Verify the workflow's trigger gates on tag-push only (NOT a nightly schedule — `feedback_cost_sensitive_ci.md`). | P0 | S125-T1 |
| S125-T3 | Single PR. CI must pass. Title: `S125: add docker.yml workflow (sibling parity)`. | P0 | S125-T2 |

### Exit criteria

- `.github/workflows/docker.yml` exists in `../fakegenesys`.
- Triggers on tag push only.
- PR squash-merged; CI green.

## S126 — Cross-repo example-test convergence + READMEs + v0.2.0 + arc close-out

### Motivation

Three different example-test patterns across four sibling fakes is incoherent: mockway has 3 shell scripts (`test-examples.sh` + `test-misconfigured.sh` + `test-updates.sh`), fakegcp has 1 (`e2e.sh`), fakeaws drives examples in-test, and fakegenesys runs only a provider-init smoke pass. Pick one. fakeaws's in-test pattern wins on every dimension that matters (see § "Why in-test, not shell" above) and is the most recent considered choice, so it becomes canonical.

This slice converges all four siblings onto that pattern in a single cross-repo sweep (`reference_cross_repo_docs_sweep.md` — 4-PR shape), updates each repo's README to point at `go test ./examples/...` as the canonical entry point, retires the shell scripts, and folds the v0.2.0 fakegenesys tag + arc close-out.

### Tickets

#### Reference read

| ID | Description | Priority | Deps |
|---|---|---|---|
| S126-T1 | Read `../fakeaws/examples/provider_smoke_test.go` — this is the canonical pattern. Note: how it discovers example dirs, how it sets up the workdir, how it invokes `tofu init` + `tofu apply` + `tofu destroy`, how it uses `t.Parallel()` + `t.TempDir()` + `t.Cleanup()`, how it asserts. Document the contract in a one-page comment block at the top of the PR series description. | P0 | S123, S124, S125 merged |

#### PR 1 — fakegenesys (the in-flight repo)

| ID | Description | Priority | Deps |
|---|---|---|---|
| S126-T2 | Extend `../fakegenesys/examples/provider_smoke_test.go` from a provider-init smoke pass into a per-example matrix: discover `examples/working/*` dirs, `t.Parallel()` one subtest per dir, `tofu init`+`apply`+`destroy` against a freshly-booted fakegenesys, assert success. Add a parallel `examples/misconfigured/*` matrix that asserts `apply` fails with the documented error class (read existing `examples/misconfigured/README.md` for the expected error per dir if one exists, otherwise just assert non-zero exit). | P0 | S126-T1 |
| S126-T3 | Update `../fakegenesys/README.md` — add a "Testing examples" section with `go test ./examples/...` as the entry point. Mention `-run TestExamples/working/foo` for selective re-run and `-v` for verbose output. | P0 | S126-T2 |
| S126-T4 | Single PR. Title: `S126/fakegenesys: per-example apply matrix in provider_smoke_test.go`. | P0 | S126-T3 |

#### PR 2 — mockway

| ID | Description | Priority | Deps |
|---|---|---|---|
| S126-T5 | Extend `../mockway/examples/provider_smoke_test.go` to a per-example apply matrix (same shape as T2). Cover `examples/working/`, `examples/misconfigured/`, and `examples/updates/`. The `updates/` matrix is two-phase: apply v1, apply v2, assert idempotency. | P0 | S126-T1 |
| S126-T6 | Delete `../mockway/scripts/test-examples.sh`, `test-misconfigured.sh`, `test-updates.sh`. Remove `make examples-test` and equivalent Makefile targets that wrap them. | P0 | S126-T5 |
| S126-T7 | Update `../mockway/README.md` — replace any references to the three shell scripts with `go test ./examples/...`. Add the "Testing examples" section per the fakeaws/fakegenesys pattern. | P0 | S126-T6 |
| S126-T8 | Single PR. Title: `S126/mockway: retire shell example harness, converge on in-test pattern`. | P0 | S126-T7 |

#### PR 3 — fakegcp

| ID | Description | Priority | Deps |
|---|---|---|---|
| S126-T9 | Extend `../fakegcp/examples/provider_smoke_test.go` to a per-example apply matrix (same shape as T2). Cover `examples/working/`, `examples/misconfigured/`, `examples/updates/`. | P0 | S126-T1 |
| S126-T10 | Delete `../fakegcp/scripts/e2e.sh`. Remove any wrapping Makefile target. | P0 | S126-T9 |
| S126-T11 | Update `../fakegcp/README.md` — replace any references to `e2e.sh` with `go test ./examples/...`. Add the "Testing examples" section. | P0 | S126-T10 |
| S126-T12 | Single PR. Title: `S126/fakegcp: retire scripts/e2e.sh, converge on in-test pattern`. | P0 | S126-T11 |

#### PR 4 — fakeaws (README only)

fakeaws already has the canonical implementation. It only needs a README touch-up to match the new wording the other three repos converge on, so a contributor reading any of the four READMEs sees the same "Testing examples" stanza.

| ID | Description | Priority | Deps |
|---|---|---|---|
| S126-T13 | Update `../fakeaws/README.md` — add/normalise the "Testing examples" section to match the wording the other three repos converge on. Cross-reference the in-test pattern as the canonical sibling approach. | P0 | S126-T1 |
| S126-T14 | Single PR. Title: `S126/fakeaws: README — canonicalise "Testing examples" stanza`. | P0 | S126-T13 |

#### Infrafactory close-out + release

| ID | Description | Priority | Deps |
|---|---|---|---|
| S126-T15 | Update `infrafactory/AGENTS.md` — in the cross-cutting fidelity-strategy comparison table (or the smoke-harness section if separate), add a note that **example-driven coverage is in-test (`go test ./examples/...`) across all four siblings as of S126** — same convergence pattern as the smoke-harness and fidelity-strategy doc sweeps. | P0 | S126-T4, S126-T8, S126-T12, S126-T14 all merged |
| S126-T16 | Tag `v0.2.0` in `../fakegenesys`: `git tag v0.2.0 && git push origin v0.2.0`. Update fakegenesys `CHANGELOG.md` with a `## [0.2.0] - 2026-MM-DD` section listing the four hardening dimensions (S123 tests, S124 review-loop closed, S125 docker workflow, S126 in-test convergence). | P0 | S126-T15 |
| S126-T17 | **Arc close-out** (Option C): append entry to `docs/status/ARCHIVE.md` § "2026-MM-DD fakegenesys v0.2 hardening — sibling parity close" with sub-section "Cross-repo convergence on in-test example coverage". Update `STATUS.md` baseline pointer. Refresh `docs/NEXT_SESSION.md` (next-arc candidate list). | P0 | S126-T16 |
| S126-T18 | Update memory: edit `project_fakegenesys_arc_closeout.md` to add a "v0.2 hardening (2026-MM-DD)" section. Update `reference_cross_repo_docs_sweep.md` to record S126 as the third instance of the 4-PR cross-repo pattern (smoke-harness, fidelity-strategy, in-test example coverage). Update `MEMORY.md` "Latest" entry. | P0 | S126-T17 |

### Exit criteria

- All four sibling repos have an `examples/provider_smoke_test.go` that drives `tofu init`+`apply`+`destroy` per-example via `t.Parallel()`.
- `mockway/scripts/test-examples.sh`, `mockway/scripts/test-misconfigured.sh`, `mockway/scripts/test-updates.sh`, `fakegcp/scripts/e2e.sh` are deleted.
- All four READMEs have a "Testing examples" section with the same wording, pointing at `go test ./examples/...`.
- infrafactory `AGENTS.md` records the cross-repo convergence.
- `v0.2.0` tag pushed; fakegenesys `CHANGELOG.md` has a `[0.2.0]` section.
- ARCHIVE + STATUS + NEXT_SESSION + MEMORY updated.
- All four PRs squash-merged with CI green.

## Parity-floor success metric

Run after S126-T6:
```
for repo in mockway fakegcp fakeaws fakegenesys; do
  cd ../$repo
  echo -n "$repo test lines: "
  find . -name '*_test.go' -not -path './vendor/*' -exec wc -l {} + | tail -1 | awk '{print $1}'
  echo -n "$repo workflows: "
  ls .github/workflows/ | wc -l
done
```

Target post-arc:
- fakegenesys test lines ≥ 6000 (currently 1918; floor ~ fakegcp 6416)
- fakegenesys workflow count = 3 (currently 2)
- fakegenesys review-pass count ≥ 4 with ≥ 2 consecutive NOTHING_TO_IMPROVE
- fakegenesys tagged at `v0.2.0`
- **Cross-repo**: all four `examples/provider_smoke_test.go` drive per-example apply matrix; zero example shell scripts remain across the four siblings; all four READMEs share the same "Testing examples" wording.

Sibling parity along all four originally-lagging dimensions, plus cross-sibling consistency on example-test pattern, is the close-out trigger.
