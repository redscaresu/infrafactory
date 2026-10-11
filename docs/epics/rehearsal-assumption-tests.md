---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: []
---

# The tofu and provider behaviour the rehearsal design rests on is pinned by tests before any rehearsal code exists (Rollout 0, tofu half; ADR-0028).

**Done when:**
- A `TestAssumption_*` exists for each claim, each shown to fail when the claim is broken (cp-backup break recorded in the PR):
  (a) a provider override that names every key of the `provider "aws"` `endpoints` block (STS included) makes `tofu apply` with real-looking keys in the shell die at a dead loopback address;
  (b) a `backend "local" {}` override with no `path` plus `-backend-config=path=<outside>` keeps the state out of the copy;
  (c) `tofu init -backend=false` under a credential-free env with an empty HOME and no `TF_CLI_CONFIG_FILE`/`TF_PLUGIN_CACHE_DIR` fetches providers and modules, and the second `init -reconfigure -get=false -lockfile=readonly -plugin-dir=...` opens no socket (E3);
  (d) the tofu facts behind E2: an `external` key provider in `terraform { encryption {} }` executes a command at init, and `tofu state pull` emits decrypted state;
  (e) a data source the mock cannot read makes `plan -refresh-only -detailed-exitcode` exit 1, distinct from 2; `apply -refresh-only` has no exit 2 (E1; ADR-0032);
  (f) against an empty mock the provider sends a data source's request before the plan fails, and independent data sources all send in one pass;
  (g) a user `zz_override.tf` merges after infrafactory's override, and a root override does not reach a `provider "aws"` declared inside a child module (the facts the gate's refusals rest on).
- Each test is parametrised by the tofu on PATH and the provider version the lock names (5.100.0 in CI), so a tofu upgrade that breaks a claim fails CI.
- ADR-0028 lists these claims; `make adr-index` is current.
- If a claim is false, the PR says so and the HLD is revised before the dependent epic is scoped (a valid outcome).

**Out of scope:** any rehearsal command, gate or runner change (the gate's refusal of `encryption`/`cloud` is rehearsal-prepare-gate's); fakeaws; real AWS.

**Constraints:** ADR-0028 (claim = test, comment cites it); every new test shown to fail; pattern `internal/cli/assumptions_test.go`; e2e gating `INFRAFACTORY_ENABLE_E2E=1` where a provider must be fetched; nothing in pitfalls/, policies/, prompts/.

**Your part:** none.

**Areas:** `internal/cli/ (rehearsal_assumptions_test.go + testdata)`; `docs/decisions/0028-*`, `docs/decisions/README.md`
