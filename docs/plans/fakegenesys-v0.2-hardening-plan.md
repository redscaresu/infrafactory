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
| Contract test coverage of post-S116 surface | n/a (no equivalent arc) | n/a | n/a | **most S122a–g endpoints have no regression test** — wire-shape invariants live only in handler docstrings |
| `.github/workflows/docker.yml` | ✓ | ✓ | ✓ | **✗** |
| Example test pattern | 3 shell scripts | `e2e.sh` | in-test apply matrix | provider-init smoke only — no per-example `tofu apply` coverage |
| Codex review-pass depth | (mature) | (mature) | 17 passes | **2 passes (pre-S116)** — entire TLS MITM + S122 surface unreviewed |
| Durable contract audit (CI-enforced) | ✗ | ✗ | ✗ | ✗ — applies to all four; addressed cross-repo in S127 |

(API surface differs across the four fakes — CCaaS is genuinely smaller than IaaS. The relevant question is not "how many test lines" but "is every contract that matters locked in by a test." S123 answers that with a per-contract matrix; see § "Coverage bars" below.)

The fidelity is real (S122 drainage = sweep-driven hardening), but the **standalone surface hasn't been hardened to sibling-parity yet**. This arc closes those gaps in the same shape fakeaws's S48 hardening loop did, ending in `v0.2.0`.

