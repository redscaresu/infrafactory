---
status: active
hld: 2026-09-30-import-production-state-into-the-mocks
depends_on: []
---

# New code is copied and gated before any tofu, every downloaded module is gated before the second init, and both inits run under a walled environment (Rollout 4, first clauses; goals 2, 9, 10, 11).

**Done when:**
- First story: the `infrafactory rehearse` command group and ADR-0042 "the rehearsal command contract and its walls" (command family, flags, the `.infrafactory/rehearsals/<id>/` layout with 0700 dir and 0600 files, the three walls). Every other `rehearse *` story waits for it.
- `rehearse prepare <dir> --account <id> --region <r> [--var-file f]` copies the code and its local module ancestor; refuses a module source escaping the repository, git/SSH/private-registry sources, `encryption` and `cloud {}` before the first tofu command.
- The gate (new file; reuses the HCL parse helpers, never the Layer 3 gate) walks every module directory, refuses user `*override.tf*` files and `provider "aws"` blocks inside child modules by name, and states its own extension policy: tfvars admitted and parsed; `.tf.json`/`.tofu` decided here, not inherited from layer3ParseFile. Admits data, module, lifecycle, count/for_each, providers hashicorp/aws, random, null, tls, time, cloudinit, local, archive; refuses provisioner, connection, http/external data sources, terraform_remote_state, other remote providers, escaping paths, and `provider "aws"` arguments outside an allowlist derived from `tofu providers schema -json`, with a test that fails when the schema adds an argument the table does not name.
- Goal 11 is checked on the resolved region: a non-literal region is resolved from the planned configuration or refused; a `var.region = other-region` test refuses.
- Init runs twice: network-only with `-backend=false` under the replace-mode env with an empty HOME and no TF_CLI_CONFIG_FILE or TF_PLUGIN_CACHE_DIR (tested); then the full gate runs again over the root and every module modules.json names (a test plants a provisioner in a fetched module and sees it refused); then the second init with `-lockfile=readonly -plugin-dir` under the dead proxy.
- The provider version is always the lock's; a registry.terraform.io lock warns; a lock naming an aws provider other than 5.100.0 runs with unknown provider-block arguments refused, and the report names the version so a failure is filed as a fakeaws story.
- harness.Command gains a replace mode carrying PATH and only named variables; a test proves HOME, TF_CLI_CONFIG_FILE, TF_REATTACH_PROVIDERS, TF_LOG_PATH, TF_PLUGIN_CACHE_DIR, TF_CLI_ARGS*, TF_VAR_*, AWS_IGNORE_CONFIGURED_ENDPOINT_URLS and AWS_SESSION_TOKEN never pass.
- Golden files; docs/operations.md runbook section.

**Decide when scoping (found by rehearsal-assumption-tests, #450 and #455):**
- Encryption reaches tofu by more than the HCL `encryption` block: the `TF_ENCRYPTION` variable, `.tofu` files and `.tf.json`. Refusing the block by name is a denylist; the gate closes all three (replace mode never passes `TF_ENCRYPTION`, with a test; the extension policy covers `.tofu` and `.tf.json`).
- A public-registry module is fetched through `git::https://github.com/...`, so the first init needs `git` and reaches github.com. Decide whether "git sources" is judged by the source string (admits registry modules) or by what the fetch does (refuses them), and test the choice.

**Out of scope:** the provider override, the rehearsal environment, the fakeaws process, `end` (rehearsal-walls); discovery onward; the tfstate pull; the UI; admitting assume_role.

**Constraints:** ADR-0023 gate bar (parse not scan, deny by default, unparseable = refused, no tofu for a refused configuration); ADR-0002 CLI contract; ADR-0028 for any tofu claim not in the 2026-10-11 amendment; the runner's process group, SIGINT and single-interrupt rules reused; ADR edit or `ADR: none` on every internal/cli PR; tofu claims pinned by `TestAssumption_UserOverrideMergesAfterOurs`, `TestAssumption_RootOverrideMissesChildModuleProvider`, `TestAssumption_SecondInitMakesNoProxiedConnection`, `TestAssumption_ExternalKeyProviderRunsAtBackendInit` (ADR-0028).

**Your part:** none (ADR-0042 merges under standing authority).

**Areas:** `internal/cli/ (rehearse.go, rehearse_prepare*.go, rehearse_gate*.go new; root.go; contract.go; golden files)`; `internal/harness/static.go (Command replace mode)`; `internal/cli/exec_runner.go`; `docs/decisions/0042 (new), README.md`; `docs/operations.md`
