# Arc: fakegenesys example drift fix (smoke harness self-containment + HCL refresh)

Status: planned (2026-06-10)
Owner: next-session claude (designed for autonomous execution)
Follows: S128–S135 (sibling CRITICAL sweep + AGENTS/README cleanup) + post-S135 lockstep audit. Discovers smoke-harness ergonomics gap surfaced by user running `FAKEGENESYS_ENABLE_E2E=1 go test ./examples/...` standalone.
Shape: goal-named 3-slice arc (~5–7 hr).

## Big picture

Running `FAKEGENESYS_ENABLE_E2E=1 go test ./examples/...` from a fresh checkout fails with a mix of:

1. **`Auth Error: 400 - invalid_client`** on most examples (auth_role, architect_user_prompt, group, location, oauth_client, ...). The provider reaches fakegenesys but sends empty credentials.
2. **`Insufficient properties blocks`** / **`Unsupported argument`** on a few examples (architect_datatable, flow, idp_generic). The HCL has drifted from what the current `mypurecloud/genesyscloud` provider expects.

These two failure modes LOOK similar (red `tofu apply` output) but they're different problems:

- (1) is a **harness self-containment gap**: the smoke test boots fakegenesys + its TLS MITM proxy, but doesn't set the env vars the genesyscloud provider's SDK reads (`HTTPS_PROXY`, `SSL_CERT_FILE`, `NO_PROXY`, `GENESYSCLOUD_OAUTHCLIENT_ID/SECRET/REGION`). The infrafactory harness (`internal/cli/cloudEnv`) sets these automatically for `cloud:genesys` scenarios. The standalone go-test doesn't. As a result, every example fails identically in a way that obscures the real HCL drift in (2).
- (2) is **example HCL rot**: the provider has evolved (e.g. `genesyscloud_architect_datatable.schema = jsonencode({...})` → `properties { ... }` blocks; `genesyscloud_idp_generic.certificate` → `certificates`; `genesyscloud_flow.file_content_hash` became unconfigurable). The examples were last validated during S108–S115 against an older provider version. Nothing in the system defends the examples from upstream provider drift — different layer from the contract audit.

We have to fix (1) before we can see (2) cleanly. After (1), running the smoke test will give a deterministic inventory of HCL drift to fix in (2).