- **S123** backfills contract-driven unit tests for the S122a–g endpoints and the broader post-S116 surface. Target is **not** a line count (forces padding on a smaller API); it's **contract coverage** along three explicit bars:
  - Every S122a–g mock-gap layer has a regression test asserting what would have failed *before* the fix (revert-the-fix → test breaks).
  - Every `CRITICAL:`/`MUST` note in handler docstrings becomes one explicit assertion (the docstrings already enumerate the invariants — convert them, don't reinvent them).
  - Every nil-deref crash site the genesyscloud SDK walks gets an explicit non-nil assertion on the stub response (e.g. `OAuthClient.Organization.Id`, `Division.Id`).
- **S124** runs a structured codex review pass against the post-S122 surface — same loop fakeaws used (substantive-only, anti-nitpick, stop on 2 consecutive NOTHING_TO_IMPROVE). Documents passes in `docs/review-passes/passN.md`.
- **S125** adds `.github/workflows/docker.yml` mirroring the sibling pattern (multi-arch buildx, push to ghcr on tag).
- **S126** converges all four sibling fakes onto the **fakeaws in-test pattern** for example-driven coverage: extend each repo's `examples/provider_smoke_test.go` from a provider-init smoke pass into a per-example `tofu apply` matrix (with `t.Parallel()` + `t.TempDir()`), then retire the shell scripts (`mockway/scripts/test-examples.sh` + `test-misconfigured.sh` + `test-updates.sh`; `fakegcp/scripts/e2e.sh`). 4-PR cross-repo sweep per `reference_cross_repo_docs_sweep.md` — one PR per sibling.
- **S127** makes the S123 contract-coverage approach **durable for future fakes**. Introduces the `CRITICAL[contract-id]:` / `MUST[contract-id]:` docstring convention + a `handlers/contract_audit_test.go` in each sibling that walks `handlers/*.go`, extracts the IDs, and fails CI if any ID lacks a paired `TestContract_<id>`. Also updates the OSS-mature day-one checklist (item 14) + adds a one-line rule to `AGENTS.md § Planning a New Arc`. Then tags `v0.2.0` and folds the arc close-out (Option C). Another 4-PR cross-repo sweep (5th instance of `reference_cross_repo_docs_sweep.md`).

S127 owns the close-out per Option C (no separate slice). S123 adopts the convention from the start so the tests it writes already satisfy the audit when S127 lands.

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
| S126 | Cross-repo example-test convergence on the fakeaws in-test pattern + README updates | ~2.5–3.5 hr |
| S127 | Cross-repo `contract_audit_test.go` + `CRITICAL[id]:` convention rollout + OSS-mature checklist + AGENTS.md rule + v0.2.0 tag + arc close-out | ~3–4 hr + close-out |

**Total**: ~10–14 focused hours.

## Standing rules

Inherit all rules from `fakegenesys-arc-plan.md`, `slices-43-48-plan.md` (fakeaws review-loop precedent), and S116–S122 (see `docs/status/ARCHIVE.md` § "2026-06-10 fakegenesys S116–S122 sustain validation"). Specifically:

- **Codex anti-nitpick** (S124): act on substantive only; stop after 2 consecutive NOTHING_TO_IMPROVE passes; record declined items in `docs/review-passes/passN.md`. See `feedback_codex_anti_nitpick.md`.
- **OSS-mature day-one** (S125/S126): docker.yml + scripts mirror sibling layout exactly. No bespoke patterns. See `feedback_oss_mature_day_one.md`.
- **No nightly CI cost** (S125): the workflow runs on tag push only, NOT on a nightly schedule. See `feedback_cost_sensitive_ci.md`.
- **Merge authority**: `gh pr merge <N> --squash --admin --delete-branch` once CI green, in `../fakegenesys`.
- **Test scope** (S123): exercise corner cases of S122a–g endpoints (PascalCase JSON key sensitivity, subject grants empty/bulkadd/bulkremove flow, password POST 204, user/groups subresources). Do NOT pad coverage by exercising trivial happy paths already covered by integration; favor the contracts the genesyscloud provider depends on (recorded in handler docstrings).
- **All work in `../fakegenesys`** — no infrafactory changes expected unless S123 surfaces a real fidelity bug, in which case follow the standard cross-repo pattern.

## S123 — Contract-driven test backfill for S122a–g + post-S116 surface

### Motivation

Most S116c/S119/S122a–g endpoints landed under sweep pressure without unit-level tests. The handler docstrings document the wire-shape invariants the genesyscloud provider depends on (search `grep -n "CRITICAL:\|MUST" handlers/*.go`), but there's no automated test enforcing them. Future refactors (e.g. if someone touches `organization.go::handleTokensMe`) could silently break the `OAuthClient` PascalCase key invariant — the kind of regression that wouldn't surface until the next sustain sweep, which is exactly the failure mode this arc closes.

We do **not** target a line count. fakegenesys's API surface is genuinely smaller than fakegcp/fakeaws (smaller endpoint set, narrower CCaaS scope), and demanding line parity would force padding. Contract coverage scales correctly: small API → fewer tests, dense-with-contracts API → more tests.

### Coverage bars (the exit criteria — line count is not a metric)

Every test added must lock in a specific contract. The three bars:

1. **Every S122a–g mock-gap layer has a regression test.** Each gap was a real provider crash or hang during the sustain arc. The regression test asserts what would have failed *before* the fix — revert the fix in CI and the test breaks.
2. **Every `CRITICAL[id]:`/`MUST[id]:` note in handler docstrings becomes one explicit assertion.** The docstrings already enumerate the invariants (the long explanatory comment above `handleTokensMe`, `handleAuthorizationProducts`, etc.). Convert them, don't reinvent them.
3. **Every nil-deref crash site the SDK walks gets an explicit non-nil assertion.** The genesyscloud Go SDK dereferences pointer fields (`OAuthClient.Organization.Id`, `Division.Id`, `Total`) without nil checks. Each such field on a stub response gets an assertion that it's present and non-nil/non-empty.

### Convention adopted from the start (S127 forward-compatibility)

S127 introduces a durable `CRITICAL[contract-id]:` docstring convention + paired `TestContract_<contract-id>` test names + a CI audit. S123 must write its tests AND amend its docstrings under that convention so the S127 audit lands clean on day one:

- **Docstring form**: `// CRITICAL[OAuthClient-PascalCase]: <existing explanation>`. `<contract-id>` is kebab-case, descriptive, stable (avoid IDs that read like ticket numbers or dates).
- **Test name form**: `TestContract_OAuthClient_PascalCase` (`-` → `_`).
- **One-to-one**: each `CRITICAL[id]:` / `MUST[id]:` note has exactly one paired test; each `TestContract_<id>` has exactly one paired docstring note.

Pre-S116 handlers (the original v0.1.0 surface) keep their plain `CRITICAL:` / `MUST:` notes — S127 sweeps them later. S123 only touches the post-S116/S122 docstrings.

### Tickets

| ID | Description | Priority | Deps |
|---|---|---|---|
| S123-T1 | Inventory: `grep -n "CRITICAL:\|MUST" handlers/*.go` → produces the docstring-derived contract list. Cross-reference each S122a–g PR description for the mock-gap layer it closed. Build a single matrix in the PR description with columns: `[handler / endpoint / contract / S122 layer / nil-deref-site / test name]`. Every row is one test. This matrix IS the coverage plan — no test outside it, no row without a test. | P0 | — |
| S123-T2 | **Regression tests for S122a–g layers.** One test file per layer (or share a file where the layer touches the same handler). Each test boots the in-process Application via the existing test helper, fires the request the provider call-chain step uses (read from the layer's PR description for the exact route + method + body shape), asserts the response shape that unblocked the call chain. Examples: `tokens_me_test.go` asserts the JSON contains key literally `"OAuthClient"` (PascalCase — the SDK's custom UnmarshalJSON does case-sensitive map lookup); `authorization_subject_test.go` asserts GET returns `grants:[]` and POST bulkadd returns 204; `users_me_test.go` asserts `division.id != ""`; `flow_jobs_test.go` asserts the upload-job protocol terminal status. | P0 | S123-T1 |
| S123-T3 | **Docstring-derived assertions.** Walk every `CRITICAL:`/`MUST` note from the T1 grep. For each: add an assertion to an existing or new test that would fail if the invariant were broken. Example: `handleAuthorizationProducts`'s "the response MUST include 'total' (int)" → assert the response JSON has `total` and it's a number. Test name should reference the contract source: `TestAuthorizationProducts_TotalIsInt_PreventsPluginSegfault`. | P0 | S123-T1 |
| S123-T4 | **Nil-deref defenses.** For each pointer field the SDK dereferences on stub responses (audit by reading the relevant provider source files referenced in the handler docstrings), assert non-nil/non-empty on the stub response. These are the "would have segfaulted the plugin" cases. Most pile onto the same test files as T2/T3 — they share fixtures. | P0 | S123-T1 |
| S123-T5 | Run `go test ./...` from `../fakegenesys`. Run `make test` from infrafactory. Confirm both pass. Confirm the T1 matrix is fully populated — every row has a test, every test maps back to a row. | P0 | S123-T2, S123-T3, S123-T4 |
| S123-T6 | Single PR with all new tests. PR description embeds the T1 coverage matrix. Title: `S123: contract-driven test backfill for S116/S122 surface`. Description states the three coverage bars and shows the matrix as evidence. **No line-count claim in the PR description** — the matrix is the evidence. | P0 | S123-T5 |

### Exit criteria

- The T1 matrix is fully populated: every `CRITICAL:`/`MUST` docstring note has a test, every S122a–g layer has a regression test, every nil-deref crash site has a non-nil assertion.
- Each test is named after the contract it locks in (so a reader can trace `git blame` from the contract back to the handler docstring that motivates it).
- `go test ./...` green; CI green; `make test` green; PR squash-merged.
- **No line-count target.** If reviewers ask for "more tests," the answer is "show me a missing matrix row" — padding without a row is rejected.

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

## S126 — Cross-repo example-test convergence + READMEs

### Motivation

Three different example-test patterns across four sibling fakes is incoherent: mockway has 3 shell scripts (`test-examples.sh` + `test-misconfigured.sh` + `test-updates.sh`), fakegcp has 1 (`e2e.sh`), fakeaws drives examples in-test, and fakegenesys runs only a provider-init smoke pass. Pick one. fakeaws's in-test pattern wins on every dimension that matters (see § "Why in-test, not shell" above) and is the most recent considered choice, so it becomes canonical.

This slice converges all four siblings onto that pattern in a single cross-repo sweep (`reference_cross_repo_docs_sweep.md` — 4-PR shape), updates each repo's README to point at `go test ./examples/...` as the canonical entry point, and retires the shell scripts. The v0.2.0 fakegenesys tag + arc close-out are deferred to S127 (so the contract-audit work lands first).

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

#### Infrafactory tracking update

| ID | Description | Priority | Deps |
|---|---|---|---|
| S126-T15 | Update `infrafactory/AGENTS.md` — in the cross-cutting fidelity-strategy comparison table (or the smoke-harness section if separate), add a note that **example-driven coverage is in-test (`go test ./examples/...`) across all four siblings as of S126** — same convergence pattern as the smoke-harness and fidelity-strategy doc sweeps. (No close-out commit yet — that lands in S127.) | P0 | S126-T4, S126-T8, S126-T12, S126-T14 all merged |

### Exit criteria

- All four sibling repos have an `examples/provider_smoke_test.go` that drives `tofu init`+`apply`+`destroy` per-example via `t.Parallel()`.
- `mockway/scripts/test-examples.sh`, `mockway/scripts/test-misconfigured.sh`, `mockway/scripts/test-updates.sh`, `fakegcp/scripts/e2e.sh` are deleted.
- All four READMEs have a "Testing examples" section with the same wording, pointing at `go test ./examples/...`.
- infrafactory `AGENTS.md` records the cross-repo convergence.
- All four PRs squash-merged with CI green.

## S127 — Cross-repo contract-audit rollout + OSS-mature update + v0.2.0 + arc close-out

### Motivation

S123 introduced contract coverage for fakegenesys's S116/S122 surface — but it lives as a convention (test-names-paired-with-docstring-notes) enforced by reviewer attention. That's a hand-discipline bar. The next sibling-fake builder (5th cloud, S130-ish) won't have lived through the S122 sustain arc and won't know the convention exists. They'll add new endpoints with `CRITICAL:` notes in docstrings but skip the test, and the gap won't surface until a sustain sweep — exactly the regression mode this arc closes.

This slice makes the convention **durable**:
- **Code-level (Option A)**: each sibling fake gets a `handlers/contract_audit_test.go` that walks `handlers/*.go`, extracts `CRITICAL[<id>]:` / `MUST[<id>]:` IDs, and fails CI if any ID lacks a paired `TestContract_<id>` in the same package. Drift becomes a failed `go test`, not a missed review item.
- **Process-level (Option B)**: the `feedback_oss_mature_day_one.md` checklist grows item 14 (`handlers/contract_audit_test.go` + the convention). `infrafactory/AGENTS.md § Planning a New Arc` grows a one-line rule pointing at the convention. The next sibling-fake builder discovers the requirement when they read either of those.

Then v0.2.0 tag + arc close-out fold in (Option C — final slice owns the close-out).

This is the 5th instance of the 4-PR cross-repo sweep pattern (`reference_cross_repo_docs_sweep.md` after smoke-harness, fidelity-strategy, in-test example coverage, and now contract audit).

### Convention reference

```go
// handlers/organization.go
//
// CRITICAL[OAuthClient-PascalCase]: SDK's Tokeninfo.UnmarshalJSON does
// case-sensitive map lookup. camelCase keys are silently dropped, leaving
// OAuthClient nil and segfaulting the plugin during oauth_client create.
body := map[string]any{
    "OAuthClient": map[string]any{...},
}
```

```go
// handlers/organization_test.go
//
// TestContract_OAuthClient_PascalCase asserts the /tokens/me response
// contains key literally named "OAuthClient" (not "oAuthClient").
func TestContract_OAuthClient_PascalCase(t *testing.T) { ... }
```

The ID space is shared between docstring and test:
- `grep -rn "CRITICAL\[" handlers/` → all contracts
- `grep -rn "func TestContract_" handlers/` → all paired tests
- Audit asserts the two sets match.

Pre-existing `CRITICAL:` / `MUST:` notes without an `[id]` are swept to the new form as part of this slice; tests get backfilled where missing.

### Tickets

#### Reference + audit-test design (lands in fakegenesys first as the reference impl)

| ID | Description | Priority | Deps |
|---|---|---|---|
| S127-T1 | Write `handlers/contract_audit_test.go` in `../fakegenesys`. Implementation: walk `handlers/*.go` source files, parse for `CRITICAL[<id>]:` / `MUST[<id>]:` patterns (regex on comment lines), build the contracts set. Walk for `func TestContract_<id>(`, build the tests set. Fail with a useful diff if the sets don't match. Allow zero contracts (no-op pass). Test the audit itself: a `TestContractAuditTest_Self` sub-test that injects a known-good docstring + test and confirms the audit passes; injects a known-bad case and confirms it fails. | P0 | — |
| S127-T2 | Sweep `../fakegenesys/handlers/*.go` for pre-S116 `CRITICAL:` / `MUST:` notes. For each: assign a kebab-case contract ID; rewrite to `CRITICAL[id]:`/`MUST[id]:`; check if a paired `TestContract_<id>` exists (S123's tests should already, but pre-S116 ones may not); add one if missing — but only if it's actually a wire-shape invariant, not a stale comment. Drop the `CRITICAL`/`MUST` framing on any note that turns out to be aspirational rather than contractual. | P0 | S127-T1 |
| S127-T3 | Run `go test ./handlers/...` from `../fakegenesys`. Confirm `TestAllContractsHaveTests` (or whatever S127-T1 names the entry point) passes. | P0 | S127-T2 |
| S127-T4 | Single PR. Title: `S127/fakegenesys: contract_audit_test.go + CRITICAL[id]: convention rollout`. PR description embeds the contract ID list as evidence. | P0 | S127-T3 |

#### Cross-repo rollout (mockway → fakegcp → fakeaws)

For each sibling: copy the `contract_audit_test.go` template from fakegenesys, sweep existing `CRITICAL:`/`MUST:` notes into the `[id]` form, backfill missing `TestContract_<id>` tests where the note describes a real wire-shape invariant.

| ID | Description | Priority | Deps |
|---|---|---|---|
| S127-T5 | Apply S127-T1–T4 to `../mockway`. Walk `handlers/`, sweep contract notes, add audit, ship as one PR. Title: `S127/mockway: contract_audit_test.go + CRITICAL[id]: rollout`. | P0 | S127-T4 (template) |
| S127-T6 | Apply S127-T1–T4 to `../fakegcp`. Same shape. Title: `S127/fakegcp: contract_audit_test.go + CRITICAL[id]: rollout`. | P0 | S127-T4 (template) |
| S127-T7 | Apply S127-T1–T4 to `../fakeaws`. Same shape — fakeaws already has the most mature test surface, so most contracts likely already have paired tests; this is mostly a rename + audit add. Title: `S127/fakeaws: contract_audit_test.go + CRITICAL[id]: rollout`. | P0 | S127-T4 (template) |

#### Process-level enforcement + infrafactory tracking

| ID | Description | Priority | Deps |
|---|---|---|---|
| S127-T8 | Update `infrafactory/AGENTS.md § Planning a New Arc` (or the closest section if naming differs at execution time): add a one-line rule — "**New sibling fakes inherit the `CRITICAL[id]:`/`TestContract_<id>` contract-audit convention from day one (see `feedback_oss_mature_day_one.md` item 14).**" — plus a pointer to fakegenesys's `handlers/contract_audit_test.go` as the reference impl. | P0 | S127-T7 |
| S127-T9 | Update `reference_cross_repo_docs_sweep.md` (auto-memory) to record S126 (in-test example coverage) and S127 (contract audit) as the 4th and 5th instances of the 4-PR cross-repo pattern. Note the pattern is now mature enough that 5 instances exist; if a 6th comes up the docstring should mention "this is now a project standard." | P0 | S127-T8 |
| S127-T10 | Note: `feedback_oss_mature_day_one.md` item 14 was already added during the planning phase (this slice). Verify the memory's content reflects the convention shape S127 actually implemented; revise if the audit ended up materially different. | P1 | S127-T7 |

#### v0.2.0 release + arc close-out (Option C)

| ID | Description | Priority | Deps |
|---|---|---|---|
| S127-T11 | Tag `v0.2.0` in `../fakegenesys`: `git tag v0.2.0 && git push origin v0.2.0`. Update fakegenesys `CHANGELOG.md` with a `## [0.2.0] - 2026-MM-DD` section listing the five hardening dimensions (S123 contract tests, S124 review-loop closed, S125 docker workflow, S126 in-test convergence, S127 contract audit). | P0 | S127-T8 |
| S127-T12 | **Arc close-out** (Option C): append entry to `infrafactory/docs/status/ARCHIVE.md` § "2026-MM-DD fakegenesys v0.2 hardening — sibling parity + durable contract coverage" with sub-sections for each slice. Update `STATUS.md` baseline pointer. Refresh `docs/NEXT_SESSION.md` (next-arc candidate list). | P0 | S127-T11 |
| S127-T13 | Update memory: edit `project_fakegenesys_arc_closeout.md` to add a "v0.2 hardening (2026-MM-DD)" section. Update `MEMORY.md` "Latest" entry. Verify `feedback_test_coverage_metrics.md` and `feedback_oss_mature_day_one.md` reflect the as-shipped convention. | P0 | S127-T12 |

### Exit criteria

- All four sibling repos have a `handlers/contract_audit_test.go` that fails CI when a `CRITICAL[id]:`/`MUST[id]:` docstring has no paired `TestContract_<id>` (or vice versa).
- All existing `CRITICAL:`/`MUST:` notes across the four siblings have been swept into the `[id]` form OR demoted (if they turned out to be aspirational rather than contractual).
- `infrafactory/AGENTS.md` has a one-line rule pointing at the convention.
- `feedback_oss_mature_day_one.md` item 14 reflects the as-shipped audit shape (Option B verified).
- `v0.2.0` tag pushed; fakegenesys `CHANGELOG.md` has a `[0.2.0]` section.
- `infrafactory/docs/status/ARCHIVE.md` + `STATUS.md` + `docs/NEXT_SESSION.md` updated.
- MEMORY updated.
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
- **Test coverage**: the S123 matrix is fully populated — every `CRITICAL[id]:`/`MUST[id]:` docstring note has a paired `TestContract_<id>`, every S122a–g layer has a regression test, every nil-deref crash site has a non-nil assertion. **Not a line count.**
- fakegenesys workflow count = 3 (currently 2)
- fakegenesys review-pass count ≥ 4 with ≥ 2 consecutive NOTHING_TO_IMPROVE
- fakegenesys tagged at `v0.2.0`
- **Cross-repo example tests**: all four `examples/provider_smoke_test.go` drive per-example apply matrix; zero example shell scripts remain across the four siblings; all four READMEs share the same "Testing examples" wording.
- **Cross-repo contract audit**: all four `handlers/contract_audit_test.go` fail CI when a docstring contract has no paired test (or vice versa); convention is documented in `feedback_oss_mature_day_one.md` item 14 + `infrafactory/AGENTS.md § Planning a New Arc`.

Sibling parity along all five dimensions (contract coverage, review-pass loop, docker workflow, in-test example pattern, durable contract audit) is the close-out trigger. The audit makes the contract-coverage approach **durable for future fakes** — drift becomes a failed CI run, not a missed review item.