The plan also touches `examples/README.md` (S126's "Testing examples" stanza) — currently misleading because it implies `go test ./examples/...` works standalone, but it doesn't without the env-var setup.

## Slices

| Slice | Title | Effort |
|---|---|---|
| S136 | Smoke harness self-containment (env-var bridge) | ~2 hr |
| S137 | HCL drift inventory + fixes (working/ tree) | ~2–3 hr |
| S138 | updates/ + misconfigured/ audit + close-out + v0.2.1 tag | ~1–2 hr + close-out |

**Total**: ~5–7 focused hours.

## Standing rules

Inherit from `sibling-critical-sweep-plan.md` + `feedback_test_coverage_metrics.md`:

- **Codex anti-nitpick** — single review pass per PR; stop on first NOTHING_TO_IMPROVE.
- **Never hand-edit `pitfalls/*.yaml`** (stash before commits).
- **No new CI nightlies.**
- **Pin the provider version** in any examples we touch so future provider releases don't silently break us again — surface drift loudly via a known-broken entry rather than silently failing.
- **Use `known_broken.yaml`** for any example we genuinely can't fix cheaply (deprecated resource shape, requires real-cloud-only feature). Each entry needs a tracking note.
- All work in `../fakegenesys`. Infrafactory only sees the close-out doc updates.

## S136 — Smoke harness self-containment

### Motivation

`examples/provider_smoke_test.go` boots a fakegenesys per example on a random port + the TLS MITM proxy on `:8443`. But it doesn't set the env vars the genesyscloud Go SDK reads:

- `HTTPS_PROXY=http://localhost:8443` — routes provider HTTPS calls (`login.<region>.pure.cloud`) through fakegenesys's MITM
- `SSL_CERT_FILE=<path>` — points at the fakegenesys CA so the provider's TLS client trusts the MITM
- `NO_PROXY=localhost,127.0.0.1` — keeps the in-process flow-upload URL out of the proxy loop
- `GENESYSCLOUD_OAUTHCLIENT_ID=any` / `GENESYSCLOUD_OAUTHCLIENT_SECRET=any` / `GENESYSCLOUD_REGION=us-east-1` — credentials + region the provider must see

`internal/cli/cloudEnv` in infrafactory sets these per-scenario. The smoke test needs to do the same so it runs standalone in CI and from a fresh `git clone`.

### Tickets

| ID | Description | Priority |
|---|---|---|
| S136-T1 | In `examples/provider_smoke_test.go::spawnFakegenesys` (or a new helper called from `walkExamplesAndRun`): after the fakegenesys boots, fetch the CA via `GET /mock/ca-cert`, write to a `t.TempDir()` file, capture the path. Set the 4 env vars listed above using `t.Setenv` (auto-cleaned by Go's test runner). | P0 |
| S136-T2 | Verify by running ONE known-clean example end-to-end: `FAKEGENESYS_ENABLE_E2E=1 go test ./examples/... -v -run 'TestProviderSmokeWorking/oauth_client'`. Expect: apply, plan-no-op, destroy all succeed. | P0 |
| S136-T3 | Update `examples/README.md` (or wherever the "Testing examples" stanza lives — added in S126) to drop the "needs env-var setup" caveat that I'm pre-empting. Reflect that the smoke test now self-contains. | P0 |
| S136-T4 | Update `README.md` § "Testing examples" stanza in fakegenesys to mention the harness is self-contained as of v0.2.1 (forward-reference). | P1 |
| S136-T5 | Single PR. Title: `S136: smoke harness self-containment (env-var bridge for standalone go test)`. PR description names each env var + why. | P0 |

### Exit criteria

- `FAKEGENESYS_ENABLE_E2E=1 go test ./examples/... -run TestProviderSmokeWorking/oauth_client -v` passes end-to-end from a fresh clone, no manual env setup.
- The 4 env vars are set in the test setup, not the operator's shell.
- `examples/README.md` accurately describes the run instructions.
- PR squash-merged with CI green.

## S137 — HCL drift inventory + fixes (working/ tree)

### Motivation

With S136 merged, a clean run of `go test ./examples/... -run TestProviderSmokeWorking` will give a deterministic inventory of HCL drift. Known failures from the pre-S136 run (filtered to real HCL drift, not the auth-cascade):

- `architect_datatable/working`: `Insufficient properties blocks` + `Unsupported argument "schema"`. Provider replaced `schema = jsonencode({...})` with `properties { ... }` blocks. Need to refactor `main.tf` accordingly.
- `flow/working`: `Value for unconfigurable attribute "file_content_hash"`. Provider made this computed-only; HCL must drop it.
- `idp_generic/working`: `Missing required argument "certificates"` + `Unsupported argument "certificate"`. Singular → plural list.

Plus any others the clean re-run surfaces.

### Tickets

| ID | Description | Priority |
|---|---|---|
| S137-T1 | After S136 merges, run `FAKEGENESYS_ENABLE_E2E=1 go test ./examples/... -v -run TestProviderSmokeWorking 2>&1 \| tee /tmp/smoke-baseline.log`. Build the actual drift inventory — one row per failing example. Cross-check each error against the current `mypurecloud/genesyscloud` provider source (`github.com/mypurecloud/terraform-provider-genesyscloud/genesyscloud/<resource>/resource_*.go`) or its docs. | P0 |
| S137-T2 | Fix `examples/working/architect_datatable/main.tf`: replace `schema = jsonencode({...})` with `properties { name = "..." type = "..." }` blocks (one per column, mirroring the JSON schema's fields). Verify the example passes the per-tree contract: apply → plan-no-op → destroy. | P0 |
| S137-T3 | Fix `examples/working/flow/main.tf`: drop the `file_content_hash` line. The provider now computes it automatically. Verify. | P0 |
| S137-T4 | Fix `examples/working/idp_generic/main.tf`: rename `certificate = "..."` to `certificates = ["..."]`. Verify. | P0 |
| S137-T5 | For any OTHER drift surfaced by T1's clean run: same per-example fix pattern. List in the PR description. | P0 |
| S137-T6 | If any example's drift can't be cheaply fixed (e.g. the resource was deprecated or requires real-cloud-only behaviour), add an entry to `examples/known_broken.yaml` with a tracking note. Use sparingly; each entry needs justification. | P1 |
| S137-T7 | Pin the provider version in the affected examples' `versions.tf` (or `providers.tf`) to the version used during this arc. Surfaces future drift loudly at upgrade time rather than silently breaking. | P1 |
| S137-T8 | Single PR. Title: `S137: refresh examples/working HCL for current genesyscloud provider schema`. Embed the drift inventory + per-example fix summary + provider version pin. | P0 |

### Exit criteria

- `FAKEGENESYS_ENABLE_E2E=1 go test ./examples/... -run TestProviderSmokeWorking` passes 100% (or 100% minus known_broken entries with documented tracking).
- Every fixed example's `versions.tf` pins the provider version.
- Drift inventory embedded in the PR description.
- PR squash-merged with CI green.

## S138 — updates/ + misconfigured/ audit + close-out + v0.2.1 tag

### Motivation

`examples/updates/` and `examples/misconfigured/` also exercise the genesyscloud provider but via different test functions (`TestProviderSmokeUpdates`, `TestProviderSmokeMisconfigured`). Each will hit similar drift if the underlying provider's `genesyscloud_<resource>` schema has shifted. With S136+S137 closed, the misconfigured/ and updates/ trees deserve the same audit. Then close-out.

### Tickets

| ID | Description | Priority |
|---|---|---|
| S138-T1 | Run `FAKEGENESYS_ENABLE_E2E=1 go test ./examples/... -run TestProviderSmokeUpdates` and `-run TestProviderSmokeMisconfigured`. Build drift inventories for each tree. | P0 |
| S138-T2 | Per-example fix the updates/ tree (each dir has `v1.tfvars` + `v2.tfvars` + `main.tf` — both varfiles may need updates). | P0 |
| S138-T3 | Per-example fix the misconfigured/ tree (`expected.txt` may need updates if the provider's error wording changed). | P0 |
| S138-T4 | After all three trees pass: bump fakegenesys to **v0.2.1** in `CHANGELOG.md`. Tag + push. The release workflow auto-builds binaries; verify the GitHub release page lands as published (not Draft). | P0 |
| S138-T5 | **Arc close-out** (Option C): append entry to `infrafactory/docs/status/ARCHIVE.md` § "2026-06-10 fakegenesys example drift fix". Update `STATUS.md` (note v0.2.1). Refresh `docs/NEXT_SESSION.md` baseline. | P0 |
| S138-T6 | Update memory: `project_fakegenesys_arc_closeout.md` gets a new section for this arc. `MEMORY.md` "Latest" entry updated. | P0 |
| S138-T7 | **Capture the lesson**: write a new `feedback_example_hcl_drift.md` memory documenting the pattern — example HCL rots silently as upstream provider versions evolve; the contract audit doesn't catch this because it's a different layer (mock's wire shape vs example's HCL shape). Surface the mitigation: provider version pin + periodic smoke-test sweep on major provider releases. | P1 |
| S138-T8 | Single PR per concern: one fakegenesys PR for T1-T3 + T4, one infrafactory PR for T5-T7. | P0 |

### Exit criteria

- All three example trees (`working/`, `updates/`, `misconfigured/`) pass their per-tree contracts.
- fakegenesys v0.2.1 tagged + pushed. Release page shows it as Latest published.
- `infrafactory/STATUS.md` + `docs/status/ARCHIVE.md` + `docs/NEXT_SESSION.md` updated.
- `MEMORY.md` "Latest" entry reflects the arc.
- `feedback_example_hcl_drift.md` exists and is indexed in MEMORY.md.

## Out of scope

- Refactoring `provider_smoke_test.go` beyond the env-var bridge (it works; don't fix what isn't broken).
- Adding new examples. Existing examples are the scope.
- Touching the contract audit machinery. The contract audit defends wire shape; example HCL is a different layer (this arc surfaces that distinction in the feedback memory).
- Backporting fixes to v0.1.0 / v0.2.0 — v0.2.1 is the next release.

## Risk: provider version

The current `mypurecloud/genesyscloud` provider may continue to evolve. By pinning the provider version in `versions.tf` per-example (T137-T7), we surface upgrade-time drift as a deliberate sweep rather than a silent breakage. If we don't pin, the next provider release could re-break the examples — and the smoke test would fail loudly in CI rather than silently in production. Either is acceptable; pinning is the more conservative choice and matches how the fakeaws + fakegcp examples handle this.

## Authority

`gh pr merge <N> --squash --admin --delete-branch` once CI green. fakegenesys PRs in `../fakegenesys`; infrafactory PR in this repo.
